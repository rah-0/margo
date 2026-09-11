package conf

import (
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetFlags(t *testing.T) {
	t.Helper()
	origArgs := os.Args
	origCmd := flag.CommandLine
	origConf := Args
	t.Cleanup(func() {
		os.Args = origArgs
		flag.CommandLine = origCmd
		Args = origConf
	})
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
}

func validArgs(extra ...string) []string {
	base := []string{
		"margo",
		"-dbUser", "u",
		"-dbPassword", "p",
		"-dbName", "n",
		"-dbIp", "127.0.0.1",
		"-dbPort", "3306",
		"-outputPath", "/tmp",
	}
	return append(base, extra...)
}

func TestCheckFlagsMissingArgs(t *testing.T) {
	resetFlags(t)
	os.Args = []string{"margo", "-outputPath", t.TempDir()}
	if err := CheckFlags(); !errors.Is(err, ErrMissingArgs) {
		t.Fatalf("expected ErrMissingArgs, got %v", err)
	}
}

func TestCheckFlagsWithoutPaths(t *testing.T) {
	resetFlags(t)
	Args.OutputPath = "previous"
	Args.QueriesPath = "previous"
	Args.MigrationsPath = "previous"
	os.Args = []string{"margo"}
	if err := CheckFlags(); err != nil {
		t.Fatalf("no action should require no database arguments: %v", err)
	}
	if Args.OutputPath != "" || Args.QueriesPath != "" || Args.MigrationsPath != "" {
		t.Fatalf("previous paths were retained: %+v", Args)
	}
}

func TestCheckFlagsMigrationsWithoutOutput(t *testing.T) {
	resetFlags(t)
	path := t.TempDir()
	os.Args = validArgs("-outputPath=", "-migrationsPath", path)
	if err := CheckFlags(); err != nil {
		t.Fatalf("migration-only flags rejected: %v", err)
	}
	if Args.OutputPath != "" || Args.MigrationsPath != path {
		t.Fatalf("unexpected paths: output=%q migrations=%q", Args.OutputPath, Args.MigrationsPath)
	}
}

func TestCheckFlagsQueriesRequireOutput(t *testing.T) {
	for _, withMigrations := range []bool{false, true} {
		name := "queries"
		if withMigrations {
			name = "queries and migrations"
		}
		t.Run(name, func(t *testing.T) {
			resetFlags(t)
			os.Args = validArgs("-outputPath=", "-queriesPath", t.TempDir())
			if withMigrations {
				os.Args = append(os.Args, "-migrationsPath", t.TempDir())
			}
			err := CheckFlags()
			if !errors.Is(err, ErrMissingArgs) || !strings.Contains(err.Error(), "-outputPath is required when -queriesPath is set") {
				t.Fatalf("expected output requirement for custom queries, got %v", err)
			}
		})
	}
}

func TestCheckFlagsQueriesPathInvalid(t *testing.T) {
	resetFlags(t)
	os.Args = validArgs("-queriesPath", filepath.Join(t.TempDir(), "does-not-exist"))
	err := CheckFlags()
	if !errors.Is(err, ErrQueriesPathInvalid) {
		t.Fatalf("expected ErrQueriesPathInvalid, got %v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestCheckFlagsQueriesPathNotDir(t *testing.T) {
	resetFlags(t)
	file := filepath.Join(t.TempDir(), "queries.sql")
	if err := os.WriteFile(file, []byte("-- noop"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Args = validArgs("-queriesPath", file)
	if err := CheckFlags(); !errors.Is(err, ErrQueriesPathNotDir) {
		t.Fatalf("expected ErrQueriesPathNotDir, got %v", err)
	}
}

func TestCheckFlagsValid(t *testing.T) {
	resetFlags(t)
	queriesPath := t.TempDir()
	migrationsPath := t.TempDir()
	os.Args = validArgs("-queriesPath", queriesPath, "-migrationsPath", migrationsPath)
	if err := CheckFlags(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if Args.QueriesPath != queriesPath || Args.MigrationsPath != migrationsPath {
		t.Fatalf("unexpected paths: queries=%q migrations=%q", Args.QueriesPath, Args.MigrationsPath)
	}
}

func TestCheckFlagsMigrationsPathInvalid(t *testing.T) {
	resetFlags(t)
	os.Args = validArgs("-migrationsPath", filepath.Join(t.TempDir(), "missing"))
	err := CheckFlags()
	if !errors.Is(err, ErrMigrationsPathInvalid) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected invalid migration path with missing-file cause, got %v", err)
	}
}

func TestCheckFlagsMigrationsPathNotDir(t *testing.T) {
	resetFlags(t)
	path := filepath.Join(t.TempDir(), "0001_users.sql")
	if err := os.WriteFile(path, []byte("DO 0;"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Args = validArgs("-migrationsPath", path)
	if err := CheckFlags(); !errors.Is(err, ErrMigrationsPathNotDir) {
		t.Fatalf("expected ErrMigrationsPathNotDir, got %v", err)
	}
}

func TestCheckFlagsWithoutMigrations(t *testing.T) {
	resetFlags(t)
	Args.MigrationsPath = "previous"
	os.Args = validArgs()
	if err := CheckFlags(); err != nil {
		t.Fatal(err)
	}
	if Args.MigrationsPath != "" {
		t.Fatalf("migrations should be disabled, got path %q", Args.MigrationsPath)
	}
}

func TestCheckFlagsOutputPathNotDir(t *testing.T) {
	resetFlags(t)
	path := filepath.Join(t.TempDir(), "output")
	if err := os.WriteFile(path, []byte("preserve this file"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Args = validArgs("-outputPath", path)
	if err := CheckFlags(); !errors.Is(err, ErrOutputPathNotDir) {
		t.Fatalf("expected ErrOutputPathNotDir, got %v", err)
	}
}

func TestCheckFlagsOutputPathParentNotDir(t *testing.T) {
	resetFlags(t)
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	os.Args = validArgs("-outputPath", filepath.Join(parent, "generated"))
	if err := CheckFlags(); !errors.Is(err, ErrOutputPathInvalid) && !errors.Is(err, ErrOutputPathNotDir) {
		t.Fatalf("expected invalid output path, got %v", err)
	}
}

func TestCheckFlagsMissingOutputPathIsNotCreated(t *testing.T) {
	resetFlags(t)
	path := filepath.Join(t.TempDir(), "missing", "generated")
	os.Args = validArgs("-outputPath", path)
	if err := CheckFlags(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("flag validation created output directories: %v", err)
	}
}

func TestCheckFlagsOutputSymlinks(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), broken); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(dir, "valid")
	if err := os.Symlink(t.TempDir(), valid); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path    string
		invalid bool
	}{
		{path: broken, invalid: true},
		{path: filepath.Join(broken, "generated"), invalid: true},
		{path: valid},
		{path: filepath.Join(valid, "generated")},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resetFlags(t)
			os.Args = validArgs("-outputPath", tc.path)
			err := CheckFlags()
			if tc.invalid {
				if !errors.Is(err, ErrOutputPathInvalid) {
					t.Fatalf("expected ErrOutputPathInvalid, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("valid directory symlink rejected: %v", err)
			}
		})
	}
}

func TestCheckFlagsOutputParentTraversal(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), broken); err != nil {
		t.Fatal(err)
	}
	for _, parent := range []string{file, broken} {
		t.Run(filepath.Base(parent), func(t *testing.T) {
			resetFlags(t)
			// Join would clean away the component this regression must validate.
			path := parent + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "output"
			os.Args = validArgs("-outputPath", path)
			if err := CheckFlags(); !errors.Is(err, ErrOutputPathInvalid) {
				t.Fatalf("expected invalid raw output path, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "output")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("validation created output: %v", err)
			}
		})
	}
}

func TestCheckFlagsInvalidPreservesConfiguration(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "credentials", args: validArgs("-dbUser=")},
		{name: "queries require output", args: validArgs("-outputPath=", "-queriesPath", dir)},
		{name: "output", args: validArgs("-outputPath", file)},
		{name: "queries", args: validArgs("-queriesPath", missing)},
		{name: "migrations", args: validArgs("-migrationsPath", missing)},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetFlags(t)
			previous := Arguments{DBName: "previous", OutputPath: "previous-output"}
			Args = previous
			os.Args = test.args
			if err := CheckFlags(); err == nil {
				t.Fatal("expected invalid arguments")
			}
			if Args != previous {
				t.Fatal("invalid arguments replaced the previous configuration")
			}
		})
	}
}

func TestCheckFlagsOutputSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	child := filepath.Join(target, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(child, link); err != nil {
		t.Fatal(err)
	}
	// Cleaning link/../output would resolve to this file, but the supplied path
	// actually points to a missing directory beneath target.
	if err := os.WriteFile(filepath.Join(dir, "output"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	separator := string(os.PathSeparator)
	path := link + separator + ".." + separator + "output"
	for _, suffix := range []string{"", separator + "nested", separator + separator} {
		t.Run("suffix="+suffix, func(t *testing.T) {
			resetFlags(t)
			os.Args = validArgs("-outputPath", path+suffix)
			if err := CheckFlags(); err != nil {
				t.Fatalf("valid symlink-parent output rejected: %v", err)
			}
			if Args.OutputPath != path+suffix {
				t.Fatal("validation changed the supplied output path")
			}
			if _, err := os.Stat(filepath.Join(target, "output")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("validation created output: %v", err)
			}
		})
	}
}
