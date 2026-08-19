// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestProjectAttentionSeparatesHeldTimedOutAndLegacyStrandedWork(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "timekeeper.db"))
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

	newTask := func(name string) model.Task {
		t.Helper()
		task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: name})
		if err != nil {
			t.Fatalf("create task %q: %v", name, err)
		}
		return task
	}
	newSprint := func(task model.Task, name string) model.Sprint {
		t.Helper()
		sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: name, EstimatedDurationSeconds: 60})
		if err != nil {
			t.Fatalf("create sprint %q: %v", name, err)
		}
		return sprint
	}

	held := newSprint(newTask("Hold"), "Await decision")
	if _, err := database.TransitionSprint(ctx, held.SprintID, "hold", "Awaiting product decision"); err != nil {
		t.Fatalf("hold sprint: %v", err)
	}

	timedOut := newSprint(newTask("Retry"), "Retrieval")
	for attempt := 1; attempt <= 4; attempt++ {
		if _, err := database.RecordSprintRetrievalAttempt(ctx, timedOut.SprintID, "retrieval failed"); err != nil {
			t.Fatalf("record attempt %d: %v", attempt, err)
		}
	}

	strandedTask := newTask("Legacy")
	stranded := newSprint(strandedTask, "Legacy open work")
	if _, err := database.db.ExecContext(ctx, "UPDATE tasks SET status = 'Completed' WHERE task_id = ?", strandedTask.TaskID); err != nil {
		t.Fatalf("seed legacy stranded sprint: %v", err)
	}

	attention, err := database.ProjectAttention(ctx, project.ProjectID)
	if err != nil {
		t.Fatalf("project attention: %v", err)
	}
	if attention.ProjectID != project.ProjectID || len(attention.Items) != 3 {
		t.Fatalf("attention = %#v", attention)
	}
	byID := map[string]model.ProjectAttentionItem{}
	for _, item := range attention.Items {
		byID[item.SprintID] = item
	}
	if item := byID[held.SprintID]; item.Kind != "sprint_on_hold" || item.HoldReason != "Awaiting product decision" {
		t.Fatalf("held attention = %#v", item)
	}
	if item := byID[timedOut.SprintID]; item.Kind != "sprint_timed_out" || item.Status != "TimedOut" {
		t.Fatalf("timed out attention = %#v", item)
	}
	if item := byID[stranded.SprintID]; item.Kind != "stranded_open_sprint" || item.Status != "Open" {
		t.Fatalf("stranded attention = %#v", item)
	}
}
