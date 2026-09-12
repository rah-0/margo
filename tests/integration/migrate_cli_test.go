//go:build integration

package integration

import (
	"database/sql"
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
	"github.com/rah-0/margo/structs"
)

type migrationCLIFailureCase struct {
	name         string
	files        map[string]string
	wantError    string
	createdTable string
	missingTable string
	failVersion  bool
}

func TestMigrationCLI(t *testing.T) {
	database := StartMariaDB(t)
	binary := filepath.Join(t.TempDir(), "margo")
	runTestCommand(t, margoRepositoryDir(t), append(os.Environ(), "GOWORK=off"),
		"go", "build", "-race", "-o", binary, "./cmd/margo")

	// The fixture user retains its database-scoped grant after this disposable
	// database is dropped, so the CLI must create it without server-wide access.
	if _, err := database.DB.ExecContext(t.Context(), "DROP DATABASE `margo_test`"); err != nil {
		t.Fatalf("drop disposable fixture database: %v", err)
	}
	if err := database.DB.Close(); err != nil {
		t.Fatalf("close fixture pool before bootstrap: %v", err)
	}

	outputPath := newMigrationOutputModule(t)
	migrationsPath := t.TempDir()
	queriesPath := t.TempDir()
	writeTestFile(t, filepath.Join(migrationsPath, "0001_users.sql"), []byte(`
CREATE TABLE IF NOT EXISTS users (id BIGINT PRIMARY KEY, name VARCHAR(100) NOT NULL);
INSERT INTO users (id, name) VALUES (1, 'initial')
ON DUPLICATE KEY UPDATE name = VALUES(name);
`))
	writeTestFile(t, filepath.Join(migrationsPath, "0002_email.sql"), []byte(`
ALTER TABLE users ADD COLUMN IF NOT EXISTS email VARCHAR(100) NOT NULL DEFAULT '';
UPDATE users SET email = 'initial@example.test' WHERE id = 1;
`))
	writeTestFile(t, filepath.Join(queriesPath, "GetEmail.sql"), []byte(`-- Params: id
-- Returns: email
-- ResultMode: one
SELECT email FROM users WHERE id = ?;
`))
	writeTestFile(t, filepath.Join(queriesPath, "GetUser.sql"), []byte(`-- Params: id
-- Returns: id name email
-- ResultMode: one
-- MapAs: users
SELECT id, name, email FROM users WHERE id = ?;
`))

	if output, err := runMigrationCLI(t, binary, database.Settings, outputPath, migrationsPath, queriesPath); err != nil {
		t.Fatalf("bootstrap and generate: %v\n%s", err, output)
	}
	var err error
	database.DB, err = sql.Open("mysql", database.DSN)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.DB.Close(); err != nil {
			t.Errorf("close migrated database: %v", err)
		}
	})
	assertCLIMigrationVersion(t, database.DB, 2)
	assertMigrationGeneratedTables(t, outputPath)

	writeTestFile(t, filepath.Join(outputPath, "MargoTest", "migration_runtime_test.go"), []byte(migrationRuntimeTests))
	runTestCommand(t, outputPath,
		append(os.Environ(), "GOWORK=off", "MARGO_INTEGRATION_DSN="+database.DSN),
		"go", "test", "-mod=mod", "-count=1", "-race", "-cover", "-covermode=atomic", "-p=1", "./...")

	t.Run("completed migrations are skipped", func(t *testing.T) {
		writeTestFile(t, filepath.Join(migrationsPath, "0001_users.sql"), []byte("invalid SQL must never execute"))
		if output, err := runMigrationCLI(t, binary, database.Settings, outputPath, migrationsPath, queriesPath); err != nil {
			t.Fatalf("generate with completed-file edit: %v\n%s", err, output)
		}
		assertCLIMigrationVersion(t, database.DB, 2)
	})

	t.Run("migration flag is optional", func(t *testing.T) {
		plainOutput := newMigrationOutputModule(t)
		if output, err := runMigrationCLI(t, binary, database.Settings, plainOutput, "", ""); err != nil {
			t.Fatalf("generate without migrations: %v\n%s", err, output)
		}
		assertMigrationGeneratedTables(t, plainOutput)
		assertCLIMigrationVersion(t, database.DB, 2)
		runTestCommand(t, plainOutput, append(os.Environ(), "GOWORK=off"),
			"go", "test", "-mod=mod", "-count=1", "-race", "-cover", "-covermode=atomic", "./...")
	})

	t.Run("symlink parent traversal preserves the output destination", func(t *testing.T) {
		module := newMigrationOutputModule(t)
		child := filepath.Join(module, "child")
		if err := os.Mkdir(child, 0o700); err != nil {
			t.Fatal(err)
		}
		linkParent := t.TempDir()
		link := filepath.Join(linkParent, "link")
		if err := os.Symlink(child, link); err != nil {
			t.Fatal(err)
		}
		unrelatedFile := filepath.Join(linkParent, "output")
		const original = "preserve the unrelated output file\n"
		writeTestFile(t, unrelatedFile, []byte(original))
		// Join would erase the symlink traversal that determines the destination.
		rawOutput := link + "/../output"
		if output, err := runMigrationCLI(t, binary, database.Settings, rawOutput, "", queriesPath); err != nil {
			t.Fatalf("generate through symlink parent: %v\n%s", err, output)
		}
		realOutput := filepath.Join(module, "output")
		assertMigrationGeneratedTables(t, realOutput)
		assertCLIMigrationVersion(t, database.DB, 2)
		content, err := os.ReadFile(unrelatedFile)
		if err != nil || string(content) != original {
			t.Fatalf("generation modified the unrelated output file: content=%q err=%v", content, err)
		}
		runTestCommand(t, module, append(os.Environ(), "GOWORK=off"),
			"go", "test", "-mod=mod", "-count=1", "-race", "-cover", "-covermode=atomic", "./...")
	})

	t.Run("invalid output stops before migrations", func(t *testing.T) {
		path := t.TempDir()
		writeTestFile(t, filepath.Join(path, "0003_pending.sql"), []byte("CREATE TABLE invalid_output_result (id INT PRIMARY KEY);"))
		outputFile := filepath.Join(t.TempDir(), "existing-file")
		const original = "existing output must remain unchanged\n"
		writeTestFile(t, outputFile, []byte(original))
		brokenLink := filepath.Join(t.TempDir(), "broken")
		if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), brokenLink); err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{
			outputFile,
			filepath.Join(outputFile, "nested", "output"),
			brokenLink,
			filepath.Join(brokenLink, "output"),
			// Preserve the raw parent components instead of cleaning them with Join.
			outputFile + "/../output",
			brokenLink + "/../output",
		} {
			output, err := runMigrationCLI(t, binary, database.Settings, target, path, queriesPath)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("expected exit code 1 for invalid output, got %v\n%s", err, output)
			}
			if !strings.Contains(output, errs.ErrOutputPathNotDir.Error()) && !strings.Contains(output, errs.ErrOutputPathInvalid.Error()) {
				t.Fatalf("expected invalid-output error, got:\n%s", output)
			}
			assertCLIMigrationVersion(t, database.DB, 2)
			assertCLIMigrationTable(t, database.DB, "invalid_output_result", false)
			content, err := os.ReadFile(outputFile)
			if err != nil {
				t.Fatalf("read existing output file: %v", err)
			}
			if string(content) != original {
				t.Error("invalid output path modified the existing file")
			}
			for _, cleanedOutput := range []string{
				filepath.Join(filepath.Dir(outputFile), "output"),
				filepath.Join(filepath.Dir(brokenLink), "output"),
			} {
				if _, err := os.Stat(cleanedOutput); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("invalid path created output at %q: %v", cleanedOutput, err)
				}
			}
		}
	})

	for _, test := range []migrationCLIFailureCase{
		{
			name: "SQL failure",
			files: map[string]string{
				"0003_partial.sql": "CREATE TABLE IF NOT EXISTS partial_result (id INT PRIMARY KEY); INVALID SQL;",
				"0004_later.sql":   "CREATE TABLE sql_failure_later (id INT PRIMARY KEY);",
			},
			wantError:    "0003_partial.sql",
			createdTable: "partial_result",
			missingTable: "sql_failure_later",
		},
		{
			name: "gap rejects entire pending batch",
			files: map[string]string{
				"0003_before_gap.sql": "CREATE TABLE gap_before (id INT PRIMARY KEY);",
				"0005_after_gap.sql":  "CREATE TABLE gap_after (id INT PRIMARY KEY);",
			},
			wantError:    errs.ErrVersionGap.Error(),
			missingTable: "gap_before",
		},
		{
			name: "unfinished transaction",
			files: map[string]string{
				"0003_transaction.sql": "START TRANSACTION; UPDATE users SET email = 'uncommitted@example.test' WHERE id = 1;",
				"0004_later.sql":       "CREATE TABLE transaction_later (id INT PRIMARY KEY);",
			},
			wantError:    errs.ErrInvalidSessionState.Error(),
			missingTable: "transaction_later",
		},
		{
			name: "autocommit disabled",
			files: map[string]string{
				"0003_autocommit.sql": "SET autocommit = 0; UPDATE users SET email = 'uncommitted@example.test' WHERE id = 1;",
				"0004_later.sql":      "CREATE TABLE autocommit_later (id INT PRIMARY KEY);",
			},
			wantError:    errs.ErrInvalidSessionState.Error(),
			missingTable: "autocommit_later",
		},
		{
			name: "selected database changed",
			files: map[string]string{
				"0003_database.sql": "USE information_schema;",
				"0004_later.sql":    "CREATE TABLE margo_test.database_later (id INT PRIMARY KEY);",
			},
			wantError:    errs.ErrInvalidSessionState.Error(),
			missingTable: "database_later",
		},
		{
			name: "checkpoint failure",
			files: map[string]string{
				"0003_checkpoint.sql": "CREATE TABLE IF NOT EXISTS checkpoint_result (id INT PRIMARY KEY);",
				"0004_later.sql":      "CREATE TABLE checkpoint_later (id INT PRIMARY KEY);",
			},
			wantError:    "checkpoint rejected",
			createdTable: "checkpoint_result",
			missingTable: "checkpoint_later",
			failVersion:  true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir()
			for name, content := range test.files {
				writeTestFile(t, filepath.Join(path, name), []byte(content))
			}
			if test.failVersion {
				_, err := database.DB.ExecContext(t.Context(), `
CREATE TRIGGER reject_migration_checkpoint
BEFORE UPDATE ON `+migrate.TableName+` FOR EACH ROW
BEGIN
  IF NEW.version > OLD.version THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'checkpoint rejected';
  END IF;
END`)
				if err != nil {
					t.Fatalf("install checkpoint failure trigger: %v", err)
				}
				t.Cleanup(func() {
					if _, err := database.DB.Exec("DROP TRIGGER reject_migration_checkpoint"); err != nil {
						t.Errorf("remove checkpoint failure trigger: %v", err)
					}
				})
			}

			before := snapshotMigrationOutput(t, outputPath)
			absentOutput := filepath.Join(t.TempDir(), "not-created")
			for _, target := range []string{outputPath, absentOutput} {
				output, err := runMigrationCLI(t, binary, database.Settings, target, path, queriesPath)
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
					t.Fatalf("expected exit code 1, got %v\n%s", err, output)
				}
				if !strings.Contains(output, test.wantError) {
					t.Fatalf("expected migration error containing %q, got:\n%s", test.wantError, output)
				}
				if strings.Count(output, "margo failed") != 1 {
					t.Fatalf("expected one terminal error log, got:\n%s", output)
				}
				assertCLIMigrationVersion(t, database.DB, 2)
			}
			if after := snapshotMigrationOutput(t, outputPath); !maps.Equal(before, after) {
				t.Error("migration failure modified existing generated output")
			}
			if _, err := os.Stat(absentOutput); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("migration failure created output directory: %v", err)
			}
			if test.createdTable != "" {
				assertCLIMigrationTable(t, database.DB, test.createdTable, true)
			}
			assertCLIMigrationTable(t, database.DB, test.missingTable, false)
		})
	}
}

