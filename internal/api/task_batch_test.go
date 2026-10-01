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

func TestTasksBatchAPIRoute(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Batch API Test"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(context.Background(), project.ProjectID, model.CreateCategoryInput{Name: "API Core"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	handler := api.New(database)

	payload := `{"tasks":[{"category_id":"` + category.CategoryID + `","name":"Batch Task A","estimated_duration_seconds":600},{"category_id":"` + category.CategoryID + `","name":"Batch Task B","estimated_duration_seconds":900}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/tasks/batch", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Tasks []model.Task `json:"tasks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(res.Tasks) != 2 || res.Tasks[0].Name != "Batch Task A" || res.Tasks[1].Name != "Batch Task B" {
		t.Fatalf("unexpected tasks batch response: %#v", res.Tasks)
	}
}
