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

func TestPulseGuardianCreatesOneDurableNudgeWhenAgentProgressLeaseExpires(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	sprintID := "SP-10004"
	progress, err := database.ReportAgentProgress(ctx, "agent:focus", model.AgentPulseProgressInput{
		LeaseDurationSeconds: 5,
		ActiveSprintID:       &sprintID,
	})
	if err != nil {
		t.Fatalf("report progress: %v", err)
	}

	beforeExpiry, err := database.EvaluatePulseGuardianAt(ctx, progress.LastProgressAt.Add(5*time.Second))
	if err != nil {
		t.Fatalf("evaluate at expiry boundary: %v", err)
	}
	if len(beforeExpiry) != 0 {
		t.Fatalf("nudge at expiry boundary = %#v", beforeExpiry)
	}

	nudges, err := database.EvaluatePulseGuardianAt(ctx, progress.LastProgressAt.Add(6*time.Second))
	if err != nil {
		t.Fatalf("evaluate after expiry: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("new nudges = %#v", nudges)
	}
	nudge := nudges[0]
	if nudge.Kind != "agent_unresponsive" || nudge.Status != "Pending" || nudge.AgentID != "agent:focus" || nudge.ActiveSprintID != "SP-10004" || nudge.DetectedAfterSeconds != 6 {
		t.Fatalf("nudge = %#v", nudge)
	}

	repeated, err := database.EvaluatePulseGuardianAt(ctx, progress.LastProgressAt.Add(30*time.Second))
	if err != nil {
		t.Fatalf("repeat evaluation: %v", err)
	}
	if len(repeated) != 0 {
		t.Fatalf("repeat evaluation must not duplicate a pending nudge: %#v", repeated)
	}
	pending, err := database.ListPendingPulseNudges(ctx, "agent:focus")
	if err != nil {
		t.Fatalf("list pending nudges: %v", err)
	}
	if len(pending) != 1 || pending[0].NudgeID != nudge.NudgeID {
		t.Fatalf("pending = %#v", pending)
	}

	acknowledged, err := database.AcknowledgePulseNudge(ctx, nudge.NudgeID, "agent:focus")
	if err != nil {
		t.Fatalf("acknowledge nudge: %v", err)
	}
	if acknowledged.Status != "Acknowledged" || acknowledged.AcknowledgedAt == nil {
		t.Fatalf("acknowledged nudge = %#v", acknowledged)
	}
	pending, err = database.ListPendingPulseNudges(ctx, "agent:focus")
	if err != nil {
		t.Fatalf("list pending after acknowledgement: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after acknowledgement = %#v", pending)
	}
}