func TestMigrationCLIWithoutOutput(t *testing.T) {
	database := StartMariaDB(t)
	binary := filepath.Join(t.TempDir(), "margo")
	runTestCommand(t, margoRepositoryDir(t), append(os.Environ(), "GOWORK=off"),
		"go", "build", "-race", "-o", binary, "./cmd/margo")
	if _, err := database.DB.ExecContext(t.Context(), "DROP DATABASE `margo_test`"); err != nil {
		t.Fatalf("drop disposable fixture database: %v", err)
	}
	if err := database.DB.Close(); err != nil {
		t.Fatalf("close fixture pool before bootstrap: %v", err)
	}

	workingDir := t.TempDir()
	before := snapshotMigrationOutput(t, workingDir)
	migrationsPath := t.TempDir()
	writeTestFile(t, filepath.Join(migrationsPath, "0001_initial.sql"), []byte(`
CREATE TABLE IF NOT EXISTS migration_only (id INT PRIMARY KEY, label VARCHAR(50) NOT NULL);
INSERT INTO migration_only (id, label) VALUES (1, 'initial')
ON DUPLICATE KEY UPDATE label = VALUES(label);
`))
	run := func() (string, error) {
		t.Helper()
		output, err := runMigrationCLIInDir(t, binary, database.Settings, "", migrationsPath, "", workingDir)
		if after := snapshotMigrationOutput(t, workingDir); !maps.Equal(before, after) {
			t.Error("migration without outputPath created files in the working directory")
		}
		return output, err
	}
	runSuccessfully := func() {
		t.Helper()
		output, err := run()
		if err != nil {
			t.Fatalf("migrate without outputPath or go.mod: %v\n%s", err, output)
		}
		if !strings.Contains(output, "margo migrations completed") || strings.Contains(output, "margo generation completed") {
			t.Fatalf("expected migration completion log, got:\n%s", output)
		}
	}
	runSuccessfully()

	var err error
	database.DB, err = sql.Open("mysql", database.DSN)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.DB.Close(); err != nil {
			t.Errorf("close migrated database: %v", err)
		}
	})
	assertCLIMigrationVersion(t, database.DB, 1)
	assertCLIMigrationTable(t, database.DB, "migration_only", true)

	writeTestFile(t, filepath.Join(migrationsPath, "0001_initial.sql"), []byte("invalid completed SQL must not execute"))
	runSuccessfully()
	assertCLIMigrationVersion(t, database.DB, 1)

	writeTestFile(t, filepath.Join(migrationsPath, "0002_next.sql"), []byte(`
INSERT INTO migration_only (id, label) VALUES (2, 'next')
ON DUPLICATE KEY UPDATE label = VALUES(label);
`))
	t.Run("queries require an output path before migrations", func(t *testing.T) {
		output, err := runMigrationCLIInDir(t, binary, database.Settings, "", migrationsPath, t.TempDir(), workingDir)
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			t.Fatalf("expected exit code 1 for queries without outputPath, got %v\n%s", err, output)
		}
		if !strings.Contains(output, errs.ErrQueriesWithoutOutput.Error()) {
			t.Fatalf("expected queries/outputPath validation error, got:\n%s", output)
		}
		assertCLIMigrationVersion(t, database.DB, 1)
		var count int
		if err := database.DB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM migration_only WHERE id = 2").Scan(&count); err != nil {
			t.Fatalf("check pending migration was not applied: %v", err)
		}
		if count != 0 {
			t.Error("invalid queries/outputPath flags applied the pending migration")
		}
		if after := snapshotMigrationOutput(t, workingDir); !maps.Equal(before, after) {
			t.Error("invalid queries/outputPath flags modified the working directory")
		}
	})
	runSuccessfully()
	assertCLIMigrationVersion(t, database.DB, 2)
	var value string
	if err := database.DB.QueryRowContext(t.Context(), "SELECT label FROM migration_only WHERE id = 2").Scan(&value); err != nil {
		t.Fatalf("read newer migration result: %v", err)
	}
	if value != "next" {
		t.Errorf("newer migration value = %q, want next", value)
	}

	writeTestFile(t, filepath.Join(migrationsPath, "0003_broken.sql"), []byte("CREATE TABLE IF NOT EXISTS migration_only_partial (id INT PRIMARY KEY); INVALID SQL;"))
	writeTestFile(t, filepath.Join(migrationsPath, "0004_later.sql"), []byte("CREATE TABLE migration_only_later (id INT PRIMARY KEY);"))
	output, err := run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1 for migration failure, got %v\n%s", err, output)
	}
	if !strings.Contains(output, "0003_broken.sql") || strings.Count(output, "margo failed") != 1 || strings.Contains(output, "completed") {
		t.Fatalf("expected one migration failure log, got:\n%s", output)
	}
	assertCLIMigrationVersion(t, database.DB, 2)
	assertCLIMigrationTable(t, database.DB, "migration_only_partial", true)
	assertCLIMigrationTable(t, database.DB, "migration_only_later", false)
}

