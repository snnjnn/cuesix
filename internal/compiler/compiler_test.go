package compiler

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
)

func TestCompile_FileDiscovery(t *testing.T) {
	// Create an in-memory filesystem using Afero
	aferoFS := afero.NewMemMapFs()
	_ = aferoFS.MkdirAll("data", 0755)
	_ = aferoFS.MkdirAll("internal", 0755)
	_ = afero.WriteFile(aferoFS, "a.cue", []byte("a: 1"), 0644)
	_ = afero.WriteFile(aferoFS, "b.cue", []byte("b: 2"), 0644)
	_ = afero.WriteFile(aferoFS, "c.yaml", []byte("c: 3"), 0644)
	_ = afero.WriteFile(aferoFS, "data/d.cue", []byte("d: 4"), 0644)
	_ = afero.WriteFile(aferoFS, "data/e.yml", []byte("e: 5"), 0644)
	_ = afero.WriteFile(aferoFS, "ignore_me.cue", []byte("ignore: 6"), 0644)
	_ = afero.WriteFile(aferoFS, "internal/f.cue", []byte("f: 7"), 0644)
	memFS := afero.NewIOFS(aferoFS) // Wrap with IOFS for fs.FS compatibility

	tests := []struct {
		name      string
		sources   []Source
		wantFiles []string
		wantErr   bool
	}{
		{
			name: "Basic include all .cue files",
			sources: []Source{
				{FS: memFS, Include: []string{"*.cue"}},
			},
			wantFiles: []string{"a.cue", "b.cue", "ignore_me.cue"},
		},
		{
			name: "Include specific file",
			sources: []Source{
				{FS: memFS, Include: []string{"a.cue"}},
			},
			wantFiles: []string{"a.cue"},
		},
		{
			name: "Include .cue and .yaml files",
			sources: []Source{
				{FS: memFS, Include: []string{"*.cue", "*.yaml"}},
			},
			wantFiles: []string{"a.cue", "b.cue", "c.yaml", "ignore_me.cue"},
		},
		{
			name: "Include and exclude specific file",
			sources: []Source{
				{FS: memFS, Include: []string{"*.cue"}, Exclude: []string{"b.cue"}},
			},
			wantFiles: []string{"a.cue", "ignore_me.cue"},
		},
		{
			name: "Include all, exclude specific directory",
			sources: []Source{
				{FS: memFS, Include: []string{"**/*.cue"}, Exclude: []string{"internal/*"}},
			},
			wantFiles: []string{"a.cue", "b.cue", "data/d.cue", "ignore_me.cue"},
		},
		{
			name: "Multiple sources, different includes",
			sources: []Source{
				{FS: memFS, Include: []string{"a.cue"}},
				{FS: memFS, Include: []string{"data/*.cue"}},
			},
			wantFiles: []string{"a.cue", "data/d.cue"},
		},
		{
			name: "No matching files",
			sources: []Source{
				{FS: memFS, Include: []string{"*.txt"}},
			},
			wantErr: true, // Expect 'no files found' error
		},
		{
			name: "Empty include patterns",
			sources: []Source{
				{FS: memFS, Include: []string{}},
			},
			wantErr: true, // Expect 'no files found' error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, files, err := Compile(tt.sources...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Compile() error = %v, wantErr %v", err, tt.wantErr)
			}

			sort.Strings(files)
			sort.Strings(tt.wantFiles)

			if !cmp.Equal(files, tt.wantFiles) {
				t.Errorf("Compile() discovered files diff: %s", cmp.Diff(tt.wantFiles, files))
			}
		})
	}
}
