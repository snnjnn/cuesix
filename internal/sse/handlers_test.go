package sse_test

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/sse"
)

func decodeEventReply(t *testing.T, line string) sse.Reply {
	t.Helper()
	encoded := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode event payload: %v", err)
	}
	var event sse.Event
	if err := json.Unmarshal(decoded, &event); err != nil {
		t.Fatalf("failed to unmarshal event payload: %v", err)
	}
	reply, err := event.ToReply()
	if err != nil {
		t.Fatalf("failed to convert event to reply: %v", err)
	}
	return reply
}

type applyCall struct {
	virtualgw string
	payload   []byte
}

type recordingReloader struct {
	mu    sync.Mutex
	err   error
	calls []applyCall
	ch    chan applyCall
}

type deadlineTrackingWriter struct {
	header    http.Header
	mu        sync.Mutex
	deadlines []time.Time
	writes    [][]byte
}

func newRecordingReloader() *recordingReloader {
	return &recordingReloader{ch: make(chan applyCall, 32)}
}

func newDeadlineTrackingWriter() *deadlineTrackingWriter {
	return &deadlineTrackingWriter{header: make(http.Header)}
}

func (w *deadlineTrackingWriter) Header() http.Header {
	return w.header
}

func (w *deadlineTrackingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes = append(w.writes, append([]byte(nil), p...))
	w.mu.Unlock()
	return len(p), nil
}

func (w *deadlineTrackingWriter) WriteHeader(_ int) {}

func (w *deadlineTrackingWriter) Flush() {}

func (w *deadlineTrackingWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	w.deadlines = append(w.deadlines, deadline)
	w.mu.Unlock()
	return nil
}

func (r *recordingReloader) Apply(_ context.Context, virtualgw string, payload []byte) error {
	if r.err != nil {
		return r.err
	}
	call := applyCall{virtualgw: virtualgw, payload: append([]byte(nil), payload...)}
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
	select {
	case r.ch <- call:
	default:
	}
	return nil
}

func (r *recordingReloader) waitCall(t *testing.T, timeout time.Duration) applyCall {
	t.Helper()
	call, ok := r.waitCallOK(timeout)
	if !ok {
		t.Fatalf("timeout waiting for Apply call")
	}
	return call
}

func (r *recordingReloader) waitCallOK(timeout time.Duration) (applyCall, bool) {
	select {
	case call := <-r.ch:
		return call, true
	case <-time.After(timeout):
		return applyCall{}, false
	}
}

func TestHandleFullNoContentUntilApply(t *testing.T) {
	t.Parallel()

	nested := newRecordingReloader()
	r := sse.New(nil, nested)
	mux := http.NewServeMux()
	mux.Handle("GET /final/full/{virtualgw}", r.HandleFull())
	req := httptest.NewRequest(http.MethodGet, "/final/full/default", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestApplyAndHandleFullAndConditional(t *testing.T) {
	t.Parallel()

	nested := newRecordingReloader()
	r := sse.New(nil, nested)
	ctx := context.Background()
	payload := []byte(`{"routes":[{"id":"1"}]}`)

	if err := r.Apply(ctx, compiler.DEFAULT_VIRTUALGW, payload); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	call := nested.waitCall(t, time.Second)
	if string(call.payload) != string(payload) {
		t.Fatalf("nested payload mismatch: got %q", string(call.payload))
	}

	mux := http.NewServeMux()
	mux.Handle("GET /final/full/{virtualgw}", r.HandleFull())
	plainReq := httptest.NewRequest(http.MethodGet, "/final/full/default", nil)
	plainRec := httptest.NewRecorder()
	mux.ServeHTTP(plainRec, plainReq)
	if plainRec.Code != http.StatusOK {
		t.Fatalf("plain status = %d", plainRec.Code)
	}
	if got := plainRec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("plain content type = %q", got)
	}
	if got := plainRec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("plain vary = %q", got)
	}
	if got := plainRec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("plain content-encoding = %q", got)
	}
	if got := plainRec.Header().Get("ETag"); got == "" {
		t.Fatalf("missing ETag")
	}
	if got := plainRec.Header().Get("Last-Modified"); got == "" {
		t.Fatalf("missing Last-Modified")
	}

	var fullReply sse.Reply
	if err := json.Unmarshal(plainRec.Body.Bytes(), &fullReply); err != nil {
		t.Fatalf("failed to decode full reply: %v", err)
	}
	if string(fullReply.Data) != string(payload) {
		t.Fatalf("reply data mismatch: got %q", string(fullReply.Data))
	}

	condReq := httptest.NewRequest(http.MethodGet, "/final/full/default", nil)
	condReq.Header.Set("If-None-Match", plainRec.Header().Get("ETag"))
	condRec := httptest.NewRecorder()
	mux.ServeHTTP(condRec, condReq)
	if condRec.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d", condRec.Code)
	}
	if condRec.Body.Len() != 0 {
		t.Fatalf("expected empty body for 304, got %q", condRec.Body.String())
	}
}

