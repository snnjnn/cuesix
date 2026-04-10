package sse

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
)

// Custom errors of this module
type StaticError string
type ContenTypeError string
type StatusCodeError int

func (err StaticError) Error() string {
	return string(err)
}

func (err ContenTypeError) Error() string {
	return fmt.Sprintf("unsupported HTTP content type: %s", string(err))
}

func (err StatusCodeError) Error() string {
	return fmt.Sprintf("unexpected HTTP response code: %d", int(err))
}

// Observer lets an external entity observe internal errors
// which are usually dismissed by the client.
type Observer interface {
	Observe(err error)
}

const (
	ErrConfigTimeouts   StaticError = "timeouts must be > 0"
	ErrConfigFactory    StaticError = "backoff factory cannot be nil"
	ErrVirtualGateway   StaticError = "virtual gateway cannot be empty"
	ErrHttpClient       StaticError = "HTTP client cannot be nil"
	ErrEventChannelFull StaticError = "event channel full, dropping event"
	ErrConnectionClosed StaticError = "connection closed"
	ErrReadTimeout      StaticError = "read timeout"
)

// helper function to observe errors
func observe(obs Observer, err error) error {
	if obs != nil {
		obs.Observe(err)
	}
	return err
}

// HTTPClient interface to ease mock tests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client watches SSE events and updates the reloader
type Client struct {
	// Latest Reply, received from SSE or call to full endpoint
	lock  sync.Mutex
	reply *Reply
	// Connectivity data
	logger      *slog.Logger
	client      HTTPClient
	readTimeout time.Duration
	sseURL      string
	// reloader to call on new config
	reloader  dispatcher.Reloader
	virtualgw string
	// Used to build backoffs for the /final/full and /final/sse endpoints
	factory func() backoff.BackOff
}

// Creates an http.Client with proper timeouts
func NewHttpClient(connectTimeout, readTimeout time.Duration) (*http.Client, error) {
	if connectTimeout <= 0 || readTimeout <= 0 {
		return nil, ErrConfigTimeouts
	}
	// Set appropiate timeouts on the transport
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 3 * connectTimeout,
	}).DialContext
	transport.TLSHandshakeTimeout = connectTimeout
	transport.ResponseHeaderTimeout = connectTimeout
	transport.IdleConnTimeout = 3 * readTimeout
	// Timeout in the client itself must be 0, because this is a SSE event stream
	client := &http.Client{
		Transport: transport,
		Timeout:   0,
	}
	return client, nil
}

// NewClient creates a SSE event client
func NewClient(logger *slog.Logger, reloader dispatcher.Reloader, virtualgw string, baseURL string, client HTTPClient, readTimeout time.Duration, factory func() backoff.BackOff) (*Client, error) {
	if client == nil {
		return nil, ErrHttpClient
	}
	if readTimeout <= 0 {
		return nil, ErrConfigTimeouts
	}
	if factory == nil {
		return nil, ErrConfigFactory
	}
	if virtualgw == "" {
		return nil, ErrVirtualGateway
	}
	sseURL, err := url.JoinPath(baseURL, "final", "sse", virtualgw)
	if err != nil {
		return nil, err
	}
	return &Client{
		logger:      logger,
		reloader:    reloader,
		virtualgw:   virtualgw,
		sseURL:      sseURL,
		client:      client,
		readTimeout: readTimeout,
		factory:     factory,
	}, nil
}

// Current return the latest reply
func (c *Client) Current() *Reply {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.reply
}

// SSEURL returns the event stream URL configured for this client.
func (c *Client) SSEURL() string {
	if c == nil {
		return ""
	}
	return c.sseURL
}

