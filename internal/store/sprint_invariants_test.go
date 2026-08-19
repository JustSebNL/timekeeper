// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func newSprintInvariantFixture(t *testing.T) (*store.Store, context.Context, model.Project, model.Category, model.Task) {
	t.Helper()
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Invariant project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Execution"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Bounded work"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return database, ctx, project, category, task
}

func TestCreateSprintRejectsCompletedParent(t *testing.T) {
	database, ctx, _, _, task := newSprintInvariantFixture(t)
	if _, err := database.UpdateTaskStatus(ctx, task.TaskID, model.UpdateTaskStatusInput{Status: "Completed"}); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	if _, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Late work", EstimatedDurationSeconds: 60}); err == nil {
		t.Fatal("creating an Open Sprint below a Completed Task must fail")
	}
}

func TestTaskCannotCompleteWhileItOwnsNonTerminalSprint(t *testing.T) {
	database, ctx, _, _, task := newSprintInvariantFixture(t)
	if _, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Still open", EstimatedDurationSeconds: 60}); err != nil {
		t.Fatalf("create sprint: %v", err)
	}
	if _, err := database.UpdateTaskStatus(ctx, task.TaskID, model.UpdateTaskStatusInput{Status: "Completed"}); err == nil {
		t.Fatal("task with an Open Sprint must not complete")
	}
}

func TestOpenSprintCanWaitWithoutStartingTheWorkClock(t *testing.T) {
	database, ctx, _, _, task := newSprintInvariantFixture(t)
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Wait for consent", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, sprint.SprintID, "hold", ""); err == nil {
		t.Fatal("holding a Sprint must retain a broad reason")
	}
	held, err := database.TransitionSprint(ctx, sprint.SprintID, "hold", "waiting_for_user: OAuth consent")
	if err != nil {
		t.Fatalf("hold Open Sprint: %v", err)
	}
	if held.Status != "On Hold" || held.HoldReason != "waiting_for_user: OAuth consent" || held.StartedAt != nil || held.ActiveDurationSeconds != 0 {
		t.Fatalf("held sprint must retain waiting reason without creating active time: %#v", held)
	}
	entries, err := database.ListTimeEntries(ctx, sprint.SprintID)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Open -> On Hold must not write a time entry: %#v", entries)
	}
}

func TestTaskHasAtMostFourSprints(t *testing.T) {
	database, ctx, _, _, task := newSprintInvariantFixture(t)
	for i := 0; i < 4; i++ {
		if _, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: fmt.Sprintf("Bounded Sprint %d", i+1), EstimatedDurationSeconds: 60}); err != nil {
			t.Fatalf("create sprint %d: %v", i+1, err)
		}
	}
	if _, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Too many", EstimatedDurationSeconds: 60}); err == nil {
		t.Fatal("fifth Sprint must be rejected")
	}
}
