package validator

import (
	"io/fs"
	"path/filepath"
)

// Compile-time interface assertions.
var (
	_ fs.FS        = (*filterFS)(nil)
	_ fs.ReadDirFS = (*filterFS)(nil)
)

// filterFS wraps an fs.FS and excludes specified paths from operations.
// It implements fs.FS and fs.ReadDirFS interfaces.
type filterFS struct {
	fs.FS
	exclude map[string]bool
}

// ReadDir reads the directory, unless it belongs to the excluded list
func (f *filterFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if f.exclude[filepath.Base(name)] {
		return nil, nil
	}
	if readdir, ok := f.FS.(fs.ReadDirFS); ok {
		return readdir.ReadDir(name)
	}
	return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
}

// FilterFS wraps an fs.FS and excludes specified folder names from all operations.
// Excluded folders and their descendants will not be visible through Open or ReadDir.
// This is useful with os.CopyFS to skip directories during copying.
//
// Example:
//
//	srcFS := FilterFS(os.DirFS("/source"), "logs", "tmp")
//	os.CopyFS("/dest", srcFS)
func FilterFS(fsys fs.FS, excludePaths ...string) fs.FS {
	exclude := make(map[string]bool, len(excludePaths))
	for _, p := range excludePaths {
		exclude[p] = true
	}
	return &filterFS{FS: fsys, exclude: exclude}
}
