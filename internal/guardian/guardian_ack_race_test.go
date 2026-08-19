// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package guardian_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestTickToleratesCallbackAcknowledgingNudgeBeforeDeliveryEvidenceIsRecorded(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var signal model.PulseGuardianSignal
		if err := json.NewDecoder(r.Body).Decode(&signal); err != nil {
			t.Errorf("decode signal: %v", err)
			http.Error(w, "bad signal", http.StatusBadRequest)
			return
		}
		if _, err := database.AcknowledgePulseNudge(r.Context(), signal.Nudge.NudgeID, signal.Nudge.AgentID); err != nil {
			t.Errorf("acknowledge from callback: %v", err)
			http.Error(w, "ack failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Timekeeper-Pulse-Accepted", "v1")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(callback.Close)

	progress, err := database.ReportAgentProgress(context.Background(), "agent:focus", model.AgentPulseProgressInput{
		LeaseDurationSeconds: 1,
		GuardianURL:          stringPtr(callback.URL + "/pulse"),
	})
	if err != nil {
		t.Fatalf("report progress: %v", err)
	}
	result, err := guardian.Tick(context.Background(), database, progress.LastProgressAt.Add(2*time.Second))
	if err != nil {
		t.Fatalf("tick must tolerate callback acknowledgement race: %v", err)
	}
	if result.Created != 1 || result.DeliveryAttempts != 1 || result.Delivered != 1 {
		t.Fatalf("tick result = %#v", result)
	}
	pending, err := database.ListPendingPulseNudges(context.Background(), "agent:focus")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("acknowledged callback left pending nudge: %#v", pending)
	}
}
