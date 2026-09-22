package store

import (
	"context"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// pulseFixture builds the chain the Guardian needs: project, category,
// task, and a started sprint of the given estimate.
func pulseFixture(t *testing.T, database *Store, estimateSeconds int64) (sprintID string, startedAt time.Time) {
	t.Helper()
	ctx := context.Background()

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "pulse fixture"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "main"})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{
		CategoryID: category.CategoryID, Name: "work",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{
		Name: "implementation", EstimatedDurationSeconds: estimateSeconds,
	})
	if err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	started, err := database.TransitionSprint(ctx, sprint.SprintID, "start", "")
	if err != nil {
		t.Fatalf("TransitionSprint: %v", err)
	}
	if started.StartedAt == nil {
		t.Fatalf("sprint started without a start timestamp")
	}
	return sprint.SprintID, *started.StartedAt
}

// TestSprintExtensionLimitIsThree pins the rule Seb relies on: three
// extensions are allowed and the fourth is refused, so work is held and
// reassessed rather than extended indefinitely.
func TestSprintExtensionLimitIsThree(t *testing.T) {
	database := openPulseStore(t)
	ctx := context.Background()
	sprintID, _ := pulseFixture(t, database, 600)

	in := model.CreateSprintTimeExtensionInput{DurationSeconds: 60, Reason: "more work"}
	for i := 1; i <= 3; i++ {
		if _, err := database.AddSprintTimeExtension(ctx, sprintID, in); err != nil {
			t.Fatalf("extension %d should be allowed: %v", i, err)
		}
	}
	if _, err := database.AddSprintTimeExtension(ctx, sprintID, in); err == nil {
		t.Fatalf("fourth extension was allowed, want refusal at 3")
	}
}

// TestSprintExtensionBuysTime proves an extension actually defers the
// overdue nudge rather than being recorded and ignored.
func TestSprintExtensionBuysTime(t *testing.T) {
	database := openPulseStore(t)
	ctx := context.Background()
	sprintID, startedAt := pulseFixture(t, database, 60)

	if _, err := database.AddSprintTimeExtension(ctx, sprintID, model.CreateSprintTimeExtensionInput{
		DurationSeconds: 60, Reason: "verified additional work",
	}); err != nil {
		t.Fatalf("AddSprintTimeExtension: %v", err)
	}

	// Past the original 60s estimate but inside the extension.
	nudges, err := database.EvaluatePulseGuardianAt(ctx, startedAt.Add(90*time.Second))
	if err != nil {
		t.Fatalf("EvaluatePulseGuardianAt: %v", err)
	}
	for _, n := range nudges {
		if n.Kind == "sprint_overdue" {
			t.Fatalf("sprint nudged as overdue while an extension was still running")
		}
	}
}

// TestHeldSprintIsNeverNudged pins the hold escape hatch: once work is
// held for input, the Guardian must stop pulsing on it.
func TestHeldSprintIsNeverNudged(t *testing.T) {
	database := openPulseStore(t)
	ctx := context.Background()
	sprintID, startedAt := pulseFixture(t, database, 60)

	if _, err := database.TransitionSprint(ctx, sprintID, "hold", "Waiting for input"); err != nil {
		t.Fatalf("TransitionSprint hold: %v", err)
	}
	nudges, err := database.EvaluatePulseGuardianAt(ctx, startedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("EvaluatePulseGuardianAt: %v", err)
	}
	if len(nudges) != 0 {
		t.Fatalf("held sprint produced %d nudges, want 0", len(nudges))
	}
}
