package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestGlobalSearchCLIMessageFiltersSendToAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		for key, want := range map[string]string{"q": "queue", "scope": "messages", "kind": "decision", "tags": "durable,reviewed", "after": "2026-10-01", "before": "2026-10-31"} {
			if query.Get(key) != want {
				t.Errorf("query[%s]=%q, want %q", key, query.Get(key), want)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(model.GlobalSearchResponse{Query: "queue", Items: []model.GlobalSearchResult{}, Limit: 20})
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := Run([]string{"search", "queue", "--messages", "--kind", "decision", "--tags", "durable,reviewed", "--after", "2026-10-01", "--before", "2026-10-31"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("search code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
}

func TestGlobalSearchCLIJSONOutputIsStructured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(model.GlobalSearchResponse{Query: "queue", Total: 1, Limit: 20, Items: []model.GlobalSearchResult{{Type: "message", ProjectID: "P-10000", ProjectName: "Queue Atlas", ObjectID: "1", Ref: "M-1", Title: "Queue note", Kind: "decision", Tags: "queue", Snippet: "Queue note", Score: 100}}})
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	if code := Run([]string{"--json", "search", "queue"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("JSON search code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	var response model.GlobalSearchResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON search: %v; output=%q", err, out.String())
	}
	if response.Total != 1 || len(response.Items) != 1 || response.Items[0].Ref != "M-1" || response.Items[0].ProjectID != "P-10000" {
		t.Fatalf("JSON search response=%+v", response)
	}
}

func TestGlobalSearchCLIRequestsScopesAndPrintsAttributedHumanResults(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Path != "/api/v1/search" || request.URL.Query().Get("q") != "queue worker" || request.URL.Query().Get("project") != "P-10000" || request.URL.Query().Get("limit") != "1" {
			t.Fatalf("request=%s %s", request.Method, request.URL.String())
		}
		if got := request.URL.Query()["scope"]; len(got) != 1 || got[0] != model.GlobalSearchScopeTasks {
			t.Fatalf("scopes=%v", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(model.GlobalSearchResponse{
			Query: "queue worker", ProjectID: "P-10000", Scopes: []string{"tasks"}, Total: 2, Limit: 1, HasMore: true,
			Items: []model.GlobalSearchResult{{Type: "task", ProjectID: "P-10000", ProjectName: "Queue Atlas", ObjectID: "T-10001", Ref: "T-10001", Title: "Queue Worker", Status: "Open", Snippet: "Queue worker deployment", Score: 220}},
		})
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"search", "queue", "worker", "--tasks", "--project", "P-10000", "--limit", "1"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("search code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	for _, want := range []string{"2 matches; showing first 1", "T-10001", "Queue Atlas", "Queue Worker", "Queue worker deployment"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q: %q", want, out.String())
		}
	}
	if requests != 1 {
		t.Fatalf("made %d requests, want one", requests)
	}
}

func TestGlobalSearchHelpDoesNotContactCore(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { requests++ }))
	defer server.Close()
	var out, errOut bytes.Buffer
	if code := Run([]string{"search", "--help"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("help code=%d stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "usage: tk search") {
		t.Fatalf("help=%q", out.String())
	}
	if requests != 0 {
		t.Fatalf("help made %d requests", requests)
	}
}
