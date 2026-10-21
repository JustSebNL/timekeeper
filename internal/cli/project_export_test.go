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

func TestProjectExport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/v1/projects/P-1/export") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		response := ProjectExportResponse{
			Format:    "timekeeper-project-export/v1",
			ProjectID: "P-1",
			ExecutionTree: ExecutionTree{
				Project: model.Project{
					ProjectID:   "P-1",
					ProjectName: "Project 1",
					Status:      "Active",
					CreatedAt:   time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
				},
				Categories: []ExecutionCategory{
					{
						Category: model.Category{
							CategoryID: "C-1",
							ProjectID:  "P-1",
							Name:       "Category 1",
							Status:     "Open",
							CreatedAt:  time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
						},
						Tasks: []ExecutionTask{
							{
								Task: model.Task{
									TaskID:                   "T-1",
									CategoryID:               "C-1",
									Name:                     "Task 1",
									Status:                   "Open",
									Priority:                 "Normal",
									EstimatedDurationSeconds: 1800,
									AutoClose:                true,
									CreatedAt:                time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
								},
								Subtasks: []ExecutionSubtask{
									{
										Subtask: model.Subtask{
											SubtaskID:                "ST-1",
											TaskID:                   "T-1",
											Name:                     "Subtask 1",
											Status:                   "Open",
											EstimatedDurationSeconds: 900,
											CreatedAt:                time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
										},
										Sprints: []model.Sprint{
											{
												SprintID:                 "SP-1",
												TaskID:                   "T-1",
												SubtaskID:                "ST-1",
												Name:                     "Sprint 1",
												Status:                   "Complete",
												EstimatedDurationSeconds: 900,
												CreatedAt:                time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	t.Run("SQL Project Export", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		code := projectExportMulti([]string{"P-1", "-", "--format", "sql"}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}

		output := out.String()

		if !strings.Contains(output, "INSERT INTO projects") {
			t.Errorf("missing projects insert")
		}
		if !strings.Contains(output, "('P-1', 'Project 1', '', '', 'Active')") {
			t.Errorf("missing project data")
		}

		if !strings.Contains(output, "INSERT INTO categories") {
			t.Errorf("missing categories insert")
		}

		if !strings.Contains(output, "INSERT INTO tasks") {
			t.Errorf("missing tasks insert")
		}

		if !strings.Contains(output, "INSERT INTO subtasks") {
			t.Errorf("missing subtasks insert")
		}

		if !strings.Contains(output, "INSERT INTO sprints") {
			t.Errorf("missing sprints insert")
		}
		if !strings.Contains(output, "('SP-1', 'ST-1', 'Sprint 1', 'Complete', 900)") {
			t.Errorf("missing sprint data: %s", output)
		}
	})

	t.Run("XLSX Project Export", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer

		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "out.xlsx")

		code := projectExportMulti([]string{"P-1", p}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}

		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("xlsx file not created")
		}
	})

	t.Run("CSV Project Export", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer

		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "out.csv")

		code := projectExportMulti([]string{"P-1", p}, &out, &errOut, server.URL)
		if code != 0 {
			t.Fatalf("expected 0, got %d: %s", code, errOut.String())
		}

		zipPath := filepath.Join(tmpDir, "out.zip")
		if _, err := os.Stat(zipPath); os.IsNotExist(err) {
			t.Errorf("zip file not created")
		}
	})
}

func TestProjectExportTSV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		response := ProjectExportResponse{
			Format:    "timekeeper-project-export/v1",
			ProjectID: "P-1",
			ExecutionTree: ExecutionTree{
				Project: model.Project{
					ProjectID:   "P-1",
					ProjectName: "Project 1",
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "out.tsv")
	var out, errOut bytes.Buffer
	code := projectExportMulti([]string{"P-1", p}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("failed: %v", errOut.String())
	}
	zr, err := zip.OpenReader(strings.TrimSuffix(p, ".tsv") + ".zip")
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	hasTSV := false
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".tsv") {
			hasTSV = true
		}
	}
	if !hasTSV {
		t.Fatalf("no .tsv files in zip")
	}
}

func TestProjectExportTarGz(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := ProjectExportResponse{
			Format:    "timekeeper-project-export/v1",
			ProjectID: "P-1",
			ExecutionTree: ExecutionTree{
				Project: model.Project{ProjectID: "P-1", ProjectName: "Project 1"},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "out.tar.gz")
	var out, errOut bytes.Buffer
	code := projectExportMulti([]string{"P-1", p}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("failed: %v", errOut.String())
	}
	f, _ := os.Open(p)
	defer f.Close()
	gzr, _ := gzip.NewReader(f)
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("open tar: %v", err)
	}
	hasCSV := false
	for hdr != nil {
		if strings.HasSuffix(hdr.Name, ".csv") {
			hasCSV = true
		}
		hdr, err = tr.Next()
	}
	if !hasCSV {
		t.Fatalf("no .csv files in tar")
	}
}

func TestProjectExportTaskFilterSendsQueryAndOmitsOtherBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects/P-1/export" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if got := r.URL.Query().Get("task_id"); got != "T-2" {
			t.Errorf("task_id query=%q, want T-2", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"format":"timekeeper-project-export/v1","project_id":"P-1","execution_tree":{"project":{"project_id":"P-1","project_name":"Atlas"},"categories":[{"category":{"category_id":"C-1","project_id":"P-1","name":"Backend"},"categories":[],"tasks":[{"task":{"task_id":"T-1","category_id":"C-1","name":"Unrelated task","status":"Open"},"sprints":[],"subtasks":[]},{"task":{"task_id":"T-2","category_id":"C-1","name":"Target task","status":"Open"},"sprints":[],"subtasks":[]}]}]},"notes":[{"content":"UNRELATED PRIVATE NOTE BODY"}],"events":[{"summary":"UNRELATED EVENT BODY"}],"operational_summary":{"internal":"other data"},"usage_summary":{"internal":"usage data"},"milestones":[{"name":"unrelated"}]}`))
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := Run([]string{"export", "P-1", "--task", "T-2"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("export code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	output := out.String()
	for _, want := range []string{"T-2", "Target task", "filtered_by"} {
		if !strings.Contains(output, want) {
			t.Errorf("filtered export missing %q: %s", want, output)
		}
	}
	for _, excluded := range []string{"T-1", "Unrelated task", "UNRELATED PRIVATE NOTE BODY", "UNRELATED EVENT BODY", "operational_summary", "usage_summary", "milestones"} {
		if strings.Contains(output, excluded) {
			t.Errorf("filtered export leaked %q: %s", excluded, output)
		}
	}
}
