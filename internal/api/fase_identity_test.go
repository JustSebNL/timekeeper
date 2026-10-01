package api_test

import (
	"bytes"
	"encoding/json"
	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestFASEAutomaticHandshakeAndMaskedProfile(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "fase.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	handler := api.New(s)
	body := `{"fingerprint":"` + strings.Repeat("a", 64) + `","runtime":"custom","identity_scope":"runtime-session","metadata":{"model":"test-model"}}`
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	first := call("POST", "/api/v1/agent-identities/resolve", body)
	if first.Code != 200 {
		t.Fatalf("handshake: %d %s", first.Code, first.Body.String())
	}
	var id model.FASEIdentity
	if err = json.Unmarshal(first.Body.Bytes(), &id); err != nil {
		t.Fatal(err)
	}
	second := call("POST", "/api/v1/agent-identities/resolve", body)
	var again model.FASEIdentity
	if err = json.Unmarshal(second.Body.Bytes(), &again); err != nil || again.AgentKey != id.AgentKey {
		t.Fatalf("%s %v", second.Body.String(), err)
	}
	profile := call("GET", "/api/v1/agent-profiles/"+id.FASEID, "")
	if profile.Code != 200 || strings.Contains(profile.Body.String(), strings.Repeat("a", 64)) || !strings.Contains(profile.Body.String(), "test-model") {
		t.Fatalf("profile: %d %s", profile.Code, profile.Body.String())
	}
	html := call("GET", "/api/v1/agent-profiles/"+id.FASEID+"/view", "")
	if html.Code != 200 || !strings.Contains(html.Body.String(), id.FASEID) {
		t.Fatalf("profile view: %d %s", html.Code, html.Body.String())
	}
}