func newMigrationOutputModule(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	writeTestFile(t, filepath.Join(path, "go.mod"), []byte(`module github.com/rah-0/margo/tests/integration/migratedtest

go 1.27

require github.com/go-sql-driver/mysql v1.10.1
`))
	return path
}

func runMigrationCLI(t *testing.T, binary string, settings structs.ConnectionOptions, outputPath, migrationsPath, queriesPath string) (string, error) {
	t.Helper()
	return runMigrationCLIInDir(t, binary, settings, outputPath, migrationsPath, queriesPath, "")
}

func runMigrationCLIInDir(t *testing.T, binary string, settings structs.ConnectionOptions, outputPath, migrationsPath, queriesPath, workingDir string) (string, error) {
	t.Helper()
	args := []string{
		"-dbUser=" + settings.User,
		"-dbPassword=" + settings.Password,
		"-dbName=" + settings.Database,
		"-dbIp=" + settings.Host,
		"-dbPort=" + settings.Port,
	}
	if outputPath != "" {
		args = append(args, "-outputPath="+outputPath)
	}
	if migrationsPath != "" {
		args = append(args, "-migrationsPath="+migrationsPath)
	}
	if queriesPath != "" {
		args = append(args, "-queriesPath="+queriesPath)
	}
	command := exec.CommandContext(t.Context(), binary, args...)
	command.Dir = workingDir
	output, err := command.CombinedOutput()
	return string(output), err
}

