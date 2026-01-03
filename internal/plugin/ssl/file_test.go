package ssl

import (
	"testing"
	"testing/fstest"
)

func TestFileHandlerReplaceTargets(t *testing.T) {
	t.Parallel()
	fallback := Certificate{CertPEM: []byte("fallback-cert"), KeyPEM: []byte("fallback-key")}
	tests := []struct {
		name        string
		filesystems []fstest.MapFS
		target      certTargets
		wantCert    string
		wantKey     string
	}{
		{
			name: "file references resolved",
			filesystems: []fstest.MapFS{
				{"cert.pem": {Data: []byte("cert-data")}, "key.pem": {Data: []byte("key-data")}},
			},
			target: certTargets{
				sslId: "id1",
				cert:  "file://cert.pem",
				key:   "file://key.pem",
				snis:  []string{"example.com"},
			},
			wantCert: "cert-data",
			wantKey:  "key-data",
		},
		{
			name:        "missing files use fallback",
			filesystems: []fstest.MapFS{{}},
			target: certTargets{
				sslId: "id2",
				cert:  "file://missing.pem",
				key:   "file://missing.pem",
			},
			wantCert: string(fallback.CertPEM),
			wantKey:  string(fallback.KeyPEM),
		},
		{
			name: "plain text passthrough",
			filesystems: []fstest.MapFS{
				{},
			},
			target: certTargets{
				sslId: "id3",
				cert:  "cert-inline",
				key:   "key-inline",
			},
			wantCert: "cert-inline",
			wantKey:  "key-inline",
		},
		{
			name:        "empty file reference fallback",
			filesystems: []fstest.MapFS{{}},
			target: certTargets{
				sslId: "id4",
				cert:  "file://",
				key:   "file://",
			},
			wantCert: string(fallback.CertPEM),
			wantKey:  string(fallback.KeyPEM),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := FileHandler{}
			for _, fs := range tt.filesystems {
				handler.Filesystems = append(handler.Filesystems, fs)
			}
			var certOut, keyOut string
			tt.target.replace = func(cert, key []byte) {
				certOut = string(cert)
				keyOut = string(key)
			}
			handler.replaceTargets(nil, []certTargets{tt.target}, fallback)
			if certOut != tt.wantCert || keyOut != tt.wantKey {
				t.Fatalf("unexpected output cert=%q key=%q want cert=%q key=%q", certOut, keyOut, tt.wantCert, tt.wantKey)
			}
		})
	}
}
