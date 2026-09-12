package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/internal/cli"
	"github.com/rah-0/margo/runner"
	"github.com/rah-0/slogx"
)

func main() {
	os.Exit(mainExitCode())
}

func mainExitCode() int {
	slogx.SetDefault(slogx.Options{
		Format: slogx.Text,
		Writer: os.Stderr,
	})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return execute(ctx, os.Args[1:], os.Stderr)
}

func execute(ctx context.Context, args []string, output io.Writer) int {
	opts, err := cli.ParseOptions(args, output)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2 // flag.FlagSet has already printed the parsing error and usage.
	}

	active := opts.OutputPath != "" || opts.QueriesPath != "" || opts.MigrationsPath != ""
	if active && opts.Connection.Port == "" {
		// Active CLI operations require a non-empty port.
		err = fmt.Errorf("%w: -dbPort: must not be empty", errs.ErrInvalidConnection)
	} else {
		err = runner.Run(ctx, opts)
	}
	if err != nil {
		slogx.Error("margo failed", err)
		return 1
	}

	switch {
	case !active:
		slog.WarnContext(ctx, "no action requested: specify -outputPath, -queriesPath, or -migrationsPath")
	case opts.OutputPath == "":
		slog.InfoContext(ctx, "margo migrations completed")
	default:
		slog.InfoContext(ctx, "margo generation completed")
	}
	return 0
}
