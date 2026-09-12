package template_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rah-0/margo/structs"
	"github.com/rah-0/margo/template"
)

var (
	tableNames  = []string{"alpha", "beta"}
	tableFields = []structs.TableField{
		{Name: "uuid", DataType: "uuid", ColumnType: "uuid"},
		{Name: "animal", DataType: "varchar", ColumnType: "varchar(255)"},
	}
)

// setupTemplateTest gives each generator test an isolated module and output path.
func setupTemplateTest(t *testing.T) template.Renderer {
	t.Helper()

	moduleRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(moduleRoot, "go.mod"),
		[]byte("module example.com/margo-template-test\n\ngo 1.27\n"),
		0o600,
	); err != nil {
		t.Fatalf("write temporary go.mod: %v", err)
	}

	outputPath := filepath.Join(moduleRoot, "output")
	return template.Renderer{
		DBName:     "margo_test",
		OutputPath: outputPath,
	}
}

func testGeneratedPackages(t *testing.T, directory string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "go", "test", "-count=1", "-race", "-cover", "-covermode=atomic", "./...")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("test generated packages: %v\n%s", err, output)
	}
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
