// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package obsidian_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/knowledge"
	"github.com/JustSebNL/timekeeper/internal/knowledge/obsidian"
)

func TestClientUsesDynamicDescriptorAndCapabilityToken(t *testing.T) {
	const token = "capability-test-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			if got := r.Header.Get("Authorization"); got != "Bearer "+token {
				t.Fatalf("authorization = %q", got)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/galaxy/status":
			_, _ = w.Write([]byte(`{"available":true,"protocol_version":"timekeeper-knowledge/v1","capabilities":["search","create"]}`))
		case "/api/v1/galaxy/search":
			_, _ = w.Write([]byte(`{"results":[{"id":"MG-1","title":"SQLite context","type":"pattern","review_status":"verified","visibility":"galaxy"}]}`))
		case "/api/v1/galaxy/items/MG-1":
			_, _ = w.Write([]byte(`{"id":"MG-1","title":"SQLite context","type":"pattern","review_status":"verified","visibility":"galaxy","content":"Keep the durable store authoritative."}`))
		case "/api/v1/galaxy/items":
			_, _ = w.Write([]byte(`{"id":"MG-2","title":"A decision","type":"decision","review_status":"inbox","visibility":"galaxy","content":"A bounded item."}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "capability.token")
	if err := os.WriteFile(tokenPath, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	descriptorPath := filepath.Join(dir, "endpoint.json")
	descriptor := obsidian.EndpointDescriptor{BaseURL: server.URL, ProtocolVersion: knowledge.ProtocolVersion, Provider: knowledge.ProviderNameObsidian, VaultID: "vault-test", TokenFile: "capability.token"}
	data, _ := json.Marshal(descriptor)
	if err := os.WriteFile(descriptorPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	client := obsidian.NewClient(descriptorPath)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	status, err := client.Status(ctx)
	if err != nil || !status.Available || status.VaultID != "vault-test" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	results, err := client.Search(ctx, knowledge.KnowledgeQuery{Query: "context", Limit: 1})
	if err != nil || len(results) != 1 || results[0].ID != "MG-1" {
		t.Fatalf("results=%#v err=%v", results, err)
	}
	item, err := client.Get(ctx, "MG-1")
	if err != nil || item.Content == "" {
		t.Fatalf("item=%#v err=%v", item, err)
	}
	created, err := client.Create(ctx, knowledge.KnowledgeCandidate{Title: "A decision", Content: "A bounded item.", Type: knowledge.ItemTypeDecision, Visibility: knowledge.VisibilityGalaxy}, "promotion-1")
	if err != nil || created.ID != "MG-2" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
}

func TestClientRejectsNonLoopbackAndRedirectEndpoints(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"http://localhost:1234",
		"http://127.0.0.1:1234/path",
		"http://127.0.0.1:1234/?redirect=1",
		"https://127.0.0.1:1234",
		"http://192.168.1.10:1234",
	}
	for _, baseURL := range cases {
		t.Run(baseURL, func(t *testing.T) {
			path := filepath.Join(dir, strings.NewReplacer(":", "_", "/", "_", "?", "_").Replace(baseURL)+".json")
			data, _ := json.Marshal(obsidian.EndpointDescriptor{BaseURL: baseURL, ProtocolVersion: knowledge.ProtocolVersion, Provider: knowledge.ProviderNameObsidian, TokenFile: "token"})
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := obsidian.NewClient(path).Status(context.Background())
			if err == nil || !strings.Contains(err.Error(), "endpoint") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
