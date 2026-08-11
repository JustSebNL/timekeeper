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

func TestSubtaskCreatedThroughAPIIsScopedToItsTask(t *testing.T) {
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
	handler := api.New(database)

	create := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+task.TaskID+"/subtasks", bytes.NewBufferString(`{"name":"Implement vector recall","goal":"Return relevant memories","estimated_duration_seconds":900}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create subtask status = %d, body = %s", created.Code, created.Body.String())
	}
	var subtask struct {
		SubtaskID   string `json:"subtask_id"`
		TaskID      string `json:"task_id"`
		ItemAddress string `json:"item_address"`
		Name        string `json:"name"`
	}
	if err := json.NewDecoder(created.Body).Decode(&subtask); err != nil {
		t.Fatalf("decode subtask: %v", err)
	}
	if subtask.SubtaskID != "ST-10003" || subtask.TaskID != task.TaskID || subtask.ItemAddress != "10000.10001.10002.10003" || subtask.Name != "Implement vector recall" {
		t.Fatalf("unexpected subtask: %#v", subtask)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+task.TaskID+"/subtasks", nil)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK {
		t.Fatalf("list subtasks status = %d, body = %s", listed.Code, listed.Body.String())
	}
	var payload struct {
		Items []struct {
			SubtaskID string `json:"subtask_id"`
			TaskID    string `json:"task_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listed.Body).Decode(&payload); err != nil {
		t.Fatalf("decode subtask list: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].SubtaskID != subtask.SubtaskID || payload.Items[0].TaskID != task.TaskID {
		t.Fatalf("unexpected subtasks: %#v", payload.Items)
	}
}
