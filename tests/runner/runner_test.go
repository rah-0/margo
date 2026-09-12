package runner_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
	"github.com/rah-0/margo/runner"
	"github.com/rah-0/margo/structs"
	testerrs "github.com/rah-0/margo/tests/errs"
)

type borrowedQueriesCase struct {
	name   string
	source fs.FS
	want   error
	custom bool
}

type borrowedErrorsCase struct {
	name       string
	database   driver.Value
	queryErr   error
	migration  bool
	filesystem bool
	want       error
}

type generationSentinelsCase struct {
	name   string
	module string
	query  string
	want   error
}

type connectionSignal struct{}

// Each pool has its own connector, with no registered driver or global test state.
type testConnector struct {
	database driver.Value
	queryErr error
	block    bool
	started  chan connectionSignal
	closed   atomic.Int32
}

func (c *testConnector) Connect(context.Context) (driver.Conn, error) {
	return &testConn{connector: c}, nil
}
func (c *testConnector) Driver() driver.Driver { return testDriver{} }

type testDriver struct{}

func (testDriver) Open(string) (driver.Conn, error) {
	return nil, testerrs.ErrConnectorRequired
}

type testConn struct {
	connector *testConnector
}

func (c *testConn) Close() error { c.connector.closed.Add(1); return nil }
func (*testConn) Begin() (driver.Tx, error) {
	return nil, testerrs.ErrUnexpectedTransaction
}
func (*testConn) Prepare(string) (driver.Stmt, error) {
	return nil, testerrs.ErrUnexpectedPrepare
}
func (*testConn) Ping(context.Context) error { return nil }
func (c *testConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if query == "SELECT DATABASE()" {
		if c.connector.block {
			close(c.connector.started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &testRows{values: [][]driver.Value{{c.connector.database}}}, nil
	}
	if c.connector.queryErr != nil {
		return nil, c.connector.queryErr
	}
	if !strings.Contains(query, "information_schema.tables") || len(args) != 2 || args[0].Value != c.connector.database || args[1].Value != migrate.TableName {
		return nil, fmt.Errorf("%w: %s, %v", testerrs.ErrUnexpectedSchemaQuery, query, args)
	}
	return new(testRows), nil
}

type testRows struct {
	values [][]driver.Value
}

func (*testRows) Columns() []string { return []string{"value"} }
func (*testRows) Close() error      { return nil }
func (r *testRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		return io.EOF
	}
	copy(dest, r.values[0])
	r.values = r.values[1:]
	return nil
}

func borrowedTestPool(t *testing.T, connector *testConnector) *sql.DB {
	t.Helper()
	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(3)
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

func assertPoolOpen(t *testing.T, pool *sql.DB, connector *testConnector) {
	t.Helper()
	if err := pool.PingContext(t.Context()); err != nil {
		t.Fatalf("borrowed pool no longer usable: %v", err)
	}
	if connector.closed.Load() != 0 || pool.Stats().MaxOpenConnections != 3 {
		t.Fatalf("borrowed pool was closed or reconfigured: closed=%d stats=%+v", connector.closed.Load(), pool.Stats())
	}
}

func TestRunBorrowedEmptySchema(t *testing.T) {
	t.Parallel()
	connector := &testConnector{database: "empty_schema"}
	pool := borrowedTestPool(t, connector)
	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(output, "go.mod"), []byte("module example.com/generated\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(t.Context(), runner.Options{DB: pool, OutputPath: output}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Join("EmptySchema", "queries.go"), filepath.Join("errs", "errors.go")} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("empty schema must generate %s: %v", name, err)
		}
	}
	assertPoolOpen(t, pool, connector)
}

func TestRunBorrowedQueriesFS(t *testing.T) {
	t.Parallel()
	readErr := &fs.PathError{Op: "open", Path: "FindCount.sql", Err: fs.ErrPermission}
	queries := fstest.MapFS{"FindCount.sql": {Data: []byte("-- Returns: count\n-- ResultMode: one\nSELECT 1 AS count;")}}
	for _, test := range []borrowedQueriesCase{
		{name: "named queries", source: queries, custom: true},
		{name: "empty root", source: fstest.MapFS{}},
		{name: "open only", source: &observedFS{source: queries}, custom: true},
		{name: "read failure", source: &observedFS{source: queries, err: readErr, errPath: readErr.Path}, want: readErr},
		{name: "invalid query", source: fstest.MapFS{"Invalid.sql": {Data: []byte("SELECT * FROM alpha;")}}, want: errs.ErrSelectStarNotAllowed},
	} {
		t.Run(test.name, func(t *testing.T) {
			connector := &testConnector{database: "app"}
			pool := borrowedTestPool(t, connector)
			output := t.TempDir()
			if err := os.WriteFile(filepath.Join(output, "go.mod"), []byte("module example.com/generated\n\ngo 1.27\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			paths := []string{filepath.Join(output, "App", "queries.go"), filepath.Join(output, "errs", "errors.go")}
			for _, path := range paths {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("existing output"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			opts := runner.Options{DB: pool, OutputPath: output, Inputs: runner.Inputs{Queries: test.source}}
			err := runner.Run(t.Context(), opts)
			if !errors.Is(err, test.want) {
				t.Fatalf("Run error = %v, want %v", err, test.want)
			}
			if test.want == readErr {
				var pathErr *fs.PathError
				if !errors.As(err, &pathErr) || pathErr != readErr || !errors.Is(err, fs.ErrPermission) {
					t.Fatalf("query read error lost its filesystem cause: %v", err)
				}
			}
			for _, path := range paths {
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if test.want != nil && string(content) != "existing output" {
					t.Fatalf("query failure replaced %s", path)
				}
				if test.want == nil {
					if _, err := parser.ParseFile(token.NewFileSet(), path, content, parser.AllErrors); err != nil {
						t.Fatalf("query source did not generate valid Go in %s: %v", path, err)
					}
				}
				if test.want == nil && path == paths[0] && strings.Contains(string(content), "func QueryFindCount(") != test.custom {
					t.Fatal("generated queries do not match the filesystem source")
				}
			}
			if test.want == nil {
				if err := runner.Run(t.Context(), opts); err != nil {
					t.Fatalf("rerun with the same filesystem failed: %v", err)
				}
			}
			assertPoolOpen(t, pool, connector)
		})
	}
}

func TestRunBorrowedErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []borrowedErrorsCase{
		{name: "null selection", want: errs.ErrDatabaseNotSelected},
		{name: "empty selection", database: "", want: errs.ErrDatabaseNotSelected},
		{name: "schema query", database: "app", queryErr: testerrs.ErrSchemaUnavailable, want: testerrs.ErrSchemaUnavailable},
		{name: "migration discovery", database: "app", migration: true, want: errs.ErrInvalidFilename},
		{name: "filesystem migration discovery", database: "app", migration: true, filesystem: true, want: errs.ErrInvalidFilename},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			connector := &testConnector{database: tc.database, queryErr: tc.queryErr}
			pool := borrowedTestPool(t, connector)
			output := filepath.Join(t.TempDir(), "output")
			opts := runner.Options{DB: pool, OutputPath: output}
			if tc.filesystem {
				opts.Inputs.Migrations = fstest.MapFS{"invalid.sql": {Data: []byte("DO 0;")}}
			} else if tc.migration {
				root := t.TempDir()
				if err := os.WriteFile(filepath.Join(root, "invalid.sql"), []byte("DO 0;"), 0o600); err != nil {
					t.Fatal(err)
				}
				opts.Inputs.Migrations = os.DirFS(root)
			}
			err := runner.Run(t.Context(), opts)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Run error = %v, want %v", err, tc.want)
			}
			if tc.migration || tc.want == errs.ErrDatabaseNotSelected {
				if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failure created output: %v", err)
				}
			}
			assertPoolOpen(t, pool, connector)
		})
	}
}