func TestHandleFullSupportsVirtualGatewayPath(t *testing.T) {
	t.Parallel()

	nested := newRecordingReloader()
	r := sse.New(nil, nested)
	if err := r.Apply(context.Background(), "secondary", []byte(`{"v":2}`)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /final/full/{virtualgw}", r.HandleFull())
	req := httptest.NewRequest(http.MethodGet, "/final/full/secondary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var fullReply sse.Reply
	if err := json.Unmarshal(rec.Body.Bytes(), &fullReply); err != nil {
		t.Fatalf("failed to decode full reply: %v", err)
	}
	if string(fullReply.Data) != `{"v":2}` {
		t.Fatalf("reply data mismatch: got %q", string(fullReply.Data))
	}
}

func TestHandleSSESeedKeepAliveAndUpdates(t *testing.T) {
	t.Parallel()

	nested := newRecordingReloader()
	r := sse.New(nil, nested)
	if err := r.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("initial Apply() error = %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /final/sse/{virtualgw}", r.HandleSSE(15*time.Millisecond))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/final/sse/default", nil)
	if err != nil {
		t.Fatalf("new request error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request error = %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("cache-control = %q", got)
	}
	if got := resp.Header.Get("Connection"); got != "keep-alive" {
		t.Fatalf("connection = %q", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("x-accel-buffering = %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	seedLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read seed line: %v", err)
	}
	if !strings.HasPrefix(seedLine, "data: ") {
		t.Fatalf("seed line = %q", seedLine)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("failed to read seed separator: %v", err)
	}

	var gotKeepAlive bool
	deadline := time.After(500 * time.Millisecond)
	for !gotKeepAlive {
		select {
		case <-deadline:
			t.Fatalf("did not receive keepalive comment")
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed while reading keepalive: %v", err)
		}
		if strings.HasPrefix(line, ": keepAlive") {
			gotKeepAlive = true
			if _, err := reader.ReadString('\n'); err != nil {
				t.Fatalf("failed to read keepalive separator: %v", err)
			}
		}
	}

	if err := r.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte(`{"v":2}`)); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}
	var gotUpdate bool
	deadline = time.After(time.Second)
	for !gotUpdate {
		select {
		case <-deadline:
			t.Fatalf("did not receive update event")
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed reading update: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			gotUpdate = true
		}
	}
}

func TestHandleSSENoDataUntilNonZeroTimestamp(t *testing.T) {
	t.Parallel()

	nested := newRecordingReloader()
	r := sse.New(nil, nested)
	mux := http.NewServeMux()
	mux.Handle("GET /final/sse/{virtualgw}", r.HandleSSE(15*time.Millisecond))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/final/sse/default", nil)
	if err != nil {
		t.Fatalf("new request error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request error = %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read first line: %v", err)
	}
	if strings.HasPrefix(firstLine, "data: ") {
		t.Fatalf("unexpected data seed line before first apply: %q", firstLine)
	}
	if !strings.HasPrefix(firstLine, ": keepAlive") {
		t.Fatalf("expected keepalive seed line, got %q", firstLine)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("failed to read seed separator: %v", err)
	}

	deadline := time.After(200 * time.Millisecond)
	waitingForApply := true
	for waitingForApply {
		select {
		case <-deadline:
			waitingForApply = false
		default:
		}
		if !waitingForApply {
			break
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed while reading initial stream: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			t.Fatalf("received data line before non-zero timestamp: %q", line)
		}
	}

	if err := r.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("apply() error = %v", err)
	}
	var gotData bool
	deadline = time.After(time.Second)
	for !gotData {
		select {
		case <-deadline:
			t.Fatalf("did not receive data line after apply")
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed while reading stream after apply: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			gotData = true
		}
	}
}

func TestNewAppliesSingleTopicWatcher(t *testing.T) {
	t.Parallel()

	r := sse.New(nil, newRecordingReloader())
	mux := http.NewServeMux()
	mux.Handle("GET /final/sse/{virtualgw}", r.HandleSSE(time.Second))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/final/sse/default", nil)
	if err != nil {
		t.Fatalf("new request error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request error = %v", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("seed line read failed: %v", err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("seed separator read failed: %v", err)
	}

	if err := r.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte(`{"v":3}`)); err != nil {
		t.Fatalf("apply() error = %v", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("event line read failed: %v", err)
	}
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("unexpected event line: %q", line)
	}
	reply := decodeEventReply(t, line)
	if reply.At.IsZero() {
		t.Fatalf("event payload has zero timestamp")
	}
	if string(reply.Data) != `{"v":3}` {
		t.Fatalf("unexpected payload in event: %q", string(reply.Data))
	}
}

func TestHandleSSESupportsVirtualGatewayPath(t *testing.T) {
	t.Parallel()

	r := sse.New(nil, newRecordingReloader())
	if err := r.Apply(context.Background(), "secondary", []byte(`{"v":7}`)); err != nil {
		t.Fatalf("apply() error = %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /final/sse/{virtualgw}", r.HandleSSE(time.Second))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/final/sse/secondary", nil)
	if err != nil {
		t.Fatalf("new request error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request error = %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("seed line read failed: %v", err)
	}
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("unexpected event line: %q", line)
	}
	reply := decodeEventReply(t, line)
	if string(reply.Data) != `{"v":7}` {
		t.Fatalf("unexpected payload in event: %q", string(reply.Data))
	}
}

func TestHandleSSERefreshesWriteDeadline(t *testing.T) {
	t.Parallel()

	r := sse.New(nil, newRecordingReloader())
	const keepAlive = 15 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/final/sse/default", nil).WithContext(ctx)
	writer := newDeadlineTrackingWriter()
	mux := http.NewServeMux()
	mux.Handle("GET /final/sse/{virtualgw}", r.HandleSSE(keepAlive))
	mux.ServeHTTP(writer, req)

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.deadlines) < 3 {
		t.Fatalf("expected at least 3 write deadline refreshes, got %d", len(writer.deadlines))
	}
	for i := 1; i < len(writer.deadlines); i++ {
		if !writer.deadlines[i].After(writer.deadlines[i-1]) {
			t.Fatalf("deadline %d did not move forward: prev=%v current=%v", i, writer.deadlines[i-1], writer.deadlines[i])
		}
		if delta := writer.deadlines[i].Sub(writer.deadlines[i-1]); delta > keepAlive+20*time.Millisecond {
			t.Fatalf("deadline %d jumped too far: delta=%v keepAlive=%v", i, delta, keepAlive)
		}
	}
	if len(writer.writes) < 3 {
		t.Fatalf("expected at least 3 SSE writes, got %d", len(writer.writes))
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
