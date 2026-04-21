package db_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	control "github.com/warpcomdev/cuesix/cmd/sixpack/control"
	"github.com/warpcomdev/cuesix/internal/compiler"
	dbinput "github.com/warpcomdev/cuesix/internal/inputs/db"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func mustExec(t *testing.T, db *sqlx.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func openDB(t *testing.T, dsn string) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sqlx.Open() error = %v", err)
	}
	return db
}

func mustCreateTable(t *testing.T, db *sqlx.DB) {
	t.Helper()
	mustExec(t, db, `
		CREATE TABLE snippets (
			namespace TEXT NOT NULL,
			virtualgw TEXT NOT NULL,
			name TEXT NOT NULL,
			content TEXT NOT NULL,
			PRIMARY KEY (namespace, virtualgw, name)
		)
	`)
}

func TestNewInputEnumeratesSnippetsFromSQLite(t *testing.T) {
	t.Parallel()

	db, err := sqlx.Open("sqlite", "file:db-input-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sqlx.Open() error = %v", err)
	}
	defer db.Close()

	mustExec(t, db, `
		CREATE TABLE snippets (
			namespace TEXT NOT NULL,
			virtualgw TEXT NOT NULL,
			name TEXT NOT NULL,
			content TEXT NOT NULL,
			PRIMARY KEY (namespace, virtualgw, name)
		)
	`)
	mustExec(t, db, `
		INSERT INTO snippets(namespace, virtualgw, name, content) VALUES
			('team-a', 'edge.api', 'routes.yaml', 'routes:
  - id: route-1
    uri: /hello'),
			('team-a', 'edge.api', 'upstreams.yaml', 'upstreams:
  - id: upstream-1
    type: roundrobin
    nodes:
      "backend:8080": 1')
	`)

	input := dbinput.NewInput(db)
	resolver := control.GatewayFromDots{
		Resolver: compiler.DefaultResolver{
			VirtualGateway: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
		},
	}
	enumerator := compiler.NewEnumerator(testutil.Logger(), input, resolver)

	got := map[string]compiler.Source{}
	for source, err := range enumerator.Enumerate() {
		if err != nil {
			t.Fatalf("Enumerate() error = %v", err)
		}
		got[source.Ref.Path] = source
	}
	namespaces, err := input.Namespaces()
	if err != nil {
		t.Fatalf("Namespaces() error = %v", err)
	}
	if !slices.Equal(namespaces, []string{"team-a"}) {
		t.Fatalf("Namespaces() = %v", namespaces)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(got))
	}

	route, ok := got["edge.api/routes.yaml"]
	if !ok {
		t.Fatalf("missing routes snippet: %v", maps.Keys(got))
	}
	if route.Virtualgw.Leaf() != "edge.api" {
		t.Fatalf("route virtual gateway = %q", route.Virtualgw.Leaf())
	}
	if !strings.Contains(string(route.Data), "uri: /hello") {
		t.Fatalf("unexpected route content: %q", string(route.Data))
	}

	upstream, ok := got["edge.api/upstreams.yaml"]
	if !ok {
		t.Fatalf("missing upstream snippet: %v", maps.Keys(got))
	}
	if upstream.Virtualgw.Leaf() != "edge.api" {
		t.Fatalf("upstream virtual gateway = %q", upstream.Virtualgw.Leaf())
	}
	if !strings.Contains(string(upstream.Data), "backend:8080") {
		t.Fatalf("unexpected upstream content: %q", string(upstream.Data))
	}
}

func TestNamespaces(t *testing.T) {
	t.Run("expects empty list when no snippets exist", func(t *testing.T) {
		db := openDB(t, "file:db-input-ns-empty?mode=memory&cache=shared")
		defer db.Close()
		mustCreateTable(t, db)

		input := dbinput.NewInput(db)
		got, err := input.Namespaces()
		if err != nil {
			t.Fatalf("Namespaces() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("Namespaces() = %v, want empty list", got)
		}
	})

	t.Run("expects list of unique namespaces", func(t *testing.T) {
		db := openDB(t, "file:db-input-ns-unique?mode=memory&cache=shared")
		defer db.Close()
		mustCreateTable(t, db)

		mustExec(t, db, `
			INSERT INTO snippets(namespace, virtualgw, name, content) VALUES
				('team-a', 'edge.api', 'routes.yaml', '{}'),
				('team-a', 'edge.api', 'upstreams.yaml', '{}'),
				('team-b', 'edge.api', 'routes.yaml', '{}')
		`)
		input := dbinput.NewInput(db)
		got, err := input.Namespaces()
		if err != nil {
			t.Fatalf("Namespaces() error = %v", err)
		}
		want := []string{"team-a", "team-b"}
		if !slices.Equal(got, want) {
			t.Errorf("Namespaces() = %v, want %v", got, want)
		}
	})

	t.Run("expects namespaces order to be ascending", func(t *testing.T) {
		db := openDB(t, "file:db-input-ns-order?mode=memory&cache=shared")
		defer db.Close()
		mustCreateTable(t, db)

		mustExec(t, db, `
			INSERT INTO snippets(namespace, virtualgw, name, content) VALUES
				('team-b', 'edge.api', 'routes.yaml', '{}'),
				('zz-a', 'edge.api', 'routes.yaml', '{}'),
				('team-a', 'edge.api', 'routes.yaml', '{}')
		`)

		input := dbinput.NewInput(db)
		got, err := input.Namespaces()
		if err != nil {
			t.Fatalf("Namespaces() error = %v", err)
		}
		want := []string{"team-a", "team-b", "zz-a"}

		for i := range got {
			if got[i] != want[i] {
				t.Errorf("Namespaces() = %v, want %v", got, want)
				break
			}
		}
	})
}

func TestEnumerate(t *testing.T) {
	t.Parallel()

	t.Run("if namespace do not exist, return empty slice", func(t *testing.T) {
		db := openDB(t, "file:db-input-enum-empty?mode=memory&cache=shared")
		defer db.Close()
		mustCreateTable(t, db)

		input := dbinput.NewInput(db)
		var got []compiler.SourceRef
		for ref := range input.Enumerate("na") {
			got = append(got, ref)
		}
		if len(got) != 0 {
			t.Errorf("Enumerate() = %v, want empty slice", got)
		}
	})
}
