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

func TestProjectEventsExposeSprintLifecycleHistory(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Hercules"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Runtime"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Event stream"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Instrument", EstimatedDurationSeconds: 900})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}

	server := api.New(database)
	startResponse := httptest.NewRecorder()
	startRequest := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/start", nil)
	startRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startResponse.Code, startResponse.Body.String())
	}

	eventsResponse := httptest.NewRecorder()
	server.ServeHTTP(eventsResponse, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/events", nil))
	if eventsResponse.Code != http.StatusOK {
		t.Fatalf("events status = %d, body = %s", eventsResponse.Code, eventsResponse.Body.String())
	}
	var events struct {
		Items []struct {
			EntityID  string `json:"entity_id"`
			EventType string `json:"event_type"`
			Message   string `json:"message"`
		} `json:"items"`
	}
	if err := json.NewDecoder(eventsResponse.Body).Decode(&events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events.Items) != 1 || events.Items[0].EntityID != sprint.SprintID || events.Items[0].EventType != "sprint_started" || events.Items[0].Message != "Sprint started." {
		t.Fatalf("events = %#v", events.Items)
	}
}
