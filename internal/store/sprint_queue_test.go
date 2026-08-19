// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestClaimNextSprintStartsOldestRunnableWorkAndCompletingItClosesItsTask(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Queue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Execution"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "First outcome"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	first, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "First", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create first sprint: %v", err)
	}
	_, err = database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Second", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create second sprint: %v", err)
	}

	claimed, err := database.ClaimNextSprint(ctx, project.ProjectID)
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if claimed.SprintID != first.SprintID || claimed.Status != "Active" {
		t.Fatalf("claimed = %#v", claimed)
	}
	if _, err = database.TransitionSprint(ctx, claimed.SprintID, "complete", "finished"); err != nil {
		t.Fatalf("complete first: %v", err)
	}
	unchanged, err := database.GetTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("read task after first: %v", err)
	}
	if unchanged.Status != "Open" {
		t.Fatalf("task closed with queued work: %#v", unchanged)
	}
	claimed, err = database.ClaimNextSprint(ctx, project.ProjectID)
	if err != nil {
		t.Fatalf("claim second: %v", err)
	}
	if _, err = database.TransitionSprint(ctx, claimed.SprintID, "complete", "finished"); err != nil {
		t.Fatalf("complete second: %v", err)
	}
	completed, err := database.GetTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("read task after queue exhausted: %v", err)
	}
	if completed.Status != "Completed" {
		t.Fatalf("task did not advance after final direct sprint: %#v", completed)
	}
}
