package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/knowledge"
)

func TestCoreClientReadsMemoryGalaxyStatusAndSearchesThroughCore(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/knowledge/status":
			_, _ = fmt.Fprint(w, `{"available":true,"provider":"obsidian","protocol_version":"timekeeper-knowledge/v1","capabilities":["knowledge.search"],"last_checked_at":"2026-10-18T19:00:00Z"}`)
		case "POST /api/v1/knowledge/search":
			var query knowledge.KnowledgeQuery
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil || query.Query != "durable context" || query.Limit != 12 {
				t.Errorf("search query = %+v, err=%v", query, err)
			}
			_, _ = fmt.Fprint(w, `{"results":[{"id":"MG-1","title":"Durable context","type":"pattern","summary":"Store context across sessions.","topics":["memory"],"tags":["core"],"review_status":"verified","visibility":"private"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	status, err := client.KnowledgeStatus(context.Background())
	if err != nil || !status.Available || status.Provider != knowledge.ProviderNameObsidian || len(status.Capabilities) != 1 {
		t.Fatalf("KnowledgeStatus() = %+v, %v", status, err)
	}
	results, err := client.SearchKnowledge(context.Background(), knowledge.KnowledgeQuery{Query: "durable context", Limit: 12})
	if err != nil || len(results) != 1 || results[0].ID != "MG-1" || results[0].Visibility != knowledge.VisibilityPrivate {
		t.Fatalf("SearchKnowledge() = %+v, %v", results, err)
	}
	want := []string{"GET /api/v1/knowledge/status", "POST /api/v1/knowledge/search"}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("Core calls = %v, want %v", calls, want)
	}
}

func TestCoreClientKnowledgeSearchNormalizesEmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/knowledge/search" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"results":null}`)
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	results, err := client.SearchKnowledge(context.Background(), knowledge.KnowledgeQuery{Query: "empty", Limit: 8})
	if err != nil || results == nil || len(results) != 0 {
		t.Fatalf("SearchKnowledge() = %#v, %v; want non-nil empty results", results, err)
	}
}
