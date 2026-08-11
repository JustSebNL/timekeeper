// Copyright (c) 2026 Seb. All rights reserved.

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

func TestTaskCreatedThroughAPIBelongsToItsCategory(t *testing.T) {
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
	handler := api.New(database)

	create := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/tasks", bytes.NewBufferString(`{"category_id":"`+category.CategoryID+`","name":"Build recall","goal":"Retrieve relevant memory","estimated_duration_seconds":1800}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create task status = %d, body = %s", created.Code, created.Body.String())
	}
	var task struct {
		TaskID      string `json:"task_id"`
		CategoryID  string `json:"category_id"`
		ItemAddress string `json:"item_address"`
		Name        string `json:"name"`
	}
	if err := json.NewDecoder(created.Body).Decode(&task); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	if task.TaskID != "T-10002" || task.CategoryID != category.CategoryID || task.ItemAddress != "10000.10001.10002" || task.Name != "Build recall" {
		t.Fatalf("unexpected task: %#v", task)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/tasks", nil)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK {
		t.Fatalf("list task status = %d, body = %s", listed.Code, listed.Body.String())
	}
	var payload struct {
		Items []struct {
			TaskID     string `json:"task_id"`
			CategoryID string `json:"category_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listed.Body).Decode(&payload); err != nil {
		t.Fatalf("decode task list: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].TaskID != "T-10002" || payload.Items[0].CategoryID != category.CategoryID {
		t.Fatalf("unexpected tasks: %#v", payload.Items)
	}
}