func TestRunBorrowedCancellation(t *testing.T) {
	t.Parallel()
	connector := &testConnector{database: "app", block: true, started: make(chan connectionSignal)}
	pool := borrowedTestPool(t, connector)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output := filepath.Join(t.TempDir(), "output")
	result := make(chan error, 1)
	go func() { result <- runner.Run(ctx, runner.Options{DB: pool, OutputPath: output}) }()
	<-connector.started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancellation created output: %v", err)
	}
	assertPoolOpen(t, pool, connector)
}

func TestRunGenerationFilesystemError(t *testing.T) {
	t.Parallel()
	connector := &testConnector{database: "app"}
	pool := borrowedTestPool(t, connector)
	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(output, "App"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := runner.Run(t.Context(), runner.Options{DB: pool, OutputPath: output})
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("Run error = %v, want wrapped PathError", err)
	}
	assertPoolOpen(t, pool, connector)
}

func TestRunGenerationSentinels(t *testing.T) {
	t.Parallel()
	for _, tc := range []generationSentinelsCase{
		{name: "missing module", want: errs.ErrGoModuleNotFound},
		{name: "missing module directive", module: "go 1.27\n", want: errs.ErrModuleDirectiveNotFound},
		{name: "select star", module: "module example.com/generated\n\ngo 1.27\n", query: "SELECT * FROM users;", want: errs.ErrSelectStarNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			connector := &testConnector{database: "app"}
			pool := borrowedTestPool(t, connector)
			root := t.TempDir()
			if tc.module != "" {
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(tc.module), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			opts := runner.Options{DB: pool, OutputPath: filepath.Join(root, "generated")}
			if tc.query != "" {
				opts.Inputs.Queries = fstest.MapFS{"FindUsers.sql": {Data: []byte(tc.query)}}
			}
			if err := runner.Run(t.Context(), opts); !errors.Is(err, tc.want) {
				t.Fatalf("Run error = %v, want %v", err, tc.want)
			}
			assertPoolOpen(t, pool, connector)
		})
	}
}

func TestRunOwnedTransportFailureIsSilent(t *testing.T) {
	if output := os.Getenv("MARGO_TEST_TRANSPORT_OUTPUT"); output != "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				_ = conn.Close() // An incomplete handshake normally invokes the driver logger.
			}
		}()
		host, port, err := net.SplitHostPort(listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		connection := structs.ConnectionOptions{User: "test", Password: "test", Database: "test", Host: host, Port: port}
		before := connection
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		if err := runner.Run(ctx, runner.Options{Connection: &connection, OutputPath: output}); err == nil {
			t.Fatal("expected connection failure")
		}
		if connection != before {
			t.Fatal("Run mutated connection settings")
		}
		if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("connection failure created output: %v", err)
		}
		os.Exit(0) // Omit the test harness completion message when checking process output.
	}
	t.Parallel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), executable, "-test.run=^TestRunOwnedTransportFailureIsSilent$")
	command.Env = append(os.Environ(), "MARGO_TEST_TRANSPORT_OUTPUT="+filepath.Join(t.TempDir(), "output"))
	output, err := command.CombinedOutput()
	if err != nil || len(output) != 0 {
		t.Fatalf("owned connection failure wrote process output or failed: %v\n%s", err, output)
	}
}
