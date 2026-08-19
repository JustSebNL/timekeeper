// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestCancelledSprintRemainsVisibleWithReasonAndNoInventedWork(t *testing.T) {
	database, ctx, _, _, task := newSprintInvariantFixture(t)
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Cancelled work", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, sprint.SprintID, "cancel", ""); err == nil {
		t.Fatal("cancelling without a reason must fail")
	}
	cancelled, err := database.TransitionSprint(ctx, sprint.SprintID, "cancel", "No longer needed after design change")
	if err != nil {
		t.Fatalf("cancel sprint: %v", err)
	}
	if cancelled.Status != "Cancelled" || cancelled.EndedAt == nil || cancelled.ActiveDurationSeconds != 0 {
		t.Fatalf("cancelled sprint = %#v", cancelled)
	}
	entries, err := database.ListTimeEntries(ctx, sprint.SprintID)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("cancelling Open work must not invent entries: %#v", entries)
	}
	listed, err := database.ListSprints(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("list sprints: %v", err)
	}
	if len(listed) != 1 || listed[0].Status != "Cancelled" {
		t.Fatalf("cancelled sprint must remain visible: %#v", listed)
	}
}
