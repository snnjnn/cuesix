package db

import (
	"io"
	"io/fs"
	"iter"

	"github.com/jmoiron/sqlx"
	"github.com/warpcomdev/cuesix/internal/compiler"
)

// Input exposes database-backed snippets through compiler.Input.
//
// Expected schema:
//
//	CREATE TABLE snippets (
//	  namespace TEXT NOT NULL,
//	  virtualgw TEXT NOT NULL,
//	  name TEXT NOT NULL,
//	  content TEXT NOT NULL,
//	  PRIMARY KEY (namespace, virtualgw, name)
//	);
//
// The resulting SourceRef must use:
//   - Namespace = namespace
//   - Path = "{virtualgw}/{name}"
type Input struct {
	db *sqlx.DB
}

// NewInput builds a database-backed compiler.Input.
//
// TODO: consider nil verification for db
func NewInput(db *sqlx.DB) compiler.Input {
	return &Input{db: db}
}

// Namespaces returns the list of available namespaces.
func (i *Input) Namespaces() []string {
	return nil
}

// Enumerate lists all snippets for a namespace as SourceRef values.
func (i *Input) Enumerate(namespace string) iter.Seq2[compiler.SourceRef, error] {
	return func(yield func(compiler.SourceRef, error) bool) {
	}
}

// Open loads the snippet content identified by the provided SourceRef.
func (i *Input) Open(ref compiler.SourceRef) (io.ReadCloser, error) {
	return nil, fs.ErrNotExist
}
