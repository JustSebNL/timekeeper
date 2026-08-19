// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestPulseAtReportsAnActiveSprintThatHasExceededItsPlannedBudget(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Pulse"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Scheduling"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Surface overdue work"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Short window", EstimatedDurationSeconds: 60, BufferPct: 50})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}
	started, err := database.TransitionSprint(ctx, sprint.SprintID, "start", "")
	if err != nil {
		t.Fatalf("start sprint: %v", err)
	}
	if started.StartedAt == nil {
		t.Fatal("started sprint has no start time")
	}
	before, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatalf("list events before pulse: %v", err)
	}

	pulse, err := database.PulseAt(ctx, started.StartedAt.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("pulse: %v", err)
	}
	if pulse.Format != "timekeeper-pulse/v1" || !pulse.GeneratedAt.Equal(started.StartedAt.Add(2*time.Minute)) || pulse.RecommendedNextPulseSeconds != 60 {
		t.Fatalf("pulse metadata = %#v", pulse)
	}
	if len(pulse.Attention) != 1 {
		t.Fatalf("attention = %#v", pulse.Attention)
	}
	item := pulse.Attention[0]
	if item.Kind != "sprint_overdue" || item.ProjectID != project.ProjectID || item.SprintID != sprint.SprintID || item.Status != "Active" || item.PlannedDurationSeconds != 90 || item.ActiveDurationSeconds != 120 || item.OverdueDurationSeconds != 30 {
		t.Fatalf("attention item = %#v", item)
	}
	after, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatalf("list events after pulse: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("pulse must not create durable events: before=%d after=%d", len(before), len(after))
	}
}

func TestPulseAtHonorsExtensionsAndExcludesHeldSprints(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Pulse accounting"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Scheduling"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Respect plan changes"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	extended, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Extended Sprint", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create extended sprint: %v", err)
	}
	started, err := database.TransitionSprint(ctx, extended.SprintID, "start", "")
	if err != nil {
		t.Fatalf("start extended sprint: %v", err)
	}
	if started.StartedAt == nil {
		t.Fatal("started extended sprint has no start time")
	}
	if _, err := database.AddSprintTimeExtension(ctx, extended.SprintID, model.CreateSprintTimeExtensionInput{DurationSeconds: 60, Reason: "One more bounded minute"}); err != nil {
		t.Fatalf("add extension: %v", err)
	}

	held, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Held Sprint", EstimatedDurationSeconds: 1})
	if err != nil {
		t.Fatalf("create held sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, held.SprintID, "start", ""); err != nil {
		t.Fatalf("start held sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, held.SprintID, "hold", "pause until a dependency is ready"); err != nil {
		t.Fatalf("hold sprint: %v", err)
	}

	atPlan, err := database.PulseAt(ctx, started.StartedAt.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("pulse at plan: %v", err)
	}
	if len(atPlan.Attention) != 0 {
		t.Fatalf("extension should keep exact-plan work clear: %#v", atPlan.Attention)
	}
	pastPlan, err := database.PulseAt(ctx, started.StartedAt.Add(2*time.Minute+time.Second))
	if err != nil {
		t.Fatalf("pulse past plan: %v", err)
	}
	if len(pastPlan.Attention) != 1 || pastPlan.Attention[0].SprintID != extended.SprintID || pastPlan.Attention[0].OverdueDurationSeconds != 1 {
		t.Fatalf("attention after extension plan = %#v", pastPlan.Attention)
	}
}
