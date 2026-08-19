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

func stringPtr(value string) *string { return &value }

func TestTickDeliversExpiredAgentNudgeToExplicitLoopbackGuardian(t *testing.T) {
	received := make(chan model.PulseNudge, 1)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("callback request = %s %q", r.Method, r.Header.Get("Content-Type"))
			http.Error(w, "invalid callback request", http.StatusBadRequest)
			return
		}
		var signal model.PulseGuardianSignal
		if err := json.NewDecoder(r.Body).Decode(&signal); err != nil {
			t.Errorf("decode callback signal: %v", err)
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		if signal.Format != "timekeeper-pulse-guardian/v1" || signal.Action != "recover_attention" {
			t.Errorf("callback signal contract = %#v", signal)
			http.Error(w, "invalid signal", http.StatusBadRequest)
			return
		}
		received <- signal.Nudge
		w.Header().Set("X-Timekeeper-Pulse-Accepted", "v1")
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
	if result.Created != 1 || result.DeliveryAttempts != 1 || result.Delivered != 1 || result.DeliveryFailures != 0 {
		t.Fatalf("tick result = %#v", result)
	}
	select {
	case nudge := <-received:
		if nudge.AgentID != "agent:focus" || nudge.Kind != "agent_unresponsive" || nudge.Status != "Pending" {
			t.Fatalf("delivered nudge = %#v", nudge)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("guardian callback did not receive a nudge")
	}
}
