package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMainExitsNonZeroOnFailure(t *testing.T) {
	if os.Getenv("MARGO_TEST_MAIN_FAILURE") == "1" {
		flag.CommandLine = flag.NewFlagSet("margo", flag.ContinueOnError)
		flag.CommandLine.SetOutput(io.Discard)
		os.Args = []string{"margo", "-outputPath=./generated"}
		main()
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainExitsNonZeroOnFailure")
	cmd.Env = append(os.Environ(), "MARGO_TEST_MAIN_FAILURE=1")
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
		`err="check flags: missing required arguments:`,
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
	if mode := os.Getenv("MARGO_TEST_MAIN_NO_PATHS"); mode != "" {
		flag.CommandLine = flag.NewFlagSet("margo", flag.ContinueOnError)
		flag.CommandLine.SetOutput(io.Discard)
		os.Args = []string{"margo"}
		if mode == "credentials" {
			// This address cannot be used for a database connection. A no-op must
			// return before validating credentials or opening the pool.
			os.Args = append(os.Args, "-dbUser=u", "-dbPassword=p", "-dbName=n", "-dbIp=invalid address", "-dbPort=")
		}
		main()
		os.Exit(0)
	}

	for _, mode := range []string{"empty", "credentials"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestMainWarnsWithoutPaths$")
			cmd.Env = append(os.Environ(), "MARGO_TEST_MAIN_NO_PATHS="+mode)
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
