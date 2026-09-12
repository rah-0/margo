package migrate_test

import (
	"errors"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/rah-0/margo/migrate"
)

// Hide MapFS's ReadDir and ReadFile methods so the standard helpers must Open.
type openOnlyFS struct {
	source       fs.FS
	openErr      error
	readDirErr   error
	afterReadDir func()
	opened       []string
	closed       []string
}

func (s *openOnlyFS) Open(name string) (fs.File, error) {
	s.opened = append(s.opened, name)
	if s.openErr != nil {
		return nil, s.openErr
	}
	file, err := s.source.Open(name)
	if err != nil {
		return nil, err
	}
	tracked := &trackedFile{File: file, source: s, name: name}
	if dir, ok := file.(fs.ReadDirFile); ok {
		return &trackedDirectory{trackedFile: tracked, directory: dir}, nil
	}
	return tracked, nil
}

type trackedFile struct {
	fs.File
	source *openOnlyFS
	name   string
}

func (f *trackedFile) Close() error {
	f.source.closed = append(f.source.closed, f.name)
	return f.File.Close()
}

type trackedDirectory struct {
	*trackedFile
	directory fs.ReadDirFile
}

func (f *trackedDirectory) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.source.readDirErr != nil {
		return nil, f.source.readDirErr
	}
	entries, err := f.directory.ReadDir(n)
	if f.source.afterReadDir != nil {
		f.source.afterReadDir()
	}
	return entries, err
}

func TestDiscoverOpenFallback(t *testing.T) {
	source := &openOnlyFS{source: fstest.MapFS{
		"0002_second.sql": {Data: []byte("DO 2;")},
		"0001_first.sql":  {Data: []byte("DO 1;")},
		"nested/bad.sql":  {},
	}}
	migrations, err := migrate.Discover(source)
	want := []migrate.Migration{{Version: 1, Path: "0001_first.sql"}, {Version: 2, Path: "0002_second.sql"}}
	if err != nil || !slices.Equal(migrations, want) {
		t.Fatalf("Discover = %v, %v; want %v", migrations, err, want)
	}
	if !slices.Equal(source.opened, []string{"."}) || !slices.Equal(source.closed, source.opened) {
		t.Fatalf("discovery must read only the root and close its handle: opened=%v, closed=%v", source.opened, source.closed)
	}
}

func TestDiscoverNil(t *testing.T) {
	if _, err := migrate.Discover(nil); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("Discover(nil) error = %v, want fs.ErrInvalid", err)
	}
}

func TestDiscoverInvalidRoot(t *testing.T) {
	_, err := migrate.Discover(fstest.MapFS{".": {Data: []byte("not a directory")}})
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != "." {
		t.Fatalf("Discover error = %v, want inspectable root filesystem error", err)
	}
}

func TestDiscoverEnumerationErrors(t *testing.T) {
	for _, stage := range []string{"open", "readdir"} {
		t.Run(stage, func(t *testing.T) {
			cause := &fs.PathError{Op: stage, Path: ".", Err: fs.ErrPermission}
			source := &openOnlyFS{source: fstest.MapFS{"0001_first.sql": {}}}
			if stage == "open" {
				source.openErr = cause
			} else {
				source.readDirErr = cause
			}
			_, err := migrate.Discover(source)
			var pathErr *fs.PathError
			if !errors.Is(err, fs.ErrPermission) || !errors.As(err, &pathErr) || pathErr != cause {
				t.Fatalf("Discover error = %v, want original filesystem error %v", err, cause)
			}
			if stage == "readdir" && !slices.Equal(source.closed, []string{"."}) {
				t.Fatalf("failed enumeration left directory open: closed=%v", source.closed)
			}
		})
	}
}