// UnmarshalJSON implements json.Unmarshaler interface
func (r *Reply) UnmarshalJSON(payload []byte) error {
	var proxy struct {
		At   string `json:"at"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(payload, &proxy); err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339Nano, proxy.At)
	if err != nil {
		return err
	}
	r.At = at
	r.Data = []byte(proxy.Data)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler interface
func (e *Event) UnmarshalJSON(payload []byte) error {
	var proxy eventProxy
	if err := json.Unmarshal(payload, &proxy); err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339Nano, proxy.At)
	if err != nil {
		return err
	}
	sum, err := strconv.ParseUint(proxy.Sum, 10, 64)
	if err != nil {
		return err
	}
	hasher := fnv.New64()
	if _, err := hasher.Write(proxy.Data); err != nil {
		return err
	}
	if sum != hasher.Sum64() {
		return fmt.Errorf("checksum mismatch: got %d, expected %d", hasher.Sum64(), sum)
	}
	e.At = at
	e.Gzipped = proxy.Gzipped
	e.Data = proxy.Data
	e.Sum = sum
	return nil
}

func (e *Event) ToReply() (Reply, error) {
	data := e.Data
	if e.Gzipped {
		reader, err := gzip.NewReader(bytes.NewReader(e.Data))
		if err != nil {
			return Reply{}, err
		}
		payload, err := io.ReadAll(reader)
		if err != nil {
			return Reply{}, err
		}
		data = payload
	}
	return Reply{
		At:   e.At,
		Data: data,
	}, nil
}

// Loop connects to the SSE endpoint and forwards events to the events channel
func (c *Client) Loop(ctx context.Context, obs Observer) {
	bo := c.factory()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := c.stream(ctx, bo, obs)
		observe(obs, err)
		c.logger.Warn("event stream disconnected, reconnecting...", "err", err)
		delay := time.Second
		if bo != nil {
			delay = bo.NextBackOff()
			if delay == backoff.Stop || delay < 0 {
				delay = time.Second
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// stream connects to the SSE endpoint and forwards events to the events channel
func (c *Client) stream(ctx context.Context, bo backoff.BackOff, obs Observer) error {
	// Setup a wait group for the goroutine that will read the body.
	// We create it before the requestCtx, because otherwise the deferred cancel()
	// would we called after the deferred wg.Wait(), potentially causing a hang.
	lines := make(chan string, 16)
	errs := make(chan error, 1)
	waitingReload := make(chan error, 1)
	defer close(lines)
	defer close(errs)
	defer close(waitingReload)
	var wg sync.WaitGroup
	defer wg.Wait()

	// Let's do a request for the headers
	c.logger.Info("connecting to event stream", "url", c.sseURL)
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, c.sseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check the response format is expected
	if resp.StatusCode != http.StatusOK {
		return StatusCodeError(resp.StatusCode)
	}
	if contentType := strings.ToLower(resp.Header.Get("Content-Type")); contentType != "" && !strings.HasPrefix(contentType, "text/event-stream") {
		return ContenTypeError(resp.Header.Get("Content-Type"))
	}

	// Ensure canceling the request context always closes the response body,
	// so scanner reads unblock promptly during shutdown.
	wg.Go(func() {
		<-requestCtx.Done()
		_ = resp.Body.Close()
	})

	// Launch the line reader
	wg.Go(func() {
		c.logger.Info("reading event stream", "url", c.sseURL)
		scanner := bufio.NewScanner(resp.Body)
		// Raise default token limit to tolerate larger lines.
		const maxLine = 16 * 1024 * 1024
		scanner.Buffer(make([]byte, 0, 64*1024), maxLine)
		for scanner.Scan() {
			select {
			case <-requestCtx.Done():
				errs <- requestCtx.Err()
				return
			case lines <- scanner.Text():
			}
		}
		errs <- scanner.Err()
	})

	// Function that will process each event
	var (
		inflight *Reply
		target   *Reply
	)
	forwardTarget := func() {
		// If there is a request inflight, just wait
		if inflight != nil {
			return
		}
		inflight = target
		wg.Go(func() {
			c.logger.Info("applying new configuration")
			err := c.reloader.Apply(ctx, c.virtualgw, inflight.Data)
			if err == nil {
				c.lock.Lock()
				c.reply = inflight
				c.lock.Unlock()
			}
			select {
			case waitingReload <- err:
			case <-ctx.Done():
			}
		})
		if bo != nil {
			bo.Reset()
		}
	}

	// Keep receiving lines from the goroutine and dispatching to
	// the events channel, until the connection breaks or times out.
	timeout := time.NewTimer(c.readTimeout)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line, ok := <-lines:
			if !ok {
				return ErrConnectionClosed
			}
			if !timeout.Stop() {
				select {
				case <-timeout.C:
				default:
				}
			}
			timeout.Reset(c.readTimeout)
			if !strings.HasPrefix(line, "data: ") {
				break
			}
			var event Event
			dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "data: "))
			if err != nil {
				return err
			}
			if err := json.Unmarshal(dec, &event); err != nil {
				return err
			}
			c.logger.Info("received event", "at", event.At)
			if target != nil && !event.At.After(target.At) {
				// Ignore events that are not newer than the one we're waiting for
				c.logger.Warn("skipping old event", "at", event.At)
			} else {
				reply, err := event.ToReply()
				if err != nil {
					return err
				}
				target = &reply
				forwardTarget()
			}
		case err, ok := <-waitingReload:
			if !ok {
				return ErrConnectionClosed
			}
			inflightAt := inflight.At
			inflight = nil
			if err != nil {
				observe(obs, err)
				c.logger.Error("failed to apply configuration", "err", err)
				break
			}
			if target.At.After(inflightAt) {
				c.logger.Warn("config expired while reloading, will retry")
				forwardTarget()
			}
		case err, ok := <-errs:
			if !ok {
				return ErrConnectionClosed
			}
			return err
		case <-timeout.C:
			return ErrReadTimeout
		}
	}
}
