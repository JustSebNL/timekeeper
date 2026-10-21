// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectScheduleDatesAndLifecycleRoundTrip(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(t.TempDir() + "/project-lifecycle.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	planned := now.AddDate(0, 0, 1).Format("2006-01-02")
	due := now.AddDate(0, 0, 10).Format("2006-01-02")
	project, err := database.CreateProject(ctx, model.CreateProjectInput{
		Name: "Lifecycle dates", PlannedStartDate: planned, TargetEndDate: due,
		ScheduleTimezone: "UTC", DeadlineWarningDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.LifecyclePhase != model.ProjectPhaseIntake || project.PlannedStartDate != planned || project.TargetEndDate != due || project.ScheduleTimezone != "UTC" {
		t.Fatalf("created schedule = %+v", project)
	}
	state, days := project.ScheduleState(now)
	if state != "on_track" || days == nil || *days != 10 {
		t.Fatalf("deadline state = %s, days=%v", state, days)
	}

	badDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	if _, err := database.UpdateProjectSchedule(ctx, project.ProjectID, model.UpdateProjectScheduleInput{TargetEndDate: &badDate}); !errors.Is(err, store.ErrValidation) {
		t.Fatalf("invalid date order error = %v", err)
	}
	warning := 14
	updated, err := database.UpdateProjectSchedule(ctx, project.ProjectID, model.UpdateProjectScheduleInput{DeadlineWarningDays: &warning})
	if err != nil || updated.DeadlineWarningDays != warning {
		t.Fatalf("updated schedule=%+v err=%v", updated, err)
	}

	planning, err := database.TransitionProjectPhase(ctx, project.ProjectID, model.TransitionProjectPhaseInput{Phase: model.ProjectPhasePlanning, Reason: "Scope and milestones agreed"})
	if err != nil || planning.LifecyclePhase != model.ProjectPhasePlanning {
		t.Fatalf("planning phase=%+v err=%v", planning, err)
	}
	execution, err := database.TransitionProjectPhase(ctx, project.ProjectID, model.TransitionProjectPhaseInput{Phase: model.ProjectPhaseExecution, Reason: "Begin delivery"})
	if err != nil || execution.LifecyclePhase != model.ProjectPhaseExecution || execution.ActualStartedAt == nil {
		t.Fatalf("execution phase=%+v err=%v", execution, err)
	}
	if _, err := database.TransitionProjectPhase(ctx, project.ProjectID, model.TransitionProjectPhaseInput{Phase: model.ProjectPhasePlanning}); !errors.Is(err, store.ErrValidation) {
		t.Fatalf("backward transition without reason error=%v", err)
	}
	planning, err = database.TransitionProjectPhase(ctx, project.ProjectID, model.TransitionProjectPhaseInput{Phase: model.ProjectPhasePlanning, Reason: "Replan after a dependency change"})
	if err != nil {
		t.Fatalf("replan transition: %v", err)
	}
	execution, err = database.TransitionProjectPhase(ctx, project.ProjectID, model.TransitionProjectPhaseInput{Phase: model.ProjectPhaseExecution})
	if err != nil || execution.ActualStartedAt == nil {
		t.Fatalf("resumed execution phase=%+v err=%v", execution, err)
	}
	validation, err := database.TransitionProjectPhase(ctx, project.ProjectID, model.TransitionProjectPhaseInput{Phase: model.ProjectPhaseValidation})
	if err != nil || validation.LifecyclePhase != model.ProjectPhaseValidation {
		t.Fatalf("validation phase=%+v err=%v", validation, err)
	}
	closeout, err := database.TransitionProjectPhase(ctx, project.ProjectID, model.TransitionProjectPhaseInput{Phase: model.ProjectPhaseCloseout})
	if err != nil || closeout.LifecyclePhase != model.ProjectPhaseCloseout {
		t.Fatalf("closeout phase=%+v err=%v", closeout, err)
	}
	if _, err := database.UpdateProjectStatus(ctx, project.ProjectID, model.UpdateProjectStatusInput{Status: "Completed"}); !errors.Is(err, store.ErrValidation) {
		t.Fatalf("completion without evidence error=%v", err)
	}
	completed, err := database.UpdateProjectStatus(ctx, project.ProjectID, model.UpdateProjectStatusInput{Status: "Completed", Reason: "Acceptance suite passed and release artifact recorded"})
	if err != nil || completed.Status != "Completed" || completed.LifecyclePhase != model.ProjectPhaseCloseout || completed.CompletedAt == nil {
		t.Fatalf("completed Project=%+v err=%v", completed, err)
	}
	state, days = completed.ScheduleState(now)
	if state != "completed" || days != nil {
		t.Fatalf("completed deadline state = %s, days=%v", state, days)
	}

	cancelledProject, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Cancelled lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpdateProjectStatus(ctx, cancelledProject.ProjectID, model.UpdateProjectStatusInput{Status: "Cancelled"}); !errors.Is(err, store.ErrValidation) {
		t.Fatalf("cancellation without reason error=%v", err)
	}
	cancelled, err := database.UpdateProjectStatus(ctx, cancelledProject.ProjectID, model.UpdateProjectStatusInput{Status: "Cancelled", Reason: "Project superseded by successor"})
	if err != nil || cancelled.CancelledAt == nil {
		t.Fatalf("cancelled Project=%+v err=%v", cancelled, err)
	}
	state, days = cancelled.ScheduleState(now)
	if state != "cancelled" || days != nil {
		t.Fatalf("cancelled Project deadline state = %s, days=%v", state, days)
	}
}

func TestProjectDeadlineStatesUseCalendarDatesAndConfiguredWarningWindow(t *testing.T) {
	now := time.Date(2026, 10, 10, 23, 0, 0, 0, time.UTC)
	project := model.Project{Status: "Open", TargetEndDate: "2026-10-11", ScheduleTimezone: "UTC", DeadlineWarningDays: 3}
	state, days := project.ScheduleState(now)
	if state != "due_soon" || days == nil || *days != 1 {
		t.Fatalf("due-soon state=%s days=%v", state, days)
	}
	project.TargetEndDate = "2026-10-09"
	state, days = project.ScheduleState(now)
	if state != "overdue" || days == nil || *days != -1 {
		t.Fatalf("overdue state=%s days=%v", state, days)
	}
	project.Status = "On Hold"
	state, _ = project.ScheduleState(now)
	if state != "overdue" {
		t.Fatalf("held Project deadline state=%s, want overdue", state)
	}
	project.Status = "Cancelled"
	state, days = project.ScheduleState(now)
	if state != "cancelled" || days != nil {
		t.Fatalf("cancelled deadline state=%s days=%v", state, days)
	}
}
