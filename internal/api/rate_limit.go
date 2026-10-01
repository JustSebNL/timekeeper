// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api

import (
	"net/http"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/guardian"
)

// rateLimitMiddleware returns HTTP 429 when an agent key exceeds token bucket capacity.
func rateLimitMiddleware(limiter *guardian.RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Identify agent key
			agentKey := strings.TrimSpace(r.Header.Get("X-Agent-ID"))
			if agentKey == "" {
				agentKey = strings.TrimSpace(r.Header.Get("X-Session-ID"))
			}

			// Unattributed or system probe requests bypass rate limiting
			if agentKey == "" || (r.Method == http.MethodGet && (r.URL.Path == "/api/v1/health" || r.URL.Path == "/api/v1/version")) {
				next.ServeHTTP(w, r)
				return
			}

			if !limiter.Allow(agentKey) {
				writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Agent request rate limit exceeded (infinite loop protection). Slow down or renew material progress lease.")
				return
			}

			// If request is progress lease renewal or sprint completion, reset token bucket
			if r.Method == http.MethodPost && (strings.Contains(r.URL.Path, "/progress") || strings.Contains(r.URL.Path, "/complete")) {
				limiter.Reset(agentKey)
			}

			next.ServeHTTP(w, r)
		})
	}
}
