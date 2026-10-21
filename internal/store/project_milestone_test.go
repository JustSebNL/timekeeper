// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectMilestonesPersistWithScopedTaskLinksAndAuditedStatus(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(t.TempDir() + "/project-milestones.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Milestone owner"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Deliver"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Other owner"})
	if err != nil {
		t.Fatal(err)
	}
	otherCategory, err := database.CreateCategory(ctx, other.ProjectID, model.CreateCategoryInput{Name: "Other work"})
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err := database.CreateTask(ctx, other.ProjectID, model.CreateTaskInput{CategoryID: otherCategory.CategoryID, Name: "Out of scope"})
	if err != nil {
		t.Fatal(err)
	}
	target := time.Now().UTC().AddDate(0, 0, 5).Format("2006-01-02")
	milestone, err := database.CreateProjectMilestone(ctx, project.ProjectID, model.CreateProjectMilestoneInput{
		Name: "Validated release", Description: "Acceptance suite passes", TargetDate: target, LinkedTaskID: task.TaskID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if milestone.ProjectID != project.ProjectID || milestone.LinkedTaskID != task.TaskID || milestone.Status != model.MilestonePlanned {
		t.Fatalf("created milestone=%+v", milestone)
	}
	listed, err := database.ListProjectMilestones(ctx, project.ProjectID)
	if err != nil || len(listed) != 1 || listed[0].MilestoneID != milestone.MilestoneID {
		t.Fatalf("milestones=%+v err=%v", listed, err)
	}
	crossProjectTask := otherTask.TaskID
	if _, err := database.UpdateProjectMilestone(ctx, project.ProjectID, milestone.MilestoneID, model.UpdateProjectMilestoneInput{LinkedTaskID: &crossProjectTask}); !errors.Is(err, store.ErrValidation) {
		t.Fatalf("cross-Project task link error=%v", err)
	}
	newTarget := time.Now().UTC().AddDate(0, 0, 6).Format("2006-01-02")
	updatedMilestone, err := database.UpdateProjectMilestone(ctx, project.ProjectID, milestone.MilestoneID, model.UpdateProjectMilestoneInput{TargetDate: &newTarget})
	if err != nil || updatedMilestone.TargetDate != newTarget {
		t.Fatalf("updated milestone=%+v err=%v", updatedMilestone, err)
	}
	started, err := database.UpdateProjectMilestoneStatus(ctx, project.ProjectID, milestone.MilestoneID, model.UpdateProjectMilestoneStatusInput{Status: model.MilestoneInProgress})
	if err != nil || started.Status != model.MilestoneInProgress {
		t.Fatalf("started milestone=%+v err=%v", started, err)
	}
	completed, err := database.UpdateProjectMilestoneStatus(ctx, project.ProjectID, milestone.MilestoneID, model.UpdateProjectMilestoneStatusInput{Status: model.MilestoneCompleted})
	if err != nil || completed.Status != model.MilestoneCompleted || completed.CompletedAt == nil || completed.DeadlineState != "completed" {
		t.Fatalf("completed milestone=%+v err=%v", completed, err)
	}
	events, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	milestoneEvents := 0
	for _, event := range events {
		if event.EntityID == milestone.MilestoneID {
			milestoneEvents++
		}
	}
	if milestoneEvents < 4 {
		t.Fatalf("milestone audit events=%d all=%+v", milestoneEvents, events)
	}
	dateChangeHasHistory := false
	for _, event := range events {
		if event.EntityID == milestone.MilestoneID && strings.Contains(event.Message, target) && strings.Contains(event.Message, newTarget) {
			dateChangeHasHistory = true
		}
	}
	if !dateChangeHasHistory {
		t.Fatalf("milestone date change must preserve old/new dates in history; events=%+v", events)
	}
}

func TestMilestoneDeadlineUsesProjectWarningWindow(t *testing.T) {
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	milestone := model.ProjectMilestone{Status: model.MilestonePlanned, TargetDate: "2026-10-12"}
	state, days := milestone.ScheduleState(now, "UTC", 2)
	if state != "due_soon" || days == nil || *days != 2 {
		t.Fatalf("deadline state=%s days=%v", state, days)
	}
	milestone.Status = model.MilestoneCancelled
	state, days = milestone.ScheduleState(now, "UTC", 2)
	if state != "cancelled" || days != nil {
		t.Fatalf("cancelled state=%s days=%v", state, days)
	}
}
