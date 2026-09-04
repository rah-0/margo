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
		os.Args = []string{"margo"}
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
		`msg="margo generation failed"`,
		`err="check flags: missing required arguments:`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected stderr to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "\x1b[") {
		t.Errorf("expected plain text without ANSI escapes, got %q", output)
	}
	if strings.Contains(output, "source=") {
		t.Errorf("expected simple output without source location, got %q", output)
	}
}
