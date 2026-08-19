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

func TestSprintLifecycleRecordsWorkAndHoldTimeEntries(t *testing.T) {
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

	create := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+task.TaskID+"/sprints", bytes.NewBufferString(`{"name":"Implement retrieval","estimated_duration_seconds":600,"buffer_pct":10}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create sprint status = %d, body = %s", created.Code, created.Body.String())
	}
	var sprint struct {
		SprintID    string `json:"sprint_id"`
		ItemAddress string `json:"item_address"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(created.Body).Decode(&sprint); err != nil {
		t.Fatalf("decode sprint: %v", err)
	}
	if sprint.SprintID != "SP-10003" || sprint.ItemAddress != "10000.10001.10002.10003" || sprint.Status != "Open" {
		t.Fatalf("unexpected created sprint: %#v", sprint)
	}

	start := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/start", bytes.NewBufferString(`{"reason":"test lifecycle"}`))
	start.Header.Set("Content-Type", "application/json")
	started := httptest.NewRecorder()
	handler.ServeHTTP(started, start)
	if started.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", started.Code, started.Body.String())
	}

	invalidStart := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/start", nil)
	invalidStart.Header.Set("Content-Type", "application/json")
	invalidStartResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidStartResponse, invalidStart)
	if invalidStartResponse.Code != http.StatusConflict {
		t.Fatalf("invalid start status = %d, body = %s", invalidStartResponse.Code, invalidStartResponse.Body.String())
	}
	var invalidPayload struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Status         string   `json:"status"`
				AllowedActions []string `json:"allowed_actions"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(invalidStartResponse.Body).Decode(&invalidPayload); err != nil {
		t.Fatalf("decode invalid transition: %v", err)
	}
	if invalidPayload.Error.Code != "invalid_transition" || invalidPayload.Error.Details.Status != "Active" || len(invalidPayload.Error.Details.AllowedActions) != 3 || invalidPayload.Error.Details.AllowedActions[0] != "cancel" || invalidPayload.Error.Details.AllowedActions[1] != "complete" || invalidPayload.Error.Details.AllowedActions[2] != "hold" {
		t.Fatalf("invalid transition payload = %#v", invalidPayload)
	}

	for _, action := range []string{"hold", "resume", "complete"} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/"+action, bytes.NewBufferString(`{"reason":"test lifecycle"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", action, response.Code, response.Body.String())
		}
	}

	entries, err := database.ListTimeEntries(context.Background(), sprint.SprintID)
	if err != nil {
		t.Fatalf("list time entries: %v", err)
	}
	if len(entries) != 3 || entries[0].EntryType != "work" || entries[1].EntryType != "hold" || entries[2].EntryType != "work" {
		t.Fatalf("unexpected time entries: %#v", entries)
	}

	entriesRequest := httptest.NewRequest(http.MethodGet, "/api/v1/sprints/"+sprint.SprintID+"/time-entries", nil)
	entriesResponse := httptest.NewRecorder()
	handler.ServeHTTP(entriesResponse, entriesRequest)
	if entriesResponse.Code != http.StatusOK {
		t.Fatalf("time entry endpoint status = %d, body = %s", entriesResponse.Code, entriesResponse.Body.String())
	}
	var entryPayload struct {
		Items []struct {
			EntryType string `json:"entry_type"`
		} `json:"items"`
	}
	if err := json.NewDecoder(entriesResponse.Body).Decode(&entryPayload); err != nil {
		t.Fatalf("decode time entry endpoint: %v", err)
	}
	if len(entryPayload.Items) != 3 || entryPayload.Items[1].EntryType != "hold" {
		t.Fatalf("unexpected time entry endpoint items: %#v", entryPayload.Items)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+task.TaskID+"/sprints", nil)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK {
		t.Fatalf("list sprint status = %d, body = %s", listed.Code, listed.Body.String())
	}
	var payload struct {
		Items []struct {
			SprintID string `json:"sprint_id"`
			Status   string `json:"status"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listed.Body).Decode(&payload); err != nil {
		t.Fatalf("decode sprint list: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].SprintID != sprint.SprintID || payload.Items[0].Status != "Completed" {
		t.Fatalf("unexpected sprints: %#v", payload.Items)
	}
}
