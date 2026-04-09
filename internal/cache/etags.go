package cache

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Reply adds cache headers and returns true if the request has been completed.
func Reply(lastModified time.Time, scope string, w http.ResponseWriter, r *http.Request) bool {
	etag, etagModified := cacheTag(lastModified, scope)
	if notModified(r, lastModified, etag) {
		applyCacheHeaders(w, etag, etagModified)
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	applyCacheHeaders(w, etag, etagModified)
	return false
}

func applyCacheHeaders(w http.ResponseWriter, etag, lastModified string) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	if lastModified != "" {
		w.Header().Set("Last-Modified", lastModified)
	}
	// Encourage clients to revalidate against our cache tags.
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
}

func notModified(r *http.Request, ts time.Time, etag string) bool {
	if etag != "" {
		for tag := range strings.SplitSeq(r.Header.Get("If-None-Match"), ",") {
			if strings.TrimSpace(tag) == etag || strings.TrimSpace(tag) == "*" {
				return true
			}
		}
	}
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		if since, err := http.ParseTime(ims); err == nil && !ts.After(since) {
			return true
		}
	}
	return false
}

func cacheTag(ts time.Time, scope string) (string, string) {
	base := fmt.Sprintf("%d", ts.UnixNano())
	if scope != "" {
		base = base + ":" + scope
	}
	return fmt.Sprintf("\"%s\"", base), ts.UTC().Format(http.TimeFormat)
}
