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

func TestProjectAttentionExposesHeldSprintReason(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Attention"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Execution"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Waiting"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Consent", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}

	handler := api.New(database)
	hold := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/hold", bytes.NewBufferString(`{"reason":"Waiting for user consent"}`))
	hold.Header.Set("Content-Type", "application/json")
	holdResponse := httptest.NewRecorder()
	handler.ServeHTTP(holdResponse, hold)
	if holdResponse.Code != http.StatusOK {
		t.Fatalf("hold status = %d, body = %s", holdResponse.Code, holdResponse.Body.String())
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/attention", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("attention status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		ProjectID string `json:"project_id"`
		Items     []struct {
			Kind       string `json:"kind"`
			SprintID   string `json:"sprint_id"`
			HoldReason string `json:"hold_reason"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode attention: %v", err)
	}
	if body.ProjectID != project.ProjectID || len(body.Items) != 1 || body.Items[0].Kind != "sprint_on_hold" || body.Items[0].SprintID != sprint.SprintID || body.Items[0].HoldReason != "Waiting for user consent" {
		t.Fatalf("attention = %#v", body)
	}
}
