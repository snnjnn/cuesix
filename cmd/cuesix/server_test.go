package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type trackingReadCloser struct {
	data       *bytes.Buffer
	closed     bool
	readBytes  int
	closeCalls int
}

func (t *trackingReadCloser) Read(p []byte) (int, error) {
	n, err := t.data.Read(p)
	t.readBytes += n
	return n, err
}

func (t *trackingReadCloser) Close() error {
	t.closed = true
	t.closeCalls++
	return nil
}

func TestDrainBodyDrainsAndCloses(t *testing.T) {
	payload := []byte("hello world")
	body := &trackingReadCloser{data: bytes.NewBuffer(payload)}
	req := httptest.NewRequest(http.MethodPost, "/", body)

	handler := drainBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler can read the body
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("handler read error: %v", err)
		}
		if !bytes.Equal(data, payload) {
			t.Fatalf("handler read unexpected body %q", data)
		}
		// Put the same body back so middleware still drains/closes it
		body.data = bytes.NewBuffer(data)
		body.readBytes = 0
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if body.readBytes != len(payload) {
		t.Fatalf("expected body to be fully drained, read %d bytes", body.readBytes)
	}
	if !body.closed || body.closeCalls == 0 {
		t.Fatalf("expected body to be closed, closed=%v calls=%d", body.closed, body.closeCalls)
	}
}
