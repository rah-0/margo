package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rah-0/margo/conf"
)

var (
	tableNames  = []string{"alpha", "beta"}
	tableFields = []conf.TableField{
		{Name: "uuid", DataType: "uuid", ColumnType: "uuid"},
		{Name: "animal", DataType: "varchar", ColumnType: "varchar(255)"},
	}
)

// setupTemplateTest gives each generator test an isolated module and output path.
func setupTemplateTest(t *testing.T) string {
	t.Helper()

	previousArgs := conf.Args
	t.Cleanup(func() {
		conf.Args = previousArgs
	})

	moduleRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(moduleRoot, "go.mod"),
		[]byte("module example.com/margo-template-test\n\ngo 1.27\n"),
		0o600,
	); err != nil {
		t.Fatalf("write temporary go.mod: %v", err)
	}

	outputPath := filepath.Join(moduleRoot, "output")
	conf.Args = conf.Arguments{
		DBName:     "margo_test",
		OutputPath: outputPath,
	}
	return outputPath
}

func assertContextAwareStatementPreparation(t *testing.T, content []byte) {
	t.Helper()

	generated := string(content)
	for _, expected := range []string{
		"func getPreparedStmt(ctx context.Context, query string)",
		"if ctx == nil {\n\t\tctx = context.Background()\n\t}\n\tstmt, err := db.PrepareContext(ctx, query)",
	} {
		if !strings.Contains(generated, expected) {
			t.Errorf("generated code does not contain %q", expected)
		}
	}

	for _, unexpected := range []string{
		"getPreparedStmt(query)",
		"db.Prepare(query)",
	} {
		if strings.Contains(generated, unexpected) {
			t.Errorf("generated code contains context-free preparation %q", unexpected)
		}
	}
}
