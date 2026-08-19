// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestPulseGuardianRenewalPreservesRegisteredGuardianUntilExplicitlyCleared(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	guardianURL := "http://127.0.0.1:19888/pulse"
	sprintID := "SP-10004"
	first, err := database.ReportAgentProgress(ctx, "agent:focus", model.AgentPulseProgressInput{
		ActiveSprintID:       &sprintID,
		LeaseDurationSeconds: 30,
		GuardianURL:          &guardianURL,
	})
	if err != nil {
		t.Fatalf("register Guardian: %v", err)
	}
	if first.GuardianURL != guardianURL || first.ActiveSprintID != sprintID {
		t.Fatalf("registered progress = %#v", first)
	}

	renewed, err := database.ReportAgentProgress(ctx, "agent:focus", model.AgentPulseProgressInput{LeaseDurationSeconds: 30})
	if err != nil {
		t.Fatalf("renew progress: %v", err)
	}
	if renewed.GuardianURL != guardianURL || renewed.ActiveSprintID != sprintID {
		t.Fatalf("renewal must preserve registered context, got %#v", renewed)
	}

	empty := ""
	cleared, err := database.ReportAgentProgress(ctx, "agent:focus", model.AgentPulseProgressInput{
		ActiveSprintID:       &empty,
		LeaseDurationSeconds: 30,
		GuardianURL:          &empty,
	})
	if err != nil {
		t.Fatalf("clear Guardian: %v", err)
	}
	if cleared.GuardianURL != "" || cleared.ActiveSprintID != "" {
		t.Fatalf("explicit clear = %#v", cleared)
	}
}
