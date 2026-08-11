// Copyright (c) 2026 Seb. All rights reserved.

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectExportProvidesPortableDurableSnapshot(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Exportable"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := database.CreateProjectNote(ctx, project.ProjectID, model.CreateProjectNoteInput{Content: "Keep this context."}, "agent-1"); err != nil {
		t.Fatalf("create note: %v", err)
	}

	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/export", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("export status = %d, body = %s", response.Code, response.Body.String())
	}
	var exported struct {
		Format    string `json:"format"`
		ProjectID string `json:"project_id"`
		Tree      struct {
			Project struct {
				ProjectID string `json:"project_id"`
			} `json:"project"`
		} `json:"execution_tree"`
		Notes []struct {
			Content string `json:"content"`
			ActorID string `json:"actor_id"`
		} `json:"notes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&exported); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if exported.Format != "timekeeper-project-export/v1" || exported.ProjectID != project.ProjectID || exported.Tree.Project.ProjectID != project.ProjectID || len(exported.Notes) != 1 || exported.Notes[0].Content != "Keep this context." || exported.Notes[0].ActorID != "agent-1" {
		t.Fatalf("export = %#v", exported)
	}
}
