// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectImport(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"project_id": "P-10556"})
	})

	mux.HandleFunc("POST /api/v1/projects/P-10556/categories", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"category_id": "C-123"})
	})

	mux.HandleFunc("POST /api/v1/projects/P-10556/tasks/batch", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"tasks": []map[string]string{
				{"task_id": "T-100"},
				{"task_id": "T-101"},
			},
		})
	})

	mux.HandleFunc("POST /api/v1/tasks/T-100/subtasks", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"subtask_id": "ST-1"})
	})
	mux.HandleFunc("POST /api/v1/tasks/T-101/subtasks", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"subtask_id": "ST-2"})
	})

	mux.HandleFunc("POST /api/v1/tasks/T-100/sprints", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"sprint_id": "SP-1"})
	})

	mux.HandleFunc("POST /api/v1/subtasks/ST-1/sprints", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"sprint_id": "SP-2"})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	t.Run("JSON Flat", func(t *testing.T) {
		jsonContent := `{
			"project": {"project_id": "old_p1", "project_name": "My Project"},
			"categories": [{"category_id": "old_c1", "name": "Cat1"}],
			"tasks": [{"task_id": "old_t1", "category_id": "old_c1", "name": "Task1"}],
			"subtasks": [{"subtask_id": "old_st1", "task_id": "old_t1", "name": "Subtask1"}],
			"sprints": [{"sprint_id": "old_sp1", "task_id": "old_t1", "name": "Sprint1"}]
		}`
		tmp := filepath.Join(t.TempDir(), "test.json")
		os.WriteFile(tmp, []byte(jsonContent), 0644)

		var out bytes.Buffer
		var errOut bytes.Buffer
		code := Run([]string{"import", tmp, "--name", "Imported Project"}, &out, &errOut, ts.URL)

		if code != 0 {
			t.Fatalf("expected code 0, got %d, errOut: %s", code, errOut.String())
		}
		if !strings.Contains(out.String(), "Categories: 1") {
			t.Errorf("expected 1 category, got %s", out.String())
		}
	})

	t.Run("TarGz Import", func(t *testing.T) {
		tmp := filepath.Join(t.TempDir(), "test.tar.gz")
		f, _ := os.Create(tmp)
		gw := gzip.NewWriter(f)
		tw := tar.NewWriter(gw)

		csvData := "project_id,name,goal,status,created_at\nP1,Proj,Goal,Active,2026\n"
		tw.WriteHeader(&tar.Header{Name: "project.csv", Size: int64(len(csvData)), Mode: 0600})
		tw.Write([]byte(csvData))

		tw.Close()
		gw.Close()
		f.Close()

		var out, errOut bytes.Buffer
		code := Run([]string{"import", tmp}, &out, &errOut, ts.URL)
		if code != 0 {
			t.Fatalf("expected code 0, got %d, errOut: %s", code, errOut.String())
		}
	})

	t.Run("RAR Import Error", func(t *testing.T) {
		tmp := filepath.Join(t.TempDir(), "test.rar")
		// Write invalid RAR header just to trigger the RAR parser
		os.WriteFile(tmp, []byte("Rar!\x1A\x07\x00\x11"), 0644)

		var out, errOut bytes.Buffer
		code := Run([]string{"import", tmp}, &out, &errOut, ts.URL)
		if code == 0 {
			t.Fatalf("expected error on invalid rar, got success")
		}
		if !strings.Contains(errOut.String(), "parse import") {
			t.Errorf("expected parse error, got: %s", errOut.String())
		}
	})

}
