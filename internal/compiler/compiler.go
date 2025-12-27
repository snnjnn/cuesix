package compiler

import (
	"fmt"
	"io/fs"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/bmatcuk/doublestar/v4"
)

// CompilerError is a custom error type for errors originating from the compiler module.
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

	for _, source := range sources {
		files, err := findFiles(source)
		if err != nil {
			return cue.Value{}, nil, &CompilerError{Msg: "failed to find files", Err: err}
		}
		allFiles = append(allFiles, files...)
	}

	if len(allFiles) == 0 {
		return cue.Value{}, nil, &CompilerError{Msg: "no files found to compile"}
	}

	// For now, just return an empty cue.Value to make the file discovery tests pass.
	// The actual compilation will be implemented in a later task.
	return ctx.CompileString(""), allFiles, nil
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