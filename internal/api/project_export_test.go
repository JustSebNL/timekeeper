// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

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
	if _, err := database.RecordUsageSnapshot(ctx, project.ProjectID, model.UsageSnapshotInput{SessionID: "export-session", AgentID: "agent-1", Model: "local", TurnSeq: 1, InputTokens: 100, OutputTokens: 20}); err != nil {
		t.Fatalf("record usage: %v", err)
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
		Usage struct {
			Totals struct {
				InputTokens  int64 `json:"input_tokens"`
				OutputTokens int64 `json:"output_tokens"`
			} `json:"totals"`
		} `json:"usage_summary"`
	}
	if err := json.NewDecoder(response.Body).Decode(&exported); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if exported.Format != "timekeeper-project-export/v1" || exported.ProjectID != project.ProjectID || exported.Tree.Project.ProjectID != project.ProjectID || len(exported.Notes) != 1 || exported.Notes[0].Content != "Keep this context." || exported.Notes[0].ActorID != "agent-1" || exported.Usage.Totals.InputTokens != 100 || exported.Usage.Totals.OutputTokens != 20 {
		t.Fatalf("export = %#v", exported)
	}
}

func TestProjectExportTaskFilterOmitsUnrelatedNotesAndEvents(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "filtered-project-export.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Filtered export"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Target task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Unrelated task"}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProjectNote(ctx, project.ProjectID, model.CreateProjectNoteInput{Content: "Unrelated private note body"}, "agent-1"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/export?task_id="+target.TaskID, nil)
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("filtered export status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{"notes", "events", "operational_summary", "usage_summary", "milestones"} {
		if _, exists := envelope[excluded]; exists {
			t.Errorf("filtered export included unrelated %s", excluded)
		}
	}
	var tree model.ExecutionTree
	if err := json.Unmarshal(envelope["execution_tree"], &tree); err != nil {
		t.Fatal(err)
	}
	if tree.Project.ProjectID != project.ProjectID || len(tree.Categories) != 1 || tree.Categories[0].Category.CategoryID != category.CategoryID || len(tree.Categories[0].Tasks) != 1 || tree.Categories[0].Tasks[0].Task.TaskID != target.TaskID {
		t.Fatalf("filtered export tree=%+v", tree)
	}
	var filter struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(envelope["filtered_by"], &filter); err != nil || filter.TaskID != target.TaskID {
		t.Fatalf("filtered_by=%s err=%v", envelope["filtered_by"], err)
	}
}
