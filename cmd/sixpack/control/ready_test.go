package control

import (
	"context"
	"testing"
)

type testReloader struct {
	err error
}

func (r testReloader) Apply(context.Context, string, []byte) error {
	return r.err
}

func TestReadyReloaderApplyHandlesNilReadyMap(t *testing.T) {
	r := ReadyReloader{
		Reloader:        testReloader{},
		maxGatewayDepth: 0,
	}

	if err := r.Apply(context.Background(), "pro-apisix-core", []byte("config")); err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}

	if r.ready == nil {
		t.Fatal("expected ready map to be initialized")
	}
	if !r.ready["pro-apisix-core"] {
		t.Fatal("expected virtual gateway to be marked ready")
	}
	if !r.Ready() {
		t.Fatal("expected readiness probe to report ready")
	}
}

func TestReadyReloaderIgnoresNestedWhenDepthZero(t *testing.T) {
	r := NewReadyReloader(testReloader{}, 0)

	if err := r.Apply(context.Background(), "edge.child", []byte("config")); err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}
	if r.Ready() {
		t.Fatal("expected nested gateway to be ignored for readiness at depth 0")
	}
	if err := r.Apply(context.Background(), "edge", []byte("config")); err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}
	if !r.Ready() {
		t.Fatal("expected top-level gateway to satisfy readiness at depth 0")
	}
}

func TestReadyReloaderAcceptsOneLevelWhenDepthOne(t *testing.T) {
	r := NewReadyReloader(testReloader{}, 1)

	if err := r.Apply(context.Background(), "edge.child", []byte("config")); err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}
	if !r.Ready() {
		t.Fatal("expected one-level nested gateway to satisfy readiness at depth 1")
	}
}
