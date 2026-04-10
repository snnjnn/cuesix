package compiler_test

import (
	"testing"

	"github.com/warpcomdev/cuesix/internal/compiler"
)

func TestSourceRefHelpers(t *testing.T) {
	t.Parallel()

	ref := compiler.SourceRef{Namespace: "/configs/team-a", Path: "routes/api.yaml"}
	if got := ref.Key(); got == "" {
		t.Fatalf("Key() returned empty string")
	}

	dir := ref.Dir()
	if dir.Namespace != ref.Namespace || dir.Path != "routes" {
		t.Fatalf("Dir() = %#v", dir)
	}

	sibling := ref.Sibling(".env")
	if sibling.Namespace != ref.Namespace || sibling.Path != "routes/.env" {
		t.Fatalf("Sibling() = %#v", sibling)
	}
}
