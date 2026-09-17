package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectionURLsUsesRuntimePort(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"friendly_urls":["http://timekeeper.local:1618/","http://api.timekeeper.local:1618/"]}`))
	}))
	defer s.Close()
	got := connectionURLs(s.URL)
	if got[0] != "http://timekeeper.local:1618/" || got[1] != "http://api.timekeeper.local:1618/" {
		t.Fatalf("%v", got)
	}
}
