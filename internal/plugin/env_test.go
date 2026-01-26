package plugin_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"iter"

	"github.com/warpcomdev/sixpack/internal/compiler"
	"github.com/warpcomdev/sixpack/internal/plugin"
	"github.com/warpcomdev/sixpack/internal/testutil"
)

type sourceItem struct {
	source compiler.Source
	err    error
}

func sourceSeq(items []sourceItem) iter.Seq2[compiler.Source, error] {
	return func(yield func(compiler.Source, error) bool) {
		for _, item := range items {
			if !yield(item.source, item.err) {
				return
			}
		}
	}
}

func TestEnvSubstituteUsesEnvAndDefaults(t *testing.T) {
	t.Setenv("API_HOST", "env.example")

	source := compiler.Source{
		FS:   fstest.MapFS{},
		Path: "config.yaml",
		Data: []byte("host: ${{ API_HOST }}\nmissing: ${{ MISSING := /default }}\nempty: ${{ MISSING }}\nblank: ${{ := /blank }}\n"),
	}

	var got compiler.Source
	for sourceOut, err := range plugin.EnvEnumerate(testutil.Logger(), "", sourceSeq([]sourceItem{
		{source: source},
	})) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = sourceOut
	}

	want := "host: env.example\nmissing: /default\nempty: \nblank: /blank\n"
	if string(got.Data) != want {
		t.Fatalf("unexpected substitution: %q", string(got.Data))
	}
}

func TestEnvSubstituteEnvFileOverridesAndMissing(t *testing.T) {
	t.Setenv("API_HOST", "base.example")

	fs := fstest.MapFS{
		"configs/.env": {Data: []byte("API_HOST=file.example\nONLY_FILE=file-only\n")},
	}

	got := make(map[string]string)
	for sourceOut, err := range plugin.EnvEnumerate(testutil.Logger(), ".env", sourceSeq([]sourceItem{
		{
			source: compiler.Source{
				FS:   fs,
				Path: "configs/app.yaml",
				Data: []byte("host: ${{ API_HOST }}\nonly: ${{ ONLY_FILE }}\n"),
			},
		},
		{
			source: compiler.Source{
				FS:   fs,
				Path: "other/app.yaml",
				Data: []byte("host: ${{ API_HOST }}\n"),
			},
		},
	})) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got[sourceOut.Path] = string(sourceOut.Data)
	}

	if got["configs/app.yaml"] != "host: file.example\nonly: file-only\n" {
		t.Fatalf("unexpected env file substitution: %q", got["configs/app.yaml"])
	}
	if got["other/app.yaml"] != "host: base.example\n" {
		t.Fatalf("unexpected fallback substitution: %q", got["other/app.yaml"])
	}
}

func TestEnvSubstituteEnvFileParseError(t *testing.T) {
	t.Parallel()

	fs := fstest.MapFS{
		"bad/.env": {Data: []byte("BAD=\"")},
	}
	source := compiler.Source{
		FS:   fs,
		Path: "bad/app.yaml",
		Data: []byte("host: ${{ API_HOST }}\n"),
	}

	var gotErr error
	for _, err := range plugin.EnvEnumerate(testutil.Logger(), ".env", sourceSeq([]sourceItem{
		{source: source},
	})) {
		gotErr = err
		break
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "env file bad/.env") {
		t.Fatalf("expected env file error, got %v", gotErr)
	}
}