func assertCLIMigrationVersion(t *testing.T, database *sql.DB, want uint64) {
	t.Helper()
	var version uint64
	if err := database.QueryRowContext(t.Context(), "SELECT version FROM "+migrate.TableName+" WHERE id = 1").Scan(&version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != want {
		t.Fatalf("migration version = %d, want %d", version, want)
	}
}

func assertCLIMigrationTable(t *testing.T, database *sql.DB, name string, want bool) {
	t.Helper()
	var count int
	if err := database.QueryRowContext(t.Context(), `
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?`, name).Scan(&count); err != nil {
		t.Fatalf("check table %q: %v", name, err)
	}
	if (count != 0) != want {
		t.Errorf("table %q exists = %t, want %t", name, count != 0, want)
	}
}

func assertMigrationGeneratedTables(t *testing.T, outputPath string) {
	t.Helper()
	for _, name := range []string{"queries.go", filepath.Join("Users", "entity.go")} {
		if _, err := os.Stat(filepath.Join(outputPath, "MargoTest", name)); err != nil {
			t.Errorf("expected generated %s: %v", name, err)
		}
	}
	for path, content := range snapshotMigrationOutput(t, outputPath) {
		if strings.Contains(path, "MargoSchemaVersion") || strings.Contains(content, migrate.TableName) || strings.Contains(content, "MargoSchemaVersion") {
			t.Errorf("internal version table leaked into generated output %q", path)
		}
	}
}

func snapshotMigrationOutput(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			files[name+"/"] = ""
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[name] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot generated output: %v", err)
	}
	return files
}

