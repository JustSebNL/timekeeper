package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFriendlyListenerUsesPrimaryHandler(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "timekeeper.local" {
			t.Errorf("host lost: %s", r.Host)
		}
		w.Write([]byte("primary"))
	})
	proxy := newProxyServer("127.0.0.1:0", h, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest("GET", "http://timekeeper.local/health", nil)
	out := httptest.NewRecorder()
	proxy.Handler.ServeHTTP(out, req)
	if out.Code != 200 || out.Body.String() != "primary" {
		t.Fatalf("proxy loops away from primary: %d %s", out.Code, out.Body)
	}
}
func TestFriendlyURLsUsesConfiguredPort(t *testing.T) {
	for _, tc := range []struct{ addr, want string }{{"127.0.0.1:80", "http://timekeeper.local/"}, {"127.0.0.1:8080", "http://timekeeper.local:8080/"}, {"127.0.0.1:1618", "http://timekeeper.local:1618/"}} {
		got := friendlyURLs(tc.addr)
		if len(got) != 3 || got[0] != tc.want {
			t.Fatalf("%s -> %v", tc.addr, got)
		}
	}
}
