package cache_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/warpcondev/cuesix/internal/cache"
)

func TestReplySetsCacheHeaders(t *testing.T) {
	t.Parallel()

	lastModified := time.Unix(1700000000, 123456789).UTC()
	req := httptest.NewRequest(http.MethodGet, "/final/full", nil)
	rec := httptest.NewRecorder()

	handled := cache.Reply(lastModified, "full-config", rec, req)

	if handled {
		t.Fatalf("Reply() handled request unexpectedly")
	}
	if got := rec.Header().Get("ETag"); got != `"1700000000123456789:full-config"` {
		t.Fatalf("ETag = %q", got)
	}
	if got := rec.Header().Get("Last-Modified"); got != lastModified.Format(http.TimeFormat) {
		t.Fatalf("Last-Modified = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=0, must-revalidate" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestReplyConditionalRequests(t *testing.T) {
	t.Parallel()

	lastModified := time.Unix(1700000000, 0).UTC()
	etag := `"1700000000000000000:full-config"`

	tests := []struct {
		name   string
		header func(*http.Request)
		want   int
	}{
		{
			name: "if none match exact",
			header: func(r *http.Request) {
				r.Header.Set("If-None-Match", etag)
			},
			want: http.StatusNotModified,
		},
		{
			name: "if none match wildcard",
			header: func(r *http.Request) {
				r.Header.Set("If-None-Match", "*")
			},
			want: http.StatusNotModified,
		},
		{
			name: "if none match comma separated with spaces",
			header: func(r *http.Request) {
				r.Header.Set("If-None-Match", `"other", `+etag+`, "third"`)
			},
			want: http.StatusNotModified,
		},
		{
			name: "if modified since exact",
			header: func(r *http.Request) {
				r.Header.Set("If-Modified-Since", lastModified.Format(http.TimeFormat))
			},
			want: http.StatusNotModified,
		},
		{
			name: "if modified since newer",
			header: func(r *http.Request) {
				r.Header.Set("If-Modified-Since", lastModified.Add(time.Second).Format(http.TimeFormat))
			},
			want: http.StatusNotModified,
		},
		{
			name: "if modified since invalid ignored",
			header: func(r *http.Request) {
				r.Header.Set("If-Modified-Since", "not-a-date")
			},
			want: http.StatusOK,
		},
		{
			name: "if none match mismatch",
			header: func(r *http.Request) {
				r.Header.Set("If-None-Match", `"other"`)
			},
			want: http.StatusOK,
		},
		{
			name: "if modified since older",
			header: func(r *http.Request) {
				r.Header.Set("If-Modified-Since", lastModified.Add(-time.Second).Format(http.TimeFormat))
			},
			want: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/final/full", nil)
			tt.header(req)
			rec := httptest.NewRecorder()

			handled := cache.Reply(lastModified, "full-config", rec, req)

			if got := rec.Code; got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
			if handled != (tt.want == http.StatusNotModified) {
				t.Fatalf("handled = %v, want %v", handled, tt.want == http.StatusNotModified)
			}
			if got := rec.Header().Get("ETag"); got != etag {
				t.Fatalf("ETag = %q", got)
			}
			if got := rec.Header().Get("Last-Modified"); got != lastModified.Format(http.TimeFormat) {
				t.Fatalf("Last-Modified = %q", got)
			}
			if got := rec.Header().Get("Cache-Control"); got != "public, max-age=0, must-revalidate" {
				t.Fatalf("Cache-Control = %q", got)
			}
		})
	}
}

func TestReplyScopeChangesETag(t *testing.T) {
	t.Parallel()

	lastModified := time.Unix(1700000000, 0).UTC()

	reqA := httptest.NewRequest(http.MethodGet, "/final/full", nil)
	recA := httptest.NewRecorder()
	cache.Reply(lastModified, "scope-a", recA, reqA)

	reqB := httptest.NewRequest(http.MethodGet, "/final/full", nil)
	recB := httptest.NewRecorder()
	cache.Reply(lastModified, "scope-b", recB, reqB)

	if recA.Header().Get("ETag") == recB.Header().Get("ETag") {
		t.Fatalf("expected different ETags for different scopes")
	}
}
