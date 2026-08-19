// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"fmt"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestFourthRetrievalAttemptTimesOutSprintWithoutErasingEvidence(t *testing.T) {
	database, ctx, _, _, task := newSprintInvariantFixture(t)
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Bounded retrieval", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		recorded, err := database.RecordSprintRetrievalAttempt(ctx, sprint.SprintID, fmt.Sprintf("attempt %d did not retrieve material progress", attempt))
		if err != nil {
			t.Fatalf("record attempt %d: %v", attempt, err)
		}
		if recorded.AttemptNumber != attempt || recorded.TimedOut {
			t.Fatalf("attempt %d = %#v", attempt, recorded)
		}
	}

	fourth, err := database.RecordSprintRetrievalAttempt(ctx, sprint.SprintID, "attempt 4 did not retrieve material progress")
	if err != nil {
		t.Fatalf("record fourth attempt: %v", err)
	}
	if fourth.AttemptNumber != 4 || !fourth.TimedOut {
		t.Fatalf("fourth attempt = %#v", fourth)
	}
	items, err := database.ListSprints(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("list sprints: %v", err)
	}
	if len(items) != 1 || items[0].Status != "TimedOut" || items[0].ActiveDurationSeconds != 0 {
		t.Fatalf("timed out sprint = %#v", items)
	}
	if _, err := database.RecordSprintRetrievalAttempt(ctx, sprint.SprintID, "fifth attempt"); err == nil {
		t.Fatal("TimedOut Sprint must retain exactly four attempts")
	}
}
