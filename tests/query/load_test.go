package query_test

import (
	"context"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/query"
	"github.com/rah-0/margo/structs"
)

type querySourceCase struct {
	name   string
	source fs.FS
}

type queryRootErrorCase struct {
	name   string
	source fs.FS
	want   error
}

type queryReadFaultCase struct {
	op, path string
}

func TestLoadSources(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{
		"FindCount.SQL":          {Data: []byte("-- Returns: count\n-- ResultMode: one\nSELECT COUNT(uuid) AS count FROM alpha;\n")},
		"FindAlpha.sql":          {Data: []byte("-- Params: uuid\n-- Returns: uuid animal\n-- ResultMode: one\n-- MapAs: alpha\nSELECT uuid, animal FROM alpha WHERE uuid = ?;\n")},
		"README.md":              {Data: []byte("SELECT * FROM ignored;")},
		"nested.sql/Invalid.sql": {Data: []byte("SELECT * FROM ignored;")},
	}
	want := []structs.NamedQuery{
		{Name: "FindAlpha", Query: "SELECT uuid, animal FROM alpha WHERE uuid = ?;", Params: []string{"uuid"}, Returns: []string{"uuid", "animal"}, Mode: "one", MapAs: "alpha"},
		{Name: "FindCount", Query: "SELECT COUNT(uuid) AS count FROM alpha;", Returns: []string{"count"}, Mode: "one"},
	}
	for i := range want {
		want[i].QueryEncoded = base64.StdEncoding.EncodeToString([]byte(want[i].Query))
	}
	opened := &queryFS{source: files}
	nested := fstest.MapFS{}
	for name, file := range files {
		nested["_Queries/"+name] = file
	}
	sub, err := fs.Sub(nested, "_Queries")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []querySourceCase{
		{"disk", os.DirFS(queryFilesOnDisk(t, files))},
		{"filesystem", files},
		{"open only", opened},
		{"subdirectory", sub},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := query.Load(t.Context(), test.source)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Load = %+v, %v; want %+v", got, err, want)
			}
		})
	}
	names := []string{".", "FindAlpha.sql", "FindCount.SQL"}
	if !slices.Equal(opened.opened, names) || !slices.Equal(opened.closed, names) || opened.sourceClosed {
		t.Fatalf("lookup or ownership mismatch: opened=%v closed=%v sourceClosed=%v", opened.opened, opened.closed, opened.sourceClosed)
	}
}

func TestLoadEmptyRoot(t *testing.T) {
	t.Parallel()
	for _, source := range []fs.FS{fstest.MapFS{}, os.DirFS(t.TempDir()), &queryFS{source: fstest.MapFS{}}} {
		queries, err := query.Load(t.Context(), source)
		if err != nil || len(queries) != 0 {
			t.Fatalf("empty root queries = %v, error = %v", queries, err)
		}
	}
}

func TestLoadInvalidRoots(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []queryRootErrorCase{
		{name: "nil", want: fs.ErrInvalid},
		{name: "filesystem file", source: fstest.MapFS{".": {Data: []byte("not a directory")}}},
		{name: "disk file", source: os.DirFS(file)},
		{name: "missing disk root", source: os.DirFS(filepath.Join(dir, "missing")), want: fs.ErrNotExist},
	} {
		t.Run(test.name, func(t *testing.T) {
			queries, err := query.Load(t.Context(), test.source)
			var pathErr *fs.PathError
			if queries != nil || !errors.As(err, &pathErr) || (test.want != nil && !errors.Is(err, test.want)) {
				t.Fatalf("Load = %v, %v; want inspectable root error %v", queries, err, test.want)
			}
		})
	}
}

func TestLoadErrorsStopReading(t *testing.T) {
	t.Parallel()
	for _, stage := range []queryReadFaultCase{
		{"open", "."}, {"readdir", "."}, {"open", "BSecond.sql"}, {"read", "BSecond.sql"},
	} {
		t.Run(stage.op+" "+stage.path, func(t *testing.T) {
			cause := &fs.PathError{Op: stage.op, Path: stage.path, Err: fs.ErrPermission}
			source := &queryFS{source: fstest.MapFS{
				"AFirst.sql":  {Data: []byte("-- Returns: id\nSELECT 1 AS id;")},
				"BSecond.sql": {Data: []byte("-- Returns: id\nSELECT 2 AS id;")},
				"CThird.sql":  {Data: []byte("-- Returns: id\nSELECT 3 AS id;")},
			}, fault: cause}
			queries, err := query.Load(t.Context(), source)
			var pathErr *fs.PathError
			if queries != nil || !errors.Is(err, fs.ErrPermission) || !errors.As(err, &pathErr) || pathErr != cause || !strings.Contains(err.Error(), stage.path) {
				t.Fatalf("Load = %v, %v; want original filesystem cause %v", queries, err, cause)
			}
			if slices.Contains(source.opened, "CThird.sql") || !slices.Equal(source.closed, source.opened) || source.sourceClosed {
				t.Fatalf("read failure continued or left files open: opened=%v closed=%v sourceClosed=%v", source.opened, source.closed, source.sourceClosed)
			}
		})
	}
}

func TestLoadInvalidQuery(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{
		"AFirst.sql":   {Data: []byte("SELECT 1;")},
		"BInvalid.sql": {Data: []byte("SELECT * FROM alpha;")},
		"CThird.sql":   {Data: []byte("SELECT 3;")},
	}
	for _, source := range []fs.FS{files, os.DirFS(queryFilesOnDisk(t, files))} {
		queries, err := query.Load(t.Context(), source)
		if queries != nil || !errors.Is(err, errs.ErrSelectStarNotAllowed) || !strings.Contains(err.Error(), "BInvalid.sql") {
			t.Fatalf("Load = %v, %v; want SELECT * error naming the query", queries, err)
		}
	}
}

func TestLoadCancellationBoundaries(t *testing.T) {
	for _, stage := range []string{"before root", "after root", "after file"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			source := &queryFS{source: fstest.MapFS{"AFirst.sql": {Data: []byte("SELECT 1;")}, "BSecond.sql": {Data: []byte("SELECT 2;")}}}
			if stage == "before root" {
				cancel()
			} else {
				source.afterRead = func(name string) {
					if (stage == "after root" && name == ".") || (stage == "after file" && name == "AFirst.sql") {
						cancel()
					}
				}
			}
			queries, err := query.Load(ctx, source)
			if queries != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("Load = %v, %v; want context.Canceled without partial results", queries, err)
			}
			if (stage == "before root" && source.openCalls != 0) || slices.Contains(source.opened, "BSecond.sql") || !slices.Equal(source.opened, source.closed) {
				t.Fatalf("cancellation crossed a read boundary or left handles open: opened=%v closed=%v", source.opened, source.closed)
			}
		})
	}
}

func TestLoadDiskSymlinkRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(target, "child"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "FindValue.sql"), []byte("SELECT 1 AS value;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "FindValue.sql"), []byte("SELECT * FROM wrong_root;"), 0o600); err != nil {
		t.Fatal(err)
	}
	queries, err := query.Load(t.Context(), os.DirFS(link+string(filepath.Separator)+".."))
	if err != nil || len(queries) != 1 || queries[0].Query != "SELECT 1 AS value;" {
		t.Fatalf("Load = %v, %v; disk root was cleaned before reading", queries, err)
	}
}
