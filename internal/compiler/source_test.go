package compiler_test

import (
	"testing"

	"github.com/warpcondev/cuesix/internal/compiler"
)

func TestSourceRefHelpers(t *testing.T) {
	t.Parallel()

	ref := compiler.SourceRef{Root: "/configs/team-a", Path: "routes/api.yaml"}
	if got := ref.Key(); got == "" {
		t.Fatalf("Key() returned empty string")
	}

	dir := ref.Dir()
	if dir.Root != ref.Root || dir.Path != "routes" {
		t.Fatalf("Dir() = %#v", dir)
	}

	sibling := ref.Sibling(".env")
	if sibling.Root != ref.Root || sibling.Path != "routes/.env" {
		t.Fatalf("Sibling() = %#v", sibling)
	}
}
