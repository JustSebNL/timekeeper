// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
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

func TestSprintCreatedThroughAPIIsOwnedBySubtask(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "HSAM"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(context.Background(), project.ProjectID, model.CreateCategoryInput{Name: "Memory"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(context.Background(), project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Build recall", EstimatedDurationSeconds: 1800})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	subtask, err := database.CreateSubtask(context.Background(), task.TaskID, model.CreateSubtaskInput{Name: "Implement vector recall", EstimatedDurationSeconds: 900})
	if err != nil {
		t.Fatalf("create subtask: %v", err)
	}
	handler := api.New(database)

	create := httptest.NewRequest(http.MethodPost, "/api/v1/subtasks/"+subtask.SubtaskID+"/sprints", bytes.NewBufferString(`{"name":"Implement retrieval","estimated_duration_seconds":600,"buffer_pct":10}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create subtask sprint status = %d, body = %s", created.Code, created.Body.String())
	}
	var sprint struct {
		SprintID    string `json:"sprint_id"`
		TaskID      string `json:"task_id"`
		SubtaskID   string `json:"subtask_id"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.NewDecoder(created.Body).Decode(&sprint); err != nil {
		t.Fatalf("decode subtask sprint: %v", err)
	}
	if sprint.SprintID != "SP-10004" || sprint.TaskID != task.TaskID || sprint.SubtaskID != subtask.SubtaskID || sprint.ItemAddress != "10000.10001.10002.10003.10004" {
		t.Fatalf("unexpected subtask sprint: %#v", sprint)
	}

	directList := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+task.TaskID+"/sprints", nil)
	directListed := httptest.NewRecorder()
	handler.ServeHTTP(directListed, directList)
	if directListed.Code != http.StatusOK {
		t.Fatalf("list direct task sprints status = %d, body = %s", directListed.Code, directListed.Body.String())
	}
	var directPayload struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(directListed.Body).Decode(&directPayload); err != nil {
		t.Fatalf("decode direct task sprint list: %v", err)
	}
	if len(directPayload.Items) != 0 {
		t.Fatalf("subtask-owned Sprint appeared as a direct Task Sprint: %#v", directPayload.Items)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/subtasks/"+subtask.SubtaskID+"/sprints", nil)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK {
		t.Fatalf("list subtask sprints status = %d, body = %s", listed.Code, listed.Body.String())
	}
	var payload struct {
		Items []struct {
			SprintID  string `json:"sprint_id"`
			SubtaskID string `json:"subtask_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listed.Body).Decode(&payload); err != nil {
		t.Fatalf("decode subtask sprint list: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].SprintID != sprint.SprintID || payload.Items[0].SubtaskID != subtask.SubtaskID {
		t.Fatalf("unexpected subtask sprints: %#v", payload.Items)
	}
}
