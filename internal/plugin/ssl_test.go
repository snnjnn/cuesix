package plugin

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

func TestSSLPluginReplacesFields(t *testing.T) {
	fsOne := fstest.MapFS{
		"cert.pem":  {Data: []byte("cert-data")},
		"key.pem":   {Data: []byte("key-data")},
		"chain.pem": {Data: []byte("chain-data")},
		"client.pem": {Data: []byte("client-data")},
	}
	plugin := &SSLPlugin{Filesystems: []fs.FS{fsOne}}

	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"id":    "1",
				"cert":  "file://cert.pem",
				"key":   "file://key.pem",
				"certs": []any{"file://chain.pem"},
				"keys":  []any{"inline-key"},
				"client": map[string]any{
					"ca": "file://client.pem",
				},
			},
		},
	}

	got, err := plugin.Update(input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	want := map[string]any{
		"ssls": []any{
			map[string]any{
				"id":    "1",
				"cert":  "cert-data",
				"key":   "key-data",
				"certs": []any{"chain-data"},
				"keys":  []any{"inline-key"},
				"client": map[string]any{
					"ca": "client-data",
				},
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected result (-want +got):\n%s", diff)
	}
}

func TestSSLPluginMissingFile(t *testing.T) {
	plugin := &SSLPlugin{Filesystems: []fs.FS{fstest.MapFS{}}}
	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"cert": "file://missing.pem",
			},
		},
	}
	if _, err := plugin.Update(input); err == nil {
		t.Fatalf("expected error for missing file")
	}
}
