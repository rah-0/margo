package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
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
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected process failure, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}
}
