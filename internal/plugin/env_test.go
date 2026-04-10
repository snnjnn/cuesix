package plugin_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/plugin"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func collectSources(t *testing.T, input compiler.Input) map[string]string {
	t.Helper()

	enumerator := compiler.NewEnumerator(testutil.Logger(), input, compiler.DefaultResolver{
		VirtualGateway: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
	})
	got := make(map[string]string)
	for source, err := range enumerator.Enumerate() {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got[source.Ref.Path] = string(source.Data)
	}
	return got
}

func TestEnvSubstituteUsesEnvAndDefaults(t *testing.T) {
	t.Setenv("API_HOST", "env.example")

	input := compiler.InputFromFS(
		map[string]fs.FS{
			"test": fstest.MapFS{
				"config.yaml": {Data: []byte("host: ${{ API_HOST }}\nmissing: ${{ MISSING := /default }}\nempty: ${{ MISSING }}\nblank: ${{ := /blank }}\n")},
			},
		},
		[]string{"test"},
	)

	wrapped, err := plugin.NewEnvInput(testutil.Logger(), input, "")
	if err != nil {
		t.Fatalf("NewEnvInput() error = %v", err)
	}

	got := collectSources(t, wrapped)
	want := "host: env.example\nmissing: /default\nempty: \nblank: /blank\n"
	if got["config.yaml"] != want {
		t.Fatalf("unexpected substitution: %q", got["config.yaml"])
	}
}

func TestEnvSubstituteEnvFileOverridesAndMissing(t *testing.T) {
	t.Setenv("API_HOST", "base.example")

	input := compiler.InputFromFS(
		map[string]fs.FS{
			"test": fstest.MapFS{
				"configs/.env":     {Data: []byte("API_HOST=file.example\nONLY_FILE=file-only\n")},
				"configs/app.yaml": {Data: []byte("host: ${{ API_HOST }}\nonly: ${{ ONLY_FILE }}\n")},
				"other/app.yaml":   {Data: []byte("host: ${{ API_HOST }}\n")},
			},
		},
		[]string{"test"},
	)

	wrapped, err := plugin.NewEnvInput(testutil.Logger(), input, ".env")
	if err != nil {
		t.Fatalf("NewEnvInput() error = %v", err)
	}

	got := collectSources(t, wrapped)
	if got["configs/app.yaml"] != "host: file.example\nonly: file-only\n" {
		t.Fatalf("unexpected env file substitution: %q", got["configs/app.yaml"])
	}
	if got["other/app.yaml"] != "host: base.example\n" {
		t.Fatalf("unexpected fallback substitution: %q", got["other/app.yaml"])
	}
}

func TestEnvSubstituteEnvFileParseError(t *testing.T) {
	t.Parallel()

	input := compiler.InputFromFS(
		map[string]fs.FS{
			"test": fstest.MapFS{
				"bad/.env":     {Data: []byte("BAD=\"")},
				"bad/app.yaml": {Data: []byte("host: ${{ API_HOST }}\n")},
			},
		},
		[]string{"test"},
	)

	wrapped, err := plugin.NewEnvInput(testutil.Logger(), input, ".env")
	if err != nil {
		t.Fatalf("NewEnvInput() error = %v", err)
	}

	enumerator := compiler.NewEnumerator(testutil.Logger(), wrapped, compiler.DefaultResolver{
		VirtualGateway: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
	})
	var gotErr error
	for _, err := range enumerator.Enumerate() {
		gotErr = err
		break
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "env file bad/.env") {
		t.Fatalf("expected env file error, got %v", gotErr)
	}
}
