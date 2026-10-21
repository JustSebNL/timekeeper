// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

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

// Recovering one Task's goal and description used to require exporting the
// whole Project and filtering it by hand.
func TestGetTaskReturnsFocusedDetailWithChildren(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Focused Detail"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Work"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{
		Name: "Ship focused detail", CategoryID: category.CategoryID,
		Goal:                     "Read one task without exporting the project",
		Description:              "Long description that must be recoverable narrowly",
		EstimatedDurationSeconds: 1800,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	subtask, err := database.CreateSubtask(ctx, task.TaskID, model.CreateSubtaskInput{Name: "Sub item", EstimatedDurationSeconds: 600})
	if err != nil {
		t.Fatalf("create subtask: %v", err)
	}
	handler := api.New(database)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+task.TaskID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Task     model.Task `json:"task"`
		Subtasks []struct {
			SubtaskID string `json:"subtask_id"`
		} `json:"subtasks"`
		Children struct {
			OpenSubtasks int  `json:"open_subtasks"`
			CanComplete  bool `json:"can_complete"`
		} `json:"children"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode task detail: %v", err)
	}
	if payload.Task.Goal != "Read one task without exporting the project" || payload.Task.Description == "" {
		t.Fatalf("task detail lost its context: %+v", payload.Task)
	}
	if len(payload.Subtasks) != 1 || payload.Subtasks[0].SubtaskID != subtask.SubtaskID {
		t.Fatalf("unexpected subtasks: %+v", payload.Subtasks)
	}
	if payload.Children.OpenSubtasks != 1 || payload.Children.CanComplete {
		t.Fatalf("children summary must report the open blocker: %+v", payload.Children)
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/T-99999", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing task status = %d body = %s", missing.Code, missing.Body.String())
	}
}

func TestGetTaskCompletionSummaryTreatsTimedOutSprintAsTerminal(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Timed out detail"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Work"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{
		Name: "Complete after retrieval timeout", CategoryID: category.CategoryID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Timed out", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create Sprint: %v", err)
	}
	for attempt := 1; attempt <= 4; attempt++ {
		if _, err := database.RecordSprintRetrievalAttempt(ctx, sprint.SprintID, "worker could not retrieve Sprint"); err != nil {
			t.Fatalf("record retrieval attempt %d: %v", attempt, err)
		}
	}

	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+task.TaskID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Sprints  []model.Sprint `json:"sprints"`
		Children struct {
			OpenSprints int  `json:"open_sprints"`
			CanComplete bool `json:"can_complete"`
		} `json:"children"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode task detail: %v", err)
	}
	if len(payload.Sprints) != 1 || payload.Sprints[0].Status != "TimedOut" {
		t.Fatalf("expected one TimedOut Sprint in detail: %+v", payload.Sprints)
	}
	if payload.Children.OpenSprints != 0 || !payload.Children.CanComplete {
		t.Fatalf("TimedOut Sprint must be terminal for Task completion summary: %+v", payload.Children)
	}
}

func TestGetTaskCompletionSummaryIncludesSubtaskSprints(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Nested sprint summary"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Work"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{Name: "Parent", CategoryID: category.CategoryID})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	subtask, err := database.CreateSubtask(ctx, task.TaskID, model.CreateSubtaskInput{Name: "Child"})
	if err != nil {
		t.Fatalf("create subtask: %v", err)
	}
	if _, err := database.CreateSubtaskSprint(ctx, subtask.SubtaskID, model.CreateSprintInput{Name: "Still running", EstimatedDurationSeconds: 60}); err != nil {
		t.Fatalf("create subtask Sprint: %v", err)
	}
	if _, err := database.UpdateSubtaskStatus(ctx, subtask.SubtaskID, model.UpdateSubtaskStatusInput{Status: "Completed"}); err != nil {
		t.Fatalf("complete subtask: %v", err)
	}

	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+task.TaskID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Sprints []model.Sprint `json:"sprints"`
		Children struct {
			OpenSprints int  `json:"open_sprints"`
			CanComplete bool `json:"can_complete"`
		} `json:"children"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode task detail: %v", err)
	}
	if len(payload.Sprints) != 0 {
		t.Fatalf("Task detail should keep only direct Sprints visible: %+v", payload.Sprints)
	}
	if payload.Children.OpenSprints != 1 || payload.Children.CanComplete {
		t.Fatalf("open Subtask Sprint must block Task completion: %+v", payload.Children)
	}
}

func TestGetSubtaskReturnsFocusedDetail(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Subtask Detail"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Work"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{Name: "Parent", CategoryID: category.CategoryID, EstimatedDurationSeconds: 600})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	subtask, err := database.CreateSubtask(ctx, task.TaskID, model.CreateSubtaskInput{Name: "Child", Goal: "Recover narrowly", EstimatedDurationSeconds: 300})
	if err != nil {
		t.Fatalf("create subtask: %v", err)
	}

	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/subtasks/"+subtask.SubtaskID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Subtask model.Subtask `json:"subtask"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode subtask detail: %v", err)
	}
	if payload.Subtask.Goal != "Recover narrowly" || payload.Subtask.TaskID != task.TaskID {
		t.Fatalf("unexpected subtask: %+v", payload.Subtask)
	}

	missing := httptest.NewRecorder()
	api.New(database).ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/subtasks/ST-99999", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing subtask status = %d", missing.Code)
	}
}
