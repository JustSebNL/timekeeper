package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientDispatchUsesAPIWithoutServerStartup(t *testing.T) {
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/projects" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[]}`))
	}))
	defer api.Close()
	var out, errOut bytes.Buffer
	handled, code := dispatchClient([]string{"list"}, api.URL, &out, &errOut)
	if !handled || code != 0 || calls != 1 {
		t.Fatalf("handled=%v code=%d calls=%d error=%s", handled, code, calls, errOut.String())
	}
}

func TestClientDispatchDoesNotInterpretServerFlags(t *testing.T) {
	for _, args := range [][]string{nil, {"-addr", "127.0.0.1:1618"}, {"-backup-to", "backup.db"}} {
		var out, errOut bytes.Buffer
		if handled, _ := dispatchClient(args, "", &out, &errOut); handled {
			t.Fatalf("server args dispatched: %v", args)
		}
	}
}

func TestClientDispatchRejectsUnknownCommandAndMissingURL(t *testing.T) {
	for _, args := range [][]string{{"not-a-command"}, {"--url"}} {
		var out, errOut bytes.Buffer
		handled, code := dispatchClient(args, "", &out, &errOut)
		if !handled || code == 0 {
			t.Fatalf("args=%v handled=%v code=%d", args, handled, code)
		}
	}
}
