package template_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/template"
)

func TestCreateGoFileQueries(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	if err := renderer.PathCreateDBDir(); err != nil {
		t.Fatal(err)
	}

	tableQueries, err := renderer.CreateGoFileQueries(tableNames)
	if err != nil {
		t.Fatal(err)
	}
	if len(tableQueries) != 0 {
		t.Fatalf("expected no table queries, got %d", len(tableQueries))
	}
	content, err := os.ReadFile(filepath.Join(renderer.OutputPath, "MargoTest", "queries.go"))
	if err != nil {
		t.Fatalf("read generated queries file: %v", err)
	}
	assertContextAwareStatementPreparation(t, content)
}

func TestCreateGoFileQueriesPassesContextToNamedQueries(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	if err := renderer.PathCreateDBDir(); err != nil {
		t.Fatal(err)
	}
	queriesPath := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(queriesPath, "FindAlpha.sql"),
		[]byte("-- Returns: uuid\n-- ResultMode: one\nSELECT uuid FROM alpha WHERE uuid = ?\n"),
		0o600,
	); err != nil {
		t.Fatalf("write named query: %v", err)
	}
	renderer.QueriesPath = queriesPath

	tableQueries, err := renderer.CreateGoFileQueries(tableNames)
	if err != nil {
		t.Fatal(err)
	}
	if len(tableQueries) != 0 {
		t.Fatalf("expected no table queries, got %d", len(tableQueries))
	}
	content, err := os.ReadFile(filepath.Join(renderer.OutputPath, "MargoTest", "queries.go"))
	if err != nil {
		t.Fatalf("read generated queries file: %v", err)
	}
	assertContextAwareStatementPreparation(t, content)
	if !strings.Contains(string(content), "base, err := getPreparedStmt(ctx, q.Query)") {
		t.Error("generated named query does not pass its context to statement preparation")
	}
}

func TestStripSQLComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"line comment at end",
			"SELECT 1; -- comment\nSELECT 2;",
			"SELECT 1; \nSELECT 2;",
		},
		{
			"block comment inline",
			"SELECT /* inline comment */ 1;",
			"SELECT  1;",
		},
		{
			"block comment multiline",
			"SELECT 1; /* comment\nacross lines */ SELECT 2;",
			"SELECT 1;  SELECT 2;",
		},
		{
			"quote with double dash",
			"SELECT '-- not a comment';",
			"SELECT '-- not a comment';",
		},
		{
			"quote with /* block */",
			`SELECT '/* not a comment */';`,
			`SELECT '/* not a comment */';`,
		},
		{
			"nested quotes and comments",
			`SELECT "abc"; -- comment`,
			`SELECT "abc"; `,
		},
		{
			"comment between queries",
			"SELECT 1; -- comment\n-- another\nSELECT 2;",
			"SELECT 1; \n\nSELECT 2;",
		},
		{
			"only comment",
			"-- full comment line\n",
			"\n",
		},
		{
			"no comments",
			"SELECT 1; SELECT 2;",
			"SELECT 1; SELECT 2;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := template.StripSQLComments(tt.input)
			if got != tt.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.expected, got)
			}
		})
	}
}

func TestCheckNoSelectStar_Allowed(t *testing.T) {
	tests := []string{
		"SELECT id, name FROM users",
		"select count(*) from logs",
		"SELECT a.* FROM table a",
		"SELECT\nname\nFROM customers",
	}

	if err := template.CheckNoSelectStar(tests); err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestCheckNoSelectStar_Disallowed(t *testing.T) {
	tests := [][]string{
		{"SELECT * FROM users"},
		{"select\n* from products"},
		{"Select     *     from items"},
	}

	for i, qset := range tests {
		if err := template.CheckNoSelectStar(qset); !errors.Is(err, errs.ErrSelectStarNotAllowed) || !strings.Contains(err.Error(), "index 0") {
			t.Errorf("test %d: expected SELECT * validation error for query 0, got %v", i, err)
		}
	}
}

func TestCreateGoFileQueriesWithoutReturns(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"one", "many"} {
		t.Run(mode, func(t *testing.T) {
			renderer := setupTemplateTest(t)
			renderer.QueriesPath = t.TempDir()
			if err := os.WriteFile(filepath.Join(renderer.QueriesPath, "MissingReturns.sql"), []byte("-- ResultMode: "+mode+"\nSELECT 1;\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := renderer.PathCreateDBDir(); err != nil {
				t.Fatal(err)
			}
			if _, err := renderer.CreateGoFileQueries(nil); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(filepath.Join(renderer.OutputPath, "MargoTest", "queries.go"))
			if err != nil {
				t.Fatal(err)
			}
			want := "named query MissingReturns (ResultMode=" + mode + "): %w"
			if !strings.Contains(string(content), want) {
				t.Fatalf("missing metadata error %q in generated queries", want)
			}
			consumerTest := `package MargoTest_test

import (
	"errors"
	"strings"
	"testing"

	generated "example.com/margo-template-test/output/MargoTest"
	"example.com/margo-template-test/output/errs"
)

func TestMissingReturns(t *testing.T) {
	err := generated.QueryMissingReturns().Error
	if !errors.Is(err, errs.ErrMissingReturns) {
		t.Fatalf("expected shared metadata error, got %v", err)
	}
	if !strings.Contains(err.Error(), "MissingReturns") || !strings.Contains(err.Error(), "ResultMode=` + mode + `") {
		t.Fatalf("missing query context: %v", err)
	}
}
`
			if err := os.WriteFile(filepath.Join(renderer.OutputPath, "MargoTest", "queries_test.go"), []byte(consumerTest), 0o600); err != nil {
				t.Fatal(err)
			}
			testGeneratedPackages(t, renderer.OutputPath)
		})
	}
}

func TestCreateGoFileQueriesEmptySchema(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	if err := renderer.PathCreateDBDir(); err != nil {
		t.Fatal(err)
	}
	if _, err := renderer.CreateGoFileQueries(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(renderer.OutputPath, "errs", "errors.go")); err != nil {
		t.Fatalf("empty-schema shared errors file: %v", err)
	}
	testGeneratedPackages(t, renderer.OutputPath)
}

func TestCreateGoFileQueriesInvalidQueryDoesNotCreateErrors(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	renderer.QueriesPath = t.TempDir()
	if err := os.WriteFile(filepath.Join(renderer.QueriesPath, "Invalid.sql"), []byte("SELECT * FROM alpha;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := renderer.CreateGoFileQueries(nil); !errors.Is(err, errs.ErrSelectStarNotAllowed) {
		t.Fatalf("expected SELECT * validation error, got %v", err)
	}
	if _, err := os.Stat(renderer.OutputPath); !os.IsNotExist(err) {
		t.Fatalf("invalid query created output: %v", err)
	}
}
