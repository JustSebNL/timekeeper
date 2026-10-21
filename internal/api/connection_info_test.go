package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/store"
	"github.com/JustSebNL/timekeeper/internal/version"
)

func TestConnectionInfoReturnsStableInstanceAndCapabilities(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "connection-info.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	handler := api.NewWithRuntime(database, api.RuntimeStatus{
		FriendlyURLs:                 []string{"http://timekeeper.local/", "http://api.timekeeper.local/"},
		PulseGuardianEnabled:         true,
		PulseGuardianIntervalSeconds: 300,
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	type response struct {
		Service      string   `json:"service"`
		InstanceID   string   `json:"instance_id"`
		Version      string   `json:"version"`
		Commit       string   `json:"commit"`
		APIVersion   string   `json:"api_version"`
		Capabilities []string `json:"capabilities"`
		FriendlyURLs []string `json:"friendly_urls"`
	}
	get := func() response {
		resp, err := http.Get(server.URL + "/api/v1/connection-info")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("connection-info status=%d", resp.StatusCode)
		}
		var body response
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	first := get()
	second := get()
	if first.Service != "TimeKeeper" || first.InstanceID == "" || first.InstanceID != second.InstanceID {
		t.Fatalf("connection identity is missing or unstable: first=%+v second=%+v", first, second)
	}
	if first.Version != version.Get().Version || first.APIVersion != "v1" {
		t.Fatalf("compatibility metadata mismatch: %+v", first)
	}
	for _, capability := range []string{"projects.list", "projects.execution_tree", "tasks.batch.validate", "execution_batches.validate", "routing.candidates.read", "routing.category.evaluate", "routing.proposal.evaluate", "routing.proposal.apply", "pulse.guardian"} {
		found := false
		for _, actual := range first.Capabilities {
			if actual == capability {
				found = true
			}
		}
		if !found {
			t.Errorf("connection-info omitted capability %q: %v", capability, first.Capabilities)
		}
	}
	if len(first.FriendlyURLs) != 2 || first.FriendlyURLs[0] != "http://timekeeper.local/" {
		t.Fatalf("friendly URLs changed: %v", first.FriendlyURLs)
	}

	storedID, err := database.InstanceID(context.Background())
	if err != nil || storedID != first.InstanceID {
		t.Fatalf("API identity does not match Store identity: api=%q store=%q err=%v", first.InstanceID, storedID, err)
	}
}
