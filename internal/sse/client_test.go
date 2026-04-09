package sse_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/warpcondev/cuesix/internal/compiler"
	"github.com/warpcondev/cuesix/internal/dispatcher"
	"github.com/warpcondev/cuesix/internal/sse"
)

type recordingObserver struct {
	mu   sync.Mutex
	errS []error
}

func (o *recordingObserver) Observe(err error) {
	o.mu.Lock()
	o.errS = append(o.errS, err)
	o.mu.Unlock()
}

func (o *recordingObserver) snapshot() []error {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]error, len(o.errS))
	copy(out, o.errS)
	return out
}

type scriptedBody struct {
	ch     chan []byte
	buf    []byte
	closed bool
}

func newScriptedBody() *scriptedBody {
	return &scriptedBody{ch: make(chan []byte, 16)}
}

func (b *scriptedBody) Read(p []byte) (int, error) {
	for len(b.buf) == 0 {
		next, ok := <-b.ch
		if !ok {
			return 0, io.EOF
		}
		b.buf = next
	}
	n := copy(p, b.buf)
	b.buf = b.buf[n:]
	return n, nil
}

func (b *scriptedBody) Close() error {
	if !b.closed {
		close(b.ch)
		b.closed = true
	}
	return nil
}

func (b *scriptedBody) sendLine(line string) {
	b.ch <- []byte(line + "\n")
}

func (b *scriptedBody) sendBlankLine() {
	b.ch <- []byte("\n")
}

type fakeHTTPClient struct {
	resp *http.Response
	err  error
	req  *http.Request
}

func (c *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.req = req
	if c.err != nil {
		return nil, c.err
	}
	if c.resp != nil {
		c.resp.Request = req
	}
	return c.resp, nil
}

func newTestClient(t *testing.T, body io.ReadCloser, reloader dispatcher.Reloader, readTimeout time.Duration) *sse.Client {
	t.Helper()
	client, err := sse.NewClient(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		reloader,
		compiler.DEFAULT_VIRTUALGW,
		"http://example.com",
		&fakeHTTPClient{
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       body,
			},
		},
		readTimeout,
		func() backoff.BackOff { return backoff.NewConstantBackOff(5 * time.Millisecond) },
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

type blockingReloader struct {
	started chan struct{}
	release chan struct{}
}

func newBlockingReloader() *blockingReloader {
	return &blockingReloader{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

func (r *blockingReloader) Apply(ctx context.Context, _ string, _ []byte) error {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	<-r.release
	return ctx.Err()
}

func encodeEventLine(t *testing.T, payload []byte) string {
	t.Helper()
	reply := sse.Reply{At: time.Now(), Data: payload}
	event, err := reply.ToEvent()
	if err != nil {
		t.Fatalf("Reply.ToEvent() error = %v", err)
	}
	enc, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal(event) error = %v", err)
	}
	return "data: " + base64.StdEncoding.EncodeToString(enc)
}

func waitForCall(t *testing.T, r *recordingReloader) applyCall {
	t.Helper()
	synctest.Wait()
	select {
	case call := <-r.ch:
		return call
	default:
		t.Fatalf("expected Apply call")
		return applyCall{}
	}
}

func assertNoCall(t *testing.T, r *recordingReloader) {
	t.Helper()
	synctest.Wait()
	select {
	case call := <-r.ch:
		t.Fatalf("unexpected Apply call: got=%q", string(call.payload))
	default:
	}
}

func waitForObservedError(t *testing.T, obs *recordingObserver, timeout time.Duration, want func(error) bool) error {
	t.Helper()
	time.Sleep(timeout)
	synctest.Wait()
	for _, err := range obs.snapshot() {
		if want(err) {
			return err
		}
	}
	t.Fatalf("expected matching observed error, got %v", obs.snapshot())
	return nil
}

func TestNewClientValidation(t *testing.T) {
	t.Parallel()

	nested := newRecordingReloader()
	factory := func() backoff.BackOff { return backoff.NewConstantBackOff(5 * time.Millisecond) }
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	httpClient := &fakeHTTPClient{}

	if _, err := sse.NewClient(logger, nested, compiler.DEFAULT_VIRTUALGW, "http://example.com", httpClient, 0, factory); !errors.Is(err, sse.ErrConfigTimeouts) {
		t.Fatalf("expected ErrConfigTimeouts, got %v", err)
	}
	if _, err := sse.NewClient(logger, nested, compiler.DEFAULT_VIRTUALGW, "http://example.com", nil, time.Second, factory); !errors.Is(err, sse.ErrHttpClient) {
		t.Fatalf("expected ErrHttpClient, got %v", err)
	}
	if _, err := sse.NewClient(logger, nested, compiler.DEFAULT_VIRTUALGW, "http://example.com", httpClient, time.Second, nil); !errors.Is(err, sse.ErrConfigFactory) {
		t.Fatalf("expected ErrConfigFactory, got %v", err)
	}
	if _, err := sse.NewClient(logger, nested, compiler.DEFAULT_VIRTUALGW, "http://[::1", httpClient, time.Second, factory); err == nil {
		t.Fatalf("expected url join error")
	}
	if _, err := sse.NewClient(logger, nested, "", "http://example.com", httpClient, time.Second, factory); !errors.Is(err, sse.ErrVirtualGateway) {
		t.Fatalf("expected ErrVirtualGateway, got %v", err)
	}
}

func TestClientRequestsConfiguredVirtualGatewayPath(t *testing.T) {
	client, err := sse.NewClient(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		newRecordingReloader(),
		"secondary",
		"http://example.com/base",
		&fakeHTTPClient{},
		300*time.Millisecond,
		func() backoff.BackOff { return backoff.NewConstantBackOff(5 * time.Millisecond) },
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	got := client.SSEURL()
	if got != "http://example.com/base/final/sse/secondary" {
		t.Fatalf("sseURL = %q", got)
	}
}

func TestClientLoopAppliesPayloadAndKeepsCurrent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		body := newScriptedBody()
		clientNested := newRecordingReloader()
		client := newTestClient(t, body, clientNested, 300*time.Millisecond)
		obs := &recordingObserver{}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go client.Loop(ctx, obs)

		targetPayload := []byte(`{"upstream":"v1"}`)
		body.sendLine(encodeEventLine(t, targetPayload))
		body.sendBlankLine()

		call := waitForCall(t, clientNested)
		if string(call.payload) != string(targetPayload) {
			t.Fatalf("client reloader payload mismatch: got=%q want=%q", string(call.payload), string(targetPayload))
		}

		current := client.Current()
		if string(current.Data) != string(targetPayload) {
			t.Fatalf("client Current().Data mismatch: got=%q want=%q", string(current.Data), string(targetPayload))
		}
		if current.At.IsZero() {
			t.Fatalf("client Current().At should not be zero")
		}
	})
}

func TestClientUsesObserverForReadTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		body := newScriptedBody()
		const readTimeout = 30 * time.Millisecond
		client := newTestClient(t, body, newRecordingReloader(), readTimeout)
		obs := &recordingObserver{}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go client.Loop(ctx, obs)

		_ = waitForObservedError(t, obs, 2*readTimeout, func(err error) bool {
			return errors.Is(err, sse.ErrReadTimeout)
		})
	})
}

func TestClientSkipsRepeatedSSEEventWithSameTimestamp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		body := newScriptedBody()
		clientNested := newRecordingReloader()
		client := newTestClient(t, body, clientNested, 400*time.Millisecond)
		obs := &recordingObserver{}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go client.Loop(ctx, obs)

		payload := []byte(`{"v":42}`)
		reply := sse.Reply{At: time.Now(), Data: payload}
		event, err := reply.ToEvent()
		if err != nil {
			t.Fatalf("Reply.ToEvent() error = %v", err)
		}
		enc, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("json.Marshal(event) error = %v", err)
		}
		line := "data: " + base64.StdEncoding.EncodeToString(enc)

		body.sendLine(line)
		body.sendBlankLine()
		firstCall := waitForCall(t, clientNested)
		if string(firstCall.payload) != string(payload) {
			t.Fatalf("first payload mismatch: got=%q", string(firstCall.payload))
		}

		firstAt := client.Current().At
		if firstAt.IsZero() {
			t.Fatalf("expected current.At after first reload")
		}

		body.sendLine(line)
		body.sendBlankLine()
		assertNoCall(t, clientNested)
		if client.Current().At != firstAt {
			t.Fatalf("expected At to remain %s after repeated SSE event, got %s", firstAt.Format(time.RFC3339Nano), client.Current().At.Format(time.RFC3339Nano))
		}
	})
}

func TestClientCancelDuringInflightReloadExitsCleanly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		body := newScriptedBody()
		clientNested := newBlockingReloader()
		client := newTestClient(t, body, clientNested, 300*time.Millisecond)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			client.Loop(ctx, nil)
			close(done)
		}()

		body.sendLine(encodeEventLine(t, []byte(`{"upstream":"v1"}`)))
		body.sendBlankLine()
		synctest.Wait()
		select {
		case <-clientNested.started:
		default:
			t.Fatalf("expected in-flight reload before cancellation")
		}

		cancel()
		close(clientNested.release)
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatalf("client loop did not exit after context cancellation")
		}
	})
}
