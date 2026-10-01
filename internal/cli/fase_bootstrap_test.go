package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestFASEFirstCommandAutomaticallyEnrollsAndReusesIdentity(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "fase.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server := httptest.NewServer(api.New(s))
	defer server.Close()
	t.Setenv("TIMEKEEPER_AGENT_ID", "")
	t.Setenv("TIMEKEEPER_SESSION_ID", "session-one")
	t.Setenv("TIMEKEEPER_AGENT_RUNTIME", "custom")
	run := func(args ...string) string {
		t.Helper()
		var out, errOut strings.Builder
		if code := RunTracked(args, &out, &errOut, server.URL); code != 0 {
			t.Fatalf("%v: %d %s", args, code, errOut.String())
		}
		return out.String()
	}
	run("list")
	var first model.FASEIdentity
	if err = json.Unmarshal([]byte(run("agent", "identity")), &first); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.FASEID, "FASE-ID.") {
		t.Fatalf("not a Core-assigned identity: %+v", first)
	}
	var same model.FASEIdentity
	if err = json.Unmarshal([]byte(run("agent", "identity")), &same); err != nil || same.AgentKey != first.AgentKey {
		t.Fatalf("identity changed: %+v %v", same, err)
	}
	t.Setenv("TIMEKEEPER_SESSION_ID", "session-two")
	var other model.FASEIdentity
	if err = json.Unmarshal([]byte(run("agent", "identity")), &other); err != nil || other.AgentKey == first.AgentKey {
		t.Fatalf("merged agents: %+v %v", other, err)
	}
}
func TestFASEFailedHandshakeNeverRunsProjectMutation(t *testing.T) {
	mutations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/projects" {
			mutations++
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	var out, errOut strings.Builder
	if code := RunTracked([]string{"p", "new", "must not create"}, &out, &errOut, server.URL); code == 0 || mutations != 0 {
		t.Fatalf("anonymous mutation: code=%d mutations=%d", code, mutations)
	}
	out.Reset()
	errOut.Reset()
	if code := RunTracked([]string{"help"}, &out, &errOut, server.URL); code != 0 || !strings.Contains(errOut.String(), "identity") {
		t.Fatalf("offline diagnostics: %d %s", code, errOut.String())
	}
}

func TestFASEFailedHandshakeAllowsLocalHostsStatus(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	t.Setenv("TIMEKEEPER_AGENT_ID", "")
	t.Setenv("TIMEKEEPER_SESSION_ID", "installer")
	t.Setenv("TIMEKEEPER_AGENT_RUNTIME", "custom")
	var out, errOut strings.Builder
	RunTracked([]string{"hosts", "status"}, &out, &errOut, server.URL)
	if !strings.Contains(out.String(), "found:") && !strings.Contains(out.String(), "missing:") {
		t.Fatalf("hosts status did not run: out=%s err=%s", out.String(), errOut.String())
	}
}
