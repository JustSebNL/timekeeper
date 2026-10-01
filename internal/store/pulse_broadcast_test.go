package store

import (
	"context"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestCreateBroadcastNudgeUsesValidDetectionValue(t *testing.T) {
	database := openPulseStore(t)
	ctx := context.Background()
	if _, err := database.ReportAgentProgress(ctx, "agent:broadcast", model.AgentPulseProgressInput{LeaseDurationSeconds: 60}); err != nil {
		t.Fatalf("ReportAgentProgress: %v", err)
	}

	result, err := database.CreateBroadcastNudge(ctx, "continue authorized work")
	if err != nil {
		t.Fatalf("CreateBroadcastNudge: %v", err)
	}
	if result.Count != 1 || len(result.AgentIDs) != 1 || result.AgentIDs[0] != "agent:broadcast" {
		t.Fatalf("broadcast result=%+v, want one agent", result)
	}
	nudges, err := database.ListPendingPulseNudges(ctx, "agent:broadcast")
	if err != nil {
		t.Fatalf("ListPendingPulseNudges: %v", err)
	}
	if len(nudges) != 1 || nudges[0].DetectedAfterSeconds <= 0 {
		t.Fatalf("pending broadcast nudges=%+v, want positive detection value", nudges)
	}
}
