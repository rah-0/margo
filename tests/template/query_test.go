package template_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/rah-0/margo/query"
	"github.com/rah-0/margo/structs"
)

func TestCreateGoFileQueries(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	if err := renderer.PathCreateDBDir(); err != nil {
		t.Fatal(err)
	}

	tableQueries, err := renderer.CreateGoFileQueries(tableNames, nil)
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

func TestCreateGoFileQueriesPartitionsParsedInput(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	if err := renderer.PathCreateDBDir(); err != nil {
		t.Fatal(err)
	}
	queries := []structs.NamedQuery{
		{Name: "FindAlpha", Query: "SELECT uuid FROM alpha;", Returns: []string{"uuid"}, Mode: "many", MapAs: "alpha"},
		{Name: "FindCount", Query: "SELECT COUNT(uuid) AS count FROM alpha;", Returns: []string{"count"}, Mode: "one"},
		{Name: "FindBeta", Query: "SELECT uuid FROM beta;", Returns: []string{"uuid"}, Mode: "many", MapAs: "beta"},
	}
	for i := range queries {
		queries[i].QueryEncoded = base64.StdEncoding.EncodeToString([]byte(queries[i].Query))
	}
	before := slices.Clone(queries)
	mapped, err := renderer.CreateGoFileQueries(tableNames, queries)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mapped, []structs.NamedQuery{queries[0], queries[2]}) || !reflect.DeepEqual(queries, before) {
		t.Fatalf("rendering changed parsed input or mapped-query order: input=%v mapped=%v", queries, mapped)
	}
	content, err := os.ReadFile(filepath.Join(renderer.OutputPath, "MargoTest", "queries.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "func QueryFindCount(") || strings.Contains(string(content), "func QueryFindAlpha(") || strings.Contains(string(content), "func QueryFindBeta(") {
		t.Fatal("general output must use only the parsed queries without a table mapping")
	}
}

func TestCreateGoFileQueriesPassesContextToNamedQueries(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	if err := renderer.PathCreateDBDir(); err != nil {
		t.Fatal(err)
	}
	queries := []structs.NamedQuery{query.ExtractNamedQuery("-- Returns: uuid\n-- ResultMode: one\nSELECT uuid FROM alpha WHERE uuid = ?\n", "FindAlpha")}

	tableQueries, err := renderer.CreateGoFileQueries(tableNames, queries)
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

func TestCreateGoFileQueriesWithoutReturns(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"one", "many"} {
		t.Run(mode, func(t *testing.T) {
			renderer := setupTemplateTest(t)
			queries := []structs.NamedQuery{query.ExtractNamedQuery("-- ResultMode: "+mode+"\nSELECT 1;\n", "MissingReturns")}
			if err := renderer.PathCreateDBDir(); err != nil {
				t.Fatal(err)
			}
			if _, err := renderer.CreateGoFileQueries(nil, queries); err != nil {
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
	if _, err := renderer.CreateGoFileQueries(nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(renderer.OutputPath, "errs", "errors.go")); err != nil {
		t.Fatalf("empty-schema shared errors file: %v", err)
	}
	testGeneratedPackages(t, renderer.OutputPath)
}
