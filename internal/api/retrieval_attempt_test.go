// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestRetrievalAttemptAPIStopsAtFourAndSurfacesDurableHistory(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Retrieval API"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Execution"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Retrieve"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Bounded retrieval", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}
	handler := api.New(database)

	for attempt := 1; attempt <= 4; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/retrieval-attempts", bytes.NewBufferString(fmt.Sprintf(`{"reason":"attempt %d"}`, attempt)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("attempt %d status=%d body=%s", attempt, response.Code, response.Body.String())
		}
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/sprints/"+sprint.SprintID+"/retrieval-attempts", nil)
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	if got := listResponse.Body.String(); !bytes.Contains([]byte(got), []byte(`"attempt_number":4`)) || !bytes.Contains([]byte(got), []byte(`"timed_out":true`)) {
		t.Fatalf("history=%s", got)
	}
}
