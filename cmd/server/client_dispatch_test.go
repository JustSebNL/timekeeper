package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestClientDispatchUsesAPIWithoutServerStartup(t *testing.T) {
	calls := 0
	s, err := store.Open(filepath.Join(t.TempDir(), "dispatch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	handler := api.New(s)
	t.Setenv("TIMEKEEPER_SESSION_ID", "dispatch-test")
	t.Setenv("TIMEKEEPER_AGENT_RUNTIME", "test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/projects" && r.URL.Path != "/api/v1/agent-identities/resolve" {
			t.Errorf("path=%s", r.URL.Path)
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	handled, code := dispatchClient([]string{"list"}, server.URL, &out, &errOut)
	if !handled || code != 0 || calls != 2 {
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
