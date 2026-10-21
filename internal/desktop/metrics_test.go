package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoreClientReadsProjectMetricsFromCore(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet {
			t.Errorf("metrics request method = %s, want GET", r.Method)
		}
		switch r.URL.Path {
		case "/api/v1/projects/P-7/usage-summary":
			_, _ = fmt.Fprint(w, `{"project_id":"P-7","totals":{"input_tokens":1200,"output_tokens":350,"cache_creation_tokens":80,"cache_read_tokens":40,"messages":5,"session_count":1,"cost":{"status":"unpriced","reason":"no trusted local price"}},"sessions":null,"days":null}`)
		case "/api/v1/projects/P-7/operational-summary":
			_, _ = fmt.Fprint(w, `{"project_id":"P-7","total_sprints":4,"open_sprints":1,"active_sprints":1,"held_sprints":0,"completed_sprints":2,"timed_out_sprints":0,"cancelled_sprints":0,"estimated_duration_seconds":7200,"buffer_duration_seconds":600,"extension_duration_seconds":300,"planned_duration_seconds":8100,"recorded_work_seconds":3600,"recorded_hold_seconds":0}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	usage, err := client.ProjectUsageSummary(context.Background(), " P-7 ")
	if err != nil {
		t.Fatalf("ProjectUsageSummary: %v", err)
	}
	if usage.ProjectID != "P-7" || usage.Totals.InputTokens != 1200 || usage.Totals.Cost.Status != "unpriced" {
		t.Fatalf("ProjectUsageSummary() = %+v", usage)
	}
	if usage.Sessions == nil || usage.Days == nil {
		t.Fatalf("empty usage arrays must be non-nil: sessions=%v days=%v", usage.Sessions, usage.Days)
	}

	operational, err := client.ProjectOperationalSummary(context.Background(), "P-7")
	if err != nil {
		t.Fatalf("ProjectOperationalSummary: %v", err)
	}
	if operational.ProjectID != "P-7" || operational.TotalSprints != 4 || operational.PlannedDurationSeconds != 8100 || operational.RecordedWorkSeconds != 3600 {
		t.Fatalf("ProjectOperationalSummary() = %+v", operational)
	}
	want := []string{"GET /api/v1/projects/P-7/usage-summary", "GET /api/v1/projects/P-7/operational-summary"}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("Core calls = %v, want %v", calls, want)
	}
}

func TestCoreClientProjectMetricsRejectBlankAndMismatchedProjects(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if strings.HasSuffix(r.URL.Path, "/usage-summary") {
			_, _ = fmt.Fprint(w, `{"project_id":"P-other","totals":{"cost":{"status":"unpriced"}},"sessions":[],"days":[]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"project_id":"P-other"}`)
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ProjectUsageSummary(context.Background(), " "); err == nil {
		t.Fatal("blank Project ID must be rejected")
	}
	if _, err := client.ProjectOperationalSummary(context.Background(), ""); err == nil {
		t.Fatal("blank Project ID must be rejected")
	}
	if requests != 0 {
		t.Fatalf("blank Project IDs made %d Core requests", requests)
	}
	if _, err := client.ProjectUsageSummary(context.Background(), "P-7"); err == nil {
		t.Fatal("mismatched usage Project ID must be rejected")
	}
	if _, err := client.ProjectOperationalSummary(context.Background(), "P-7"); err == nil {
		t.Fatal("mismatched operational Project ID must be rejected")
	}
	if requests != 2 {
		t.Fatalf("mismatched response requests = %d, want 2", requests)
	}
}
