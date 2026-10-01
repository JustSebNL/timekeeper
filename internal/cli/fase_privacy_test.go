package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestFASEActivitySessionDoesNotExposeRegistryFingerprint(t *testing.T) {
	fingerprint := strings.Repeat("a", 64)
	currentFASE.Store(&trackedFASE{identity: model.FASEIdentity{AgentID: "FASE-A1", Runtime: "custom"}, fingerprint: fingerprint})
	defer currentFASE.Store(nil)
	first := resolveAgentIdentity()
	if first.SessionID == "" || first.SessionID == fingerprint {
		t.Fatal("public activity session exposes registry fingerprint")
	}
	if resolveAgentIdentity().SessionID != first.SessionID {
		t.Fatal("activity session changed across resolutions")
	}
}

func TestFASETransportSeparatesDisplayAuthorAndStripsForeignIdentity(t *testing.T) {
	for _, sameOrigin := range []bool{true, false} {
		t.Run(map[bool]string{true: "core", false: "foreign"}[sameOrigin], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if sameOrigin {
					if r.Header.Get("X-Agent-ID") != "FASE-A1" || r.Header.Get("X-Agent-Session-ID") == "" || r.Header.Get("X-Agent-Session-ID") == "old-session" {
						t.Error("display author overrode tracked caller identity")
					}
				} else {
					for _, key := range []string{"X-Agent-ID", "X-Agent-Session-ID", "X-Agent-Runtime"} {
						if r.Header.Get(key) != "" {
							t.Errorf("foreign origin received %s", key)
						}
					}
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			base := server.URL
			if !sameOrigin {
				base = "http://other-core.invalid"
			}
			currentFASE.Store(&trackedFASE{identity: model.FASEIdentity{AgentID: "FASE-A1", Runtime: "custom"}, fingerprint: strings.Repeat("a", 64), baseURL: base})
			defer currentFASE.Store(nil)
			r, _ := http.NewRequest(http.MethodGet, server.URL, nil)
			r.Header.Set("X-Agent-ID", "display-author")
			r.Header.Set("X-Agent-Session-ID", "old-session")
			r.Header.Set("X-Agent-Runtime", "old-runtime")
			response, err := newHTTPClient(time.Second).Do(r)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
		})
	}
}