const migrationRuntimeTests = `package MargoTest_test

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	generated "github.com/rah-0/margo/tests/integration/migratedtest/MargoTest"
	"github.com/rah-0/margo/tests/integration/migratedtest/MargoTest/Users"
)

func TestMigratedSchemaRuntime(t *testing.T) {
	database, err := sql.Open("mysql", os.Getenv("MARGO_INTEGRATION_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := generated.SetDB(database); err != nil {
		t.Fatal(err)
	}

	seed := (&Users.Entity{Id: "1"}).DBSelect(Users.NewQueryParams().WithWhere(Users.FieldId))
	if seed.Error != nil || len(seed.Entities) != 1 {
		t.Fatalf("read migrated seed: %+v", seed)
	}
	if seed.Entities[0].Email != "initial@example.test" {
		t.Fatalf("migration-added email = %q", seed.Entities[0].Email)
	}

	user := &Users.Entity{Id: "2", Name: "generated", Email: "generated@example.test"}
	if result := user.DBInsert(Users.NewQueryParams().WithInsert(Users.Fields...)); result.Error != nil {
		t.Fatalf("insert through generated entity: %v", result.Error)
	}
	standard := user.DBSelect(Users.NewQueryParams().WithWhere(Users.FieldId))
	if standard.Error != nil || len(standard.Entities) != 1 || *standard.Entities[0] != *user {
		t.Fatalf("standard query returned %+v", standard)
	}
	custom := generated.QueryGetEmail(generated.NewQueryParams().WithParams(user.Id))
	if custom.Error != nil || custom.Entity == nil || custom.Entity.Email != user.Email {
		t.Fatalf("custom query returned %+v", custom)
	}
	mapped := Users.QueryGetUser(Users.NewQueryParams().WithParams(user.Id))
	if mapped.Error != nil || mapped.Entity == nil || *mapped.Entity != *user {
		t.Fatalf("mapped custom query returned %+v", mapped)
	}
}
`
