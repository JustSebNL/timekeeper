// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestUpdateSprintHoldReasonDoesNotInventAnInterval(t *testing.T) {
	database, ctx, _, _, task := newSprintInvariantFixture(t)
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Waiting", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, sprint.SprintID, "hold", "initial dependency"); err != nil {
		t.Fatalf("hold sprint: %v", err)
	}
	updated, err := database.UpdateSprintHoldReason(ctx, sprint.SprintID, "Awaiting user consent")
	if err != nil {
		t.Fatalf("update hold reason: %v", err)
	}
	if updated.Status != "On Hold" || updated.HoldReason != "Awaiting user consent" || updated.ActiveDurationSeconds != 0 {
		t.Fatalf("updated sprint = %#v", updated)
	}
	entries, err := database.ListTimeEntries(ctx, sprint.SprintID)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("reason update must not create an interval: %#v", entries)
	}
}
