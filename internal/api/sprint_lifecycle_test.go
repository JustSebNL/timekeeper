// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestReopenedCompletedTaskCanHoldFollowUpSprint(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Reopened task"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Follow-up work"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	first, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Initial work", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create initial sprint: %v", err)
	}

	fingerprint := strings.Repeat("a", 64)
	identity, err := database.ResolveFASEIdentityAt(ctx, model.FASEIdentityInput{Fingerprint: fingerprint, Runtime: "test", IdentityScope: "runtime-session"}, time.Now())
	if err != nil {
		t.Fatalf("resolve FASE identity: %v", err)
	}
	call := func(identity model.FASEIdentity, fingerprint, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Agent-ID", identity.AgentID)
		request.Header.Set("X-Agent-Session-ID", model.FASEActivitySession(fingerprint))
		request.Header.Set("X-Agent-Runtime", "test")
		response := httptest.NewRecorder()
		api.New(database).ServeHTTP(response, request)
		return response
	}

	for _, action := range []string{"start", "complete"} {
		response := call(identity, fingerprint, http.MethodPost, "/api/v1/sprints/"+first.SprintID+"/"+action, `{}`)
		if response.Code != http.StatusOK {
			t.Fatalf("initial Sprint %s: status=%d body=%s", action, response.Code, response.Body.String())
		}
	}
	for _, status := range []string{"Completed", "Open"} {
		response := call(identity, fingerprint, http.MethodPost, "/api/v1/tasks/"+task.TaskID+"/status", `{"status":"`+status+`"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("task status %s: status=%d body=%s", status, response.Code, response.Body.String())
		}
	}

	created := call(identity, fingerprint, http.MethodPost, "/api/v1/tasks/"+task.TaskID+"/sprints", `{"name":"Follow-up","estimated_duration_seconds":60}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create follow-up Sprint: status=%d body=%s", created.Code, created.Body.String())
	}
	var followUp model.Sprint
	if err := json.NewDecoder(created.Body).Decode(&followUp); err != nil {
		t.Fatalf("decode follow-up Sprint: %v", err)
	}
	for _, action := range []string{"start", "hold"} {
		body := `{}`
		if action == "hold" {
			body = `{"reason":"follow-up handoff"}`
		}
		response := call(identity, fingerprint, http.MethodPost, "/api/v1/sprints/"+followUp.SprintID+"/"+action, body)
		if response.Code != http.StatusOK {
			t.Fatalf("follow-up Sprint %s: status=%d body=%s", action, response.Code, response.Body.String())
		}
	}
}

func TestCompletingFinalSubtaskSprintLeavesParentStatusesExplicit(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Subtask completion"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Explicit parent status"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	subtask, err := database.CreateSubtask(ctx, task.TaskID, model.CreateSubtaskInput{Name: "Bounded child work"})
	if err != nil {
		t.Fatalf("create subtask: %v", err)
	}
	sprint, err := database.CreateSubtaskSprint(ctx, subtask.SubtaskID, model.CreateSprintInput{Name: "Child execution", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create subtask Sprint: %v", err)
	}
	fingerprint := strings.Repeat("f", 64)
	identity, err := database.ResolveFASEIdentityAt(ctx, model.FASEIdentityInput{Fingerprint: fingerprint, Runtime: "test", IdentityScope: "runtime-session"}, time.Now())
	if err != nil {
		t.Fatalf("resolve FASE identity: %v", err)
	}
	handler := api.New(database)
	call := func(action string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/"+action, bytes.NewBufferString(`{}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Agent-ID", identity.AgentID)
		request.Header.Set("X-Agent-Session-ID", model.FASEActivitySession(fingerprint))
		request.Header.Set("X-Agent-Runtime", "test")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	for _, action := range []string{"start", "complete"} {
		response := call(action)
		if response.Code != http.StatusOK {
			t.Fatalf("Sprint %s: status=%d body=%s", action, response.Code, response.Body.String())
		}
	}
	updatedTask, err := database.GetTask(ctx, task.TaskID)
	if err != nil || updatedTask.Status != "Open" {
		t.Fatalf("Task status = %q, err=%v; want Open", updatedTask.Status, err)
	}
	subtasks, err := database.ListSubtasks(ctx, task.TaskID)
	if err != nil || len(subtasks) != 1 || subtasks[0].SubtaskID != subtask.SubtaskID || subtasks[0].Status != "Open" {
		t.Fatalf("Subtasks = %#v, err=%v; want the explicit Open Subtask", subtasks, err)
	}
}
