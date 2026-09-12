package query_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func queryFilesOnDisk(t *testing.T, files fstest.MapFS) string {
	t.Helper()
	dir := t.TempDir()
	for name, file := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, file.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// queryFS hides optional filesystem helpers so reads must use Open and close files.
type queryFS struct {
	source       fs.FS
	fault        *fs.PathError
	opened       []string
	closed       []string
	openCalls    int
	afterRead    func(string)
	sourceClosed bool
}

func (s *queryFS) Open(name string) (fs.File, error) {
	s.openCalls++
	if s.fault != nil && s.fault.Op == "open" && s.fault.Path == name {
		return nil, s.fault
	}
	file, err := s.source.Open(name)
	if err != nil {
		return nil, err
	}
	s.opened = append(s.opened, name)
	tracked := &queryFile{File: file, source: s, name: name}
	if dir, ok := file.(fs.ReadDirFile); ok {
		return &queryDirectory{queryFile: tracked, directory: dir}, nil
	}
	return tracked, nil
}

func (s *queryFS) Close() error { s.sourceClosed = true; return nil }

type queryFile struct {
	fs.File
	source *queryFS
	name   string
}

func (f *queryFile) Read(p []byte) (int, error) {
	if cause := f.source.fault; cause != nil && cause.Op == "read" && cause.Path == f.name {
		return 0, cause
	}
	n, err := f.File.Read(p)
	if f.source.afterRead != nil {
		f.source.afterRead(f.name)
	}
	return n, err
}

func (f *queryFile) Close() error {
	f.source.closed = append(f.source.closed, f.name)
	return f.File.Close()
}

type queryDirectory struct {
	*queryFile
	directory fs.ReadDirFile
}

func (f *queryDirectory) ReadDir(n int) ([]fs.DirEntry, error) {
	if cause := f.source.fault; cause != nil && cause.Op == "readdir" && cause.Path == f.name {
		return nil, cause
	}
	entries, err := f.directory.ReadDir(n)
	if f.source.afterRead != nil {
		f.source.afterRead(f.name)
	}
	return entries, err
}
