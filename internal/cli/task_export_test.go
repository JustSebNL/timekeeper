// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestExportFormatDuration(t *testing.T) {
	tests := []struct {
		seconds  int64
		expected string
	}{
		{0, "0s"},
		{90, "1m30s"},
		{1800, "30m"},
		{3600, "1h"},
		{5400, "1h30m"},
		{86400, "24h"},
	}

	for _, tt := range tests {
		actual := formatDuration(tt.seconds)
		if actual != tt.expected {
			t.Errorf("formatDuration(%d): expected %s, got %s", tt.seconds, tt.expected, actual)
		}
	}
}

func TestExportTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/v1/projects/P-1/tasks") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		tasks := []model.Task{
			{
				TaskID:                   "T-1",
				ProjectID:                "P-1",
				CategoryID:               "C-1",
				Name:                     "Task 1",
				Status:                   "Open",
				Priority:                 "Normal",
				EstimatedDurationSeconds: 1800,
				CalculatedCompletionPct:  50.0,
				AutoClose:                true,
				CreatedAt:                time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			{
				TaskID:                   "T-2",
				ProjectID:                "P-1",
				CategoryID:               "C-2",
				Name:                     "Task 2",
				Status:                   "Closed",
				Priority:                 "High",
				EstimatedDurationSeconds: 3600,
				CalculatedCompletionPct:  100.0,
				AutoClose:                false,
				CreatedAt:                time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
			},
		}
		json.NewEncoder(w).Encode(map[string]any{"items": tasks})
	}))
	defer server.Close()

	t.Run("CSV Export", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		code := taskExport([]string{"P-1", "-", "--format", "csv"}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}
		output := out.String()
		if !strings.Contains(output, "task_id,category_id,name,description,goal,status,priority,estimate,completion_pct,auto_close,created_at") {
			t.Errorf("missing header in CSV: %s", output)
		}
		if !strings.Contains(output, "T-1,C-1,Task 1,,,Open,Normal,30m,50.00,true,2026-01-01T12:00:00Z") {
			t.Errorf("missing row 1 in CSV: %s", output)
		}
	})

	t.Run("TSV Export", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		code := taskExport([]string{"P-1", "-", "--format", "tsv"}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}
		output := out.String()
		if !strings.Contains(output, "task_id\tcategory_id\tname\tdescription\tgoal\tstatus\tpriority\testimate\tcompletion_pct\tauto_close\tcreated_at") {
			t.Errorf("missing header in TSV: %s", output)
		}
		if !strings.Contains(output, "T-1\tC-1\tTask 1\t\t\tOpen\tNormal\t30m\t50.00\ttrue\t2026-01-01T12:00:00Z") {
			t.Errorf("missing row 1 in TSV: %s", output)
		}
	})

	t.Run("YAML Export", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		code := taskExport([]string{"P-1", "-", "--format", "yaml"}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}
		output := out.String()
		if !strings.Contains(output, "task_id: T-1") {
			t.Errorf("missing task 1 in YAML: %s", output)
		}
		if !strings.Contains(output, "estimate: 30m") {
			t.Errorf("missing human duration in YAML: %s", output)
		}
	})

	t.Run("JSON Export", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		code := taskExport([]string{"P-1", "-", "--format", "json"}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}
		output := out.String()
		var parsed map[string][]model.Task
		if err := json.Unmarshal([]byte(output), &parsed); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if len(parsed["tasks"]) != 2 {
			t.Fatalf("expected 2 tasks in json, got %d", len(parsed["tasks"]))
		}
		if parsed["tasks"][0].TaskID != "T-1" {
			t.Errorf("unexpected task ID in json: %s", parsed["tasks"][0].TaskID)
		}
	})

	t.Run("Filter by Status", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		code := taskExport([]string{"--status", "Open", "P-1", "-", "--format", "json"}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}
		var parsed map[string][]model.Task
		_ = json.Unmarshal(out.Bytes(), &parsed)
		if len(parsed["tasks"]) != 1 || parsed["tasks"][0].TaskID != "T-1" {
			t.Errorf("status filter failed: %+v", parsed["tasks"])
		}
	})

	t.Run("Filter by Category", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		code := taskExport([]string{"P-1", "-", "--format", "json", "--category", "C-2"}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}
		var parsed map[string][]model.Task
		_ = json.Unmarshal(out.Bytes(), &parsed)
		if len(parsed["tasks"]) != 1 || parsed["tasks"][0].TaskID != "T-2" {
			t.Errorf("category filter failed: %+v", parsed["tasks"])
		}
	})

	t.Run("Unknown Format Reject", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		code := taskExport([]string{"P-1", "-", "--format", "bad"}, &out, &errOut, server.URL)
		if code != 2 {
			t.Fatalf("expected 2, got %d", code)
		}
		if !strings.Contains(errOut.String(), "unknown format") {
			t.Errorf("unexpected error: %s", errOut.String())
		}
	})

	t.Run("Extension Detection", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "out.csv")

		code := taskExport([]string{"P-1", p}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}

		data, _ := os.ReadFile(p)
		if !strings.Contains(string(data), "task_id,category_id") {
			t.Errorf("CSV not generated correctly from extension")
		}
	})
}

func TestTaskExportZIP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []model.Task{{TaskID: "T-1", Name: "ZipTask", Priority: "Normal"}}})
	}))
	defer server.Close()
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "out.zip")
	var out, errOut bytes.Buffer
	code := taskExport([]string{"P-1", p}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("failed: %v", errOut.String())
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != "tasks.csv" {
		t.Fatalf("bad zip contents")
	}
}

func TestTaskExportTarGz(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []model.Task{{TaskID: "T-1", Name: "TarTask", Priority: "Normal"}}})
	}))
	defer server.Close()
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "out.tar.gz")
	var out, errOut bytes.Buffer
	code := taskExport([]string{"P-1", p}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("failed: %v", errOut.String())
	}
	f, _ := os.Open(p)
	defer f.Close()
	gzr, _ := gzip.NewReader(f)
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	hdr, err := tr.Next()
	if err != nil || hdr.Name != "tasks.csv" {
		t.Fatalf("bad tar contents: %v", err)
	}
}
