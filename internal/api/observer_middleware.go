// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/observer"
)

// Observe wraps a handler so every request is journalled.
//
// This is the seam that turns the observer from a loaded capability into
// one that actually receives events. Without it the journal reports
// healthy and stays empty.
//
// Journalling is best-effort and never affects the response: a full
// disk, a closed journal or a slow write must degrade to "no event"
// rather than failing the caller's request.
func Observe(obs observer.Observer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if obs == nil {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Fire and forget, with a bounded context so a stalled
			// journal cannot hold a goroutine forever.
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				payload, err := json.Marshal(map[string]any{
					"kind":    "http_request",
					"method":  r.Method,
					"path":    redactPath(r.URL.Path),
					"status":  rec.code,
					"elapsed": time.Since(start).Milliseconds(),
				})
				if err != nil {
					return
				}
				// Errors are intentionally dropped: the request already
				// succeeded, and journalling is not on the critical path.
				_, _, _ = obs.Append(ctx, newEventID(start), payload)
			}()
		})
	}
}

// statusRecorder captures the status code, which the ResponseWriter
// otherwise discards.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// redactPath keeps only the route shape, never the identifiers in it.
//
// Journalling a raw path would put project IDs, task IDs and sprint IDs
// into the journal unredacted.
func redactPath(path string) string {
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		// Anything that looks like an entity id becomes a placeholder.
		if looksLikeID(seg) {
			segments[i] = "{id}"
		}
	}
	return strings.Join(segments, "/")
}

// looksLikeID reports whether a path segment is an entity identifier,
// such as P-10289, T-10359 or SP-10363.
func looksLikeID(seg string) bool {
	if len(seg) < 2 {
		return false
	}
	// Known entity prefixes used by TimeKeeper ids.
	for _, prefix := range []string{"P-", "T-", "SP-", "C-", "M-"} {
		if strings.HasPrefix(seg, prefix) {
			return true
		}
	}
	// A long hex or digit run is treated as an opaque identifier.
	if len(seg) >= 12 {
		opaque := true
		for _, ch := range seg {
			if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F') {
				opaque = false
				break
			}
		}
		if opaque {
			return true
		}
	}
	return false
}

// newEventID builds a stable identifier from the start time so a replay
// of the same request is deduplicated rather than recorded twice.
func newEventID(start time.Time) string {
	return "http-" + start.UTC().Format("20060102T150405.000000000")
}
