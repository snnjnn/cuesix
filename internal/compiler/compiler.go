package compiler

import (
	"fmt"
	"io/fs"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/bmatcuk/doublestar/v4"
)

// CompilerError is a custom error type for wrapping underlying errors.
type CompilerError struct {
	Msg string
	Err error
}

func (e *CompilerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("compiler error: %s: %v", e.Msg, e.Err)
	}
	return fmt.Sprintf("compiler error: %s", e.Msg)
}

func (e *CompilerError) Unwrap() error {
	return e.Err
}

// Sentinel errors for the compiler.
const (
	ErrNoFilesFound = CompilerErrorString("no files found to compile")
)

// CompilerErrorString is a simple string-based error type for constant errors.
type CompilerErrorString string

func (e CompilerErrorString) Error() string {
	return string(e)
}

// Source defines a source of CUE/YAML files for the compiler.
type Source struct {
	FS      fs.FS
	Include []string
	Exclude []string
}

// Compile discovers and compiles CUE/YAML files from the given sources.
func Compile(sources ...Source) (cue.Value, []string, error) {
	ctx := cuecontext.New()
	var allFiles []string
	var allValues []cue.Value

	for _, source := range sources {
		files, err := findFiles(source)
		if err != nil {
			return cue.Value{}, nil, &CompilerError{Msg: "failed to find files", Err: err}
		}
		for _, file := range files {
			content, err := fs.ReadFile(source.FS, file)
			if err != nil {
				return cue.Value{}, nil, &CompilerError{Msg: fmt.Sprintf("failed to read file %s", file), Err: err}
			}
			val := ctx.CompileBytes(content, cue.Filename(file))
			if err := val.Err(); err != nil {
				return cue.Value{}, nil, &CompilerError{Msg: fmt.Sprintf("failed to compile file %s", file), Err: err}
			}
			allValues = append(allValues, val)
			allFiles = append(allFiles, file)
		}
	}

	if len(allFiles) == 0 {
		return cue.Value{}, nil, ErrNoFilesFound
	}

	value := ctx.CompileString("")
	for _, v := range allValues {
		value = value.Unify(v)
	}

	if err := value.Err(); err != nil {
		return cue.Value{}, allFiles, &CompilerError{Msg: "failed to unify CUE values", Err: err}
	}

	return value, allFiles, nil
}

func findFiles(source Source) ([]string, error) {
	var files []string
	err := fs.WalkDir(source.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Check against include patterns
		included := false
		for _, pattern := range source.Include {
			match, err := doublestar.Match(pattern, path)
			if err != nil {
				return err // Invalid pattern
			}
			if match {
				included = true
				break
			}
		}

		if !included {
			return nil
		}

		// Check against exclude patterns
		for _, pattern := range source.Exclude {
			match, err := doublestar.Match(pattern, path)
			if err != nil {
				return err // Invalid pattern
			}
			if match {
				return nil // Excluded
			}
		}

		files = append(files, path)
		return nil
	})
	return files, err
}