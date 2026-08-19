// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestClaimNextSprintEndpointStartsRunnableSprint(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Queue"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Work"})
	task, _ := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Run"})
	sprint, _ := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "First", EstimatedDurationSeconds: 60})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/sprints/claim-next", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("claim status=%d body=%s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"sprint_id":"`+sprint.SprintID+`"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"status":"Active"`)) {
		t.Fatalf("claim body=%s", response.Body.String())
	}
}
