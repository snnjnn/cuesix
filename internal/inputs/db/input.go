package db

import (
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"iter"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/warpcomdev/cuesix/internal/compiler"
)

var (
	ErrInvalidSourcePathFormat = errors.New("invalid source ref format, expected '{virtualgw}/{name}'")
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
	var namespaces []string
	// TODO: consider handling errors here
	i.db.Select(&namespaces, "SELECT DISTINCT namespace FROM snippets ORDER BY namespace ASC")
	return namespaces
}

// Enumerate lists all snippets for a namespace as SourceRef values.
func (i *Input) Enumerate(namespace string) iter.Seq2[compiler.SourceRef, error] {
	return func(yield func(compiler.SourceRef, error) bool) {
		rows, err := i.db.Queryx(
			"SELECT virtualgw, name FROM snippets WHERE namespace = ? ORDER BY virtualgw ASC, name ASC", namespace)
		if err != nil {
			yield(compiler.SourceRef{}, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var virtualgw, name string
			if err := rows.Scan(&virtualgw, &name); err != nil {
				yield(compiler.SourceRef{}, err)
				return
			}
			if !yield(compiler.SourceRef{
				Namespace: namespace,
				Path:      virtualgw + "/" + name,
			}, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(compiler.SourceRef{}, err)
			return
		}
	}
}

// Open loads the snippet content identified by the provided SourceRef.
func (i *Input) Open(ref compiler.SourceRef) (io.ReadCloser, error) {
	parts := strings.Split(ref.Path, "/")
	if len(parts) != 2 {
		return nil, ErrInvalidSourcePathFormat
	}
	virtualgw, name := parts[0], parts[1]

	var content string
	err := i.db.Get(&content,
		"SELECT content FROM snippets WHERE namespace = ? AND virtualgw = ? AND name = ?",
		ref.Namespace, virtualgw, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	return io.NopCloser(strings.NewReader(content)), nil
}
