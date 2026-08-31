//go:build integration || benchmark

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const margoModulePath = "github.com/rah-0/margo"

// GenerateInto runs MarGO against database and writes the generated packages
// beneath outputPath. queriesPath may be empty when no custom queries are needed.
func GenerateInto(t testing.TB, database *MariaDB, outputPath, queriesPath string) {
	t.Helper()

	args := []string{
		"run", ".",
		"-dbUser=" + database.Settings.Username,
		"-dbPassword=" + database.Settings.Password,
		"-dbName=" + database.Settings.Database,
		"-dbIp=" + database.Settings.Host,
		"-dbPort=" + database.Settings.Port,
		"-outputPath=" + outputPath,
	}
	if queriesPath != "" {
		args = append(args, "-queriesPath="+queriesPath)
	}

	runTestCommand(
		t,
		margoRepositoryDir(t),
		append(os.Environ(), "GOWORK=off"),
		"go", args...,
	)
}

func margoRepositoryDir(t testing.TB) string {
	t.Helper()

	_, sourceFile, _, callerOK := runtime.Caller(0)
	if callerOK && filepath.IsAbs(sourceFile) {
		repositoryDir := filepath.Dir(filepath.Dir(sourceFile))
		if isMargoRepository(repositoryDir) {
			return repositoryDir
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve MarGO repository working directory: %v", err)
	}
	for dir := workingDir; ; dir = filepath.Dir(dir) {
		if isMargoRepository(dir) {
			return dir
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}

	t.Fatalf(
		"resolve MarGO repository from source %q or working directory %q",
		sourceFile,
		workingDir,
	)
	return ""
}

func isMargoRepository(dir string) bool {
	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}

	for line := range strings.Lines(string(content)) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")) == margoModulePath
		}
	}
	return false
}

func runTestCommand(t testing.TB, dir string, env []string, name string, args ...string) {
	t.Helper()

	command := exec.CommandContext(t.Context(), name, args...)
	command.Dir = dir
	if env != nil {
		command.Env = env
	}

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", name, err, output)
	}
}
