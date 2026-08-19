// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package guardian_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestTickDoesNotTreatUnmarkedCallbackResponseAsGuardianAcceptance(t *testing.T) {
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(callback.Close)

	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	progress, err := database.ReportAgentProgress(context.Background(), "agent:focus", model.AgentPulseProgressInput{
		LeaseDurationSeconds: 1,
		GuardianURL:          stringPtr(callback.URL + "/pulse"),
	})
	if err != nil {
		t.Fatalf("report progress: %v", err)
	}

	result, err := guardian.Tick(context.Background(), database, progress.LastProgressAt.Add(2*time.Second))
	if err != nil {
		t.Fatalf("tick guardian: %v", err)
	}
	if result.Created != 1 || result.DeliveryAttempts != 1 || result.Delivered != 0 || result.DeliveryFailures != 1 {
		t.Fatalf("tick result = %#v", result)
	}
	pending, err := database.ListPendingPulseNudges(context.Background(), "agent:focus")
	if err != nil {
		t.Fatalf("list pending nudge: %v", err)
	}
	if len(pending) != 1 || pending[0].DeliveryAttempts != 1 || pending[0].DeliveredAt != nil {
		t.Fatalf("pending nudge = %#v", pending)
	}
}
