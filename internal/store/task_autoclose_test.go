// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestTaskAutoCloseDefaultPolicy(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "AutoClose Default Test"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Core"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{
		CategoryID:               category.CategoryID,
		Name:                     "Default AutoClose Task",
		EstimatedDurationSeconds: 600,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if !task.AutoClose {
		t.Fatalf("expected task.AutoClose to default to true, got false")
	}

	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{
		Name:                     "Sprint 1",
		EstimatedDurationSeconds: 300,
	})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}

	if _, err := database.TransitionSprint(ctx, sprint.SprintID, "start", ""); err != nil {
		t.Fatalf("start sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, sprint.SprintID, "complete", "Done"); err != nil {
		t.Fatalf("complete sprint: %v", err)
	}

	updatedTask, err := database.GetTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.Status != "Completed" {
		t.Fatalf("expected task status to be Completed automatically, got %s", updatedTask.Status)
	}
}

func TestTaskNoAutoClosePolicy(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "No AutoClose Test"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Core"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	falseVal := false
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{
		CategoryID:               category.CategoryID,
		Name:                     "Manual Close Design Task",
		EstimatedDurationSeconds: 600,
		AutoClose:                &falseVal,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.AutoClose {
		t.Fatalf("expected task.AutoClose to be false, got true")
	}

	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{
		Name:                     "Design Sprint",
		EstimatedDurationSeconds: 300,
	})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}

	if _, err := database.TransitionSprint(ctx, sprint.SprintID, "start", ""); err != nil {
		t.Fatalf("start sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, sprint.SprintID, "complete", "Done"); err != nil {
		t.Fatalf("complete sprint: %v", err)
	}

	updatedTask, err := database.GetTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.Status != "Open" {
		t.Fatalf("expected task status to remain Open for --no-autoclose task, got %s", updatedTask.Status)
	}

	// Manually completing task should work
	if _, err := database.UpdateTaskStatus(ctx, task.TaskID, model.UpdateTaskStatusInput{Status: "Completed"}); err != nil {
		t.Fatalf("manual complete task: %v", err)
	}
	finalTask, err := database.GetTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("get final task: %v", err)
	}
	if finalTask.Status != "Completed" {
		t.Fatalf("expected final task status to be Completed, got %s", finalTask.Status)
	}
}
