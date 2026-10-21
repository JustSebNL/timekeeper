// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectDeadlineWatchAggregatesOpenProjectsAndMilestones(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(t.TempDir() + "/project-deadlines.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	projectSoon, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Project due soon", TargetEndDate: now.AddDate(0, 0, 2).Format("2006-01-02")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProjectMilestone(ctx, projectSoon.ProjectID, model.CreateProjectMilestoneInput{Name: "Milestone due soon", TargetDate: now.AddDate(0, 0, 1).Format("2006-01-02")}); err != nil {
		t.Fatal(err)
	}
	projectOverdue, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Project overdue", TargetEndDate: now.AddDate(0, 0, -2).Format("2006-01-02")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProjectMilestone(ctx, projectOverdue.ProjectID, model.CreateProjectMilestoneInput{Name: "Milestone overdue", TargetDate: now.AddDate(0, 0, -3).Format("2006-01-02")}); err != nil {
		t.Fatal(err)
	}
	completed, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Closed Project", TargetEndDate: now.AddDate(0, 0, -5).Format("2006-01-02")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpdateProjectStatus(ctx, completed.ProjectID, model.UpdateProjectStatusInput{Status: "Completed", Reason: "Work accepted"}); err != nil {
		t.Fatal(err)
	}
	watch, err := database.ProjectDeadlineWatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(watch.Items) != 4 {
		t.Fatalf("deadline items=%+v", watch.Items)
	}
	states := map[string]int{}
	for _, item := range watch.Items {
		states[item.DeadlineState]++
		if item.ProjectID == completed.ProjectID {
			t.Fatalf("completed Project leaked into deadline watch: %+v", item)
		}
	}
	if states["overdue"] != 2 || states["due_soon"] != 2 {
		t.Fatalf("deadline state counts=%v items=%+v", states, watch.Items)
	}
	if watch.Items[0].DeadlineState != "overdue" || watch.Items[1].DeadlineState != "overdue" {
		t.Fatalf("overdue items must sort ahead of due-soon items: %+v", watch.Items)
	}
}
