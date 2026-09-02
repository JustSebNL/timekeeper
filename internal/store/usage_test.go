// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestUsageSnapshotsAreIdempotentAndReturnCumulativeDelta(t *testing.T) {
	database, err := store.Open(t.TempDir() + "/usage.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Usage project"})
	if err != nil {
		t.Fatal(err)
	}

	first, err := database.RecordUsageSnapshot(ctx, project.ProjectID, model.UsageSnapshotInput{
		SessionID: "session-1", AgentID: "codex", Model: "gpt-5", TurnSeq: 1,
		InputTokens: 1000, OutputTokens: 200, CacheReadTokens: 50, Messages: 2,
		CapturedAt: time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Delta.InputTokens != 1000 || first.Delta.OutputTokens != 200 || first.Duplicate {
		t.Fatalf("first result = %#v", first)
	}

	second, err := database.RecordUsageSnapshot(ctx, project.ProjectID, model.UsageSnapshotInput{
		SessionID: "session-1", AgentID: "codex", Model: "gpt-5", TurnSeq: 2,
		InputTokens: 1600, OutputTokens: 350, CacheReadTokens: 100, Messages: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Delta.InputTokens != 600 || second.Delta.OutputTokens != 150 || second.Delta.CacheReadTokens != 50 || second.Delta.Messages != 1 {
		t.Fatalf("second delta = %#v", second.Delta)
	}

	duplicate, err := database.RecordUsageSnapshot(ctx, project.ProjectID, model.UsageSnapshotInput{
		SessionID: "session-1", AgentID: "codex", Model: "gpt-5", TurnSeq: 2,
		InputTokens: 1600, OutputTokens: 350, CacheReadTokens: 100, Messages: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.Delta.InputTokens != 0 {
		t.Fatalf("duplicate result = %#v", duplicate)
	}

	summary, err := database.ProjectUsageSummary(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Totals.InputTokens != 1600 || summary.Totals.OutputTokens != 350 || summary.Totals.Messages != 3 || len(summary.Sessions) != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(summary.Days) == 0 || summary.Days[0].InputTokens != 1600 {
		t.Fatalf("usage days = %#v", summary.Days)
	}
}

func TestUsageRejectsOutOfOrderAndCrossProjectSprint(t *testing.T) {
	database, err := store.Open(t.TempDir() + "/usage.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Usage project"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Other usage project"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.RecordUsageSnapshot(ctx, project.ProjectID, model.UsageSnapshotInput{SessionID: "s", AgentID: "agent", TurnSeq: 2, InputTokens: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.RecordUsageSnapshot(ctx, project.ProjectID, model.UsageSnapshotInput{SessionID: "s", AgentID: "agent", TurnSeq: 1, InputTokens: 20}); err == nil {
		t.Fatal("out-of-order snapshot must fail")
	}
	if _, err := database.RecordUsageSnapshot(ctx, other.ProjectID, model.UsageSnapshotInput{SessionID: "s", AgentID: "agent", TurnSeq: 1, SprintID: "SP-does-not-belong"}); err == nil {
		t.Fatal("unknown sprint must fail")
	}
}
