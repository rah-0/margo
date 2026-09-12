package cli_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/internal/cli"
	"github.com/rah-0/margo/runner"
	"github.com/rah-0/margo/structs"
	testerrs "github.com/rah-0/margo/tests/errs"
)

var cliExecutable string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) (code int) {
	root, err := repositoryRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	directory, err := os.MkdirTemp("", "margo-cli-tests-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(directory); err != nil {
			fmt.Fprintln(os.Stderr, "remove CLI test directory:", err)
			code = 1
		}
	}()
	cliExecutable = filepath.Join(directory, "margo")
	build := exec.Command("go", "build", "-race", "-o", cliExecutable, "./cmd/margo")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build CLI: %v\n%s", err, output)
		return 1
	}
	return m.Run()
}

func repositoryRoot() (string, error) {
	var starts []string
	if directory, err := os.Getwd(); err == nil {
		starts = append(starts, directory)
	}
	if _, source, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(source))
	}
	for _, start := range starts {
		directory, err := filepath.Abs(start)
		if err != nil {
			continue
		}
		for {
			data, err := os.ReadFile(filepath.Join(directory, "go.mod"))
			if err == nil && strings.HasPrefix(string(data), "module github.com/rah-0/margo\n") {
				return directory, nil
			}
			parent := filepath.Dir(directory)
			if parent == directory {
				break
			}
			directory = parent
		}
	}
	return "", testerrs.ErrModuleRootNotFound
}

func TestMainExitsNonZeroOnFailure(t *testing.T) {
	cmd := exec.Command(cliExecutable, "-outputPath=./generated")
	cmd.Dir = t.TempDir()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected process failure, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}

	output := stderr.String()
	if strings.Count(output, "\n") != 1 || !strings.HasSuffix(output, "\n") {
		t.Fatalf("expected one stderr log record, got %q", output)
	}
	for _, want := range []string{
		"level=ERROR",
		`msg="margo failed"`,
		errs.ErrInvalidConnection.Error(),
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected stderr to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "source=") {
		t.Errorf("expected simple output without source location, got %q", output)
	}
}

func TestMainWarnsWithoutPaths(t *testing.T) {
	for _, mode := range []string{"empty", "credentials"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			var args []string
			if mode == "credentials" {
				// This address cannot be used for a database connection. A no-op
				// must return before validating credentials or opening the pool.
				args = []string{"-dbUser=u", "-dbPassword=p", "-dbName=n", "-dbIp=invalid address", "-dbPort="}
			}
			cmd := exec.Command(cliExecutable, args...)
			cmd.Dir = dir
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("expected successful no-op, got %v: %s", err, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("expected empty stdout, got %q", stdout.String())
			}
			output := stderr.String()
			if strings.Count(output, "\n") != 1 || !strings.Contains(output, "level=WARN") {
				t.Fatalf("expected one warning, got %q", output)
			}
			for _, want := range []string{"no action requested", "-outputPath", "-queriesPath", "-migrationsPath"} {
				if !strings.Contains(output, want) {
					t.Errorf("warning missing %q: %s", want, output)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("no-op changed the working directory: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestParseOptionsUsesIndependentFlagSets(t *testing.T) {
	global := flag.CommandLine
	first, err := cli.ParseOptions([]string{
		"-dbUser=first", "-dbPassword=secret", "-dbName=first_db", "-dbIp=127.0.0.1",
		"-dbPort=3307", "-outputPath=first_output", "-queriesPath=first_queries", "-migrationsPath=first_migrations",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if first.Connection.User != "first" || first.Connection.Password != "secret" ||
		first.Connection.Database != "first_db" || first.Connection.Host != "127.0.0.1" ||
		first.Connection.Port != "3307" || first.OutputPath != "first_output" ||
		first.QueriesPath != "first_queries" || first.MigrationsPath != "first_migrations" {
		t.Fatal("flags did not populate options")
	}
	second, err := cli.ParseOptions(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if *second.Connection != (structs.ConnectionOptions{Port: "3306"}) ||
		second.OutputPath != "" || second.QueriesPath != "" || second.MigrationsPath != "" {
		t.Fatal("options retained state from the previous invocation")
	}
	if flag.CommandLine != global || global.Lookup("dbUser") != nil {
		t.Fatal("CLI parsing modified global flags")
	}
	if err := runner.Run(context.Background(), second); err != nil {
		t.Fatalf("public API rejected parsed no-op: %v", err)
	}
}

func TestMainExitCodes(t *testing.T) {
	for _, tt := range []struct {
		mode string
		args []string
		code int
		want string
	}{
		{"help", []string{"-help"}, 0, "Usage of margo:"},
		{"unknown", []string{"-not-a-margo-flag"}, 2, "flag provided but not defined"},
		{"missing-value", []string{"-dbUser"}, 2, "flag needs an argument"},
		{"empty-port", []string{"-outputPath=generated", "-dbPort="}, 1, "-dbPort: must not be empty"},
		{"queries-without-output", []string{"-queriesPath=queries"}, 1, errs.ErrQueriesWithoutOutput.Error()},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			cmd := exec.Command(cliExecutable, tt.args...)
			cmd.Dir = t.TempDir()
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			code := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
				code = exitErr.ExitCode()
			}
			if code != tt.code || !strings.Contains(stderr.String(), tt.want) || stdout.Len() != 0 {
				t.Fatalf("exit=%d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
			}
			if tt.mode == "help" && strings.Contains(stderr.String(), "no action requested") {
				t.Fatal("help printed no-op warning")
			}
		})
	}
}
