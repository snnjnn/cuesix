package sse

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/warpcondev/cuesix/internal/cache"
	"github.com/warpcondev/cuesix/internal/compiler"
	"github.com/warpcondev/cuesix/internal/cursor"
	"github.com/warpcondev/cuesix/internal/dispatcher"
)

const (
	ETagScope = "full-config"
)

// Reply struct with full config information
type Reply struct {
	At   time.Time `json:"at"`
	Data []byte    `json:"data"`
}

// MarshalJSON implements json.Marshaler interface
func (r Reply) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		At   string `json:"at"`
		Data string `json:"data"`
	}{
		At:   r.At.Format(time.RFC3339Nano),
		Data: string(r.Data),
	})
}

// Event struct that helps serializing Reply to []byte
type Event struct {
	At      time.Time `json:"at"`
	Gzipped bool      `json:"gzipped"`
	Data    []byte    `json:"data"`
	Sum     uint64    `json:"sum"`
}

// ToEvent converts a Reply into a base64 encoded string with optional compression
func (r *Reply) ToEvent() (Event, error) {
	gzipped := false
	payload := r.Data
	if payload != nil && len(payload) > 4096 {
		var buf bytes.Buffer
		compressor := gzip.NewWriter(&buf)
		if _, err := compressor.Write(r.Data); err != nil {
			return Event{}, err
		}
		if err := compressor.Flush(); err != nil {
			return Event{}, err
		}
		if err := compressor.Close(); err != nil {
			return Event{}, err
		}
		payload = buf.Bytes()
		gzipped = true
	}
	hasher := fnv.New64()
	if _, err := hasher.Write(payload); err != nil {
		return Event{}, err
	}
	return Event{
		At:      r.At,
		Data:    payload,
		Gzipped: gzipped,
		Sum:     hasher.Sum64(),
	}, nil
}

type eventProxy struct {
	At      string `json:"at"`
	Gzipped bool   `json:"gzipped"`
	Data    []byte `json:"data"`
	Sum     string `json:"sum"`
}

// ToEvent converts a Reply into a base64 encoded string with optional compression
func (e Event) MarshalJSON() ([]byte, error) {
	return json.Marshal(eventProxy{
		At:      e.At.Format(time.RFC3339Nano),
		Gzipped: e.Gzipped,
		Data:    e.Data,
		Sum:     strconv.FormatUint(e.Sum, 10),
	})
}

type record struct {
	payload []byte // latest Reply object, marshaled to JSON
	encoded string // latest Reply object, marshaled base64
	at      time.Time
}

// Reloader stores full validated config and distributes it with SSE
type Reloader struct {
	cursor.Watcher[string]
	logger  *slog.Logger
	records map[string]record
	nested  dispatcher.Reloader
}

// New initializes a Reloader
func New(logger *slog.Logger, reloader dispatcher.Reloader) *Reloader {
	if logger == nil {
		logger = slog.Default()
	}
	result := &Reloader{
		logger:  logger,
		records: make(map[string]record),
		nested:  reloader,
	}
	// Use always a single topic ""
	result.Embedded(func(virtualgw string) string { return virtualgw })
	return result
}

// Apply implements dispatcher.Reloader interface
func (reloader *Reloader) Apply(ctx context.Context, virtualgw string, payload []byte) error {
	if err := reloader.nested.Apply(ctx, virtualgw, payload); err != nil {
		return err
	}
	// Build a Reply object to cache it
	reply := &Reply{
		At:   time.Now(),
		Data: payload,
	}
	replyStr, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	event, err := reply.ToEvent()
	if err != nil {
		return err
	}
	enc, err := json.Marshal(event)
	if err != nil {
		return err
	}
	replyEnc := base64.StdEncoding.EncodeToString(enc)
	// Update the reloader and notify all SSE clients
	reloader.WithLock(func() {
		reloader.records[virtualgw] = record{
			at:      reply.At,
			payload: replyStr,
			encoded: replyEnc,
		}
		reloader.NotifyAllLocked(ctx, virtualgw)
	})
	return nil
}

// Fetch full dispatched configuration payload
func (reloader *Reloader) HandleFull() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		virtualgw, ok := compiler.PathVirtualGateway(w, r)
		if !ok {
			return
		}
		reloader.handleFullGateway(virtualgw, w, r)
	})
}

func (reloader *Reloader) handleFullGateway(virtualgw string, w http.ResponseWriter, r *http.Request) {
	var gwstate record
	reloader.WithLock(func() {
		gwstate = reloader.records[virtualgw]
	})
	if gwstate.at.IsZero() || gwstate.payload == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	scope := ETagScope
	w.Header().Set("Vary", "Accept-Encoding")
	if cache.Reply(gwstate.at, scope, w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(gwstate.payload)
}

// Subscribe to reload notifications via SSE
func (reloader *Reloader) HandleSSE(keepAlive time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		virtualgw, ok := compiler.PathVirtualGateway(w, r)
		if !ok {
			return
		}
		if err := reloader.handleSSE(keepAlive, virtualgw, w, r); err != nil {
			reloader.logger.Warn("SSE stream closed", "error", err.Error())
		}
	})
}

func (reloader *Reloader) handleSSE(keepAlive time.Duration, virtualgw string, w http.ResponseWriter, r *http.Request) error {
	if keepAlive <= 0 {
		keepAlive = 30 * time.Second
	}
	writeTimeout := 2 * keepAlive
	track := reloader.Watch(8, virtualgw)
	defer track.Close()
	var gwstate record
	reloader.WithLock(func() {
		gwstate = reloader.records[virtualgw]
	})
	header := w.Header()
	// SSE headers
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	// Return seed processed time
	rc := http.NewResponseController(w)
	refreshWriteDeadline := func() error {
		return rc.SetWriteDeadline(time.Now().Add(writeTimeout))
	}
	if err := refreshWriteDeadline(); err != nil {
		return err
	}
	if gwstate.at.IsZero() || gwstate.encoded == "" {
		if _, err := w.Write([]byte(": keepAlive\n\n")); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", gwstate.encoded); err != nil {
			return err
		}
	}
	if err := rc.Flush(); err != nil {
		return err
	}
	ticker := time.NewTicker(keepAlive)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Send keepalive comment
			if err := refreshWriteDeadline(); err != nil {
				return err
			}
			if _, err := w.Write([]byte(": keepAlive\n\n")); err != nil {
				return err
			}
			if err := rc.Flush(); err != nil {
				return err
			}
		case <-r.Context().Done():
			return r.Context().Err()
		case gwname, ok := <-track.Cursor:
			if !ok {
				return errors.New("track closed")
			}
			if gwname != virtualgw {
				return fmt.Errorf("mismatch in virtual gateway name, %s != %s", gwname, virtualgw)
			}
			// Send data
			reloader.WithLock(func() {
				gwstate = reloader.records[virtualgw]
			})
			if err := refreshWriteDeadline(); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", gwstate.encoded); err != nil {
				return err
			}
			if err := rc.Flush(); err != nil {
				return err
			}
		}
	}
}
