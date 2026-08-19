// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package guardian_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestTickDoesNotRedeliverAnAcceptedNudgeBeforeAcknowledgement(t *testing.T) {
	var calls atomic.Int32
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
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

	first, err := guardian.Tick(context.Background(), database, progress.LastProgressAt.Add(2*time.Second))
	if err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if first.Created != 1 || first.DeliveryAttempts != 1 || first.Delivered != 1 {
		t.Fatalf("first tick = %#v", first)
	}
	second, err := guardian.Tick(context.Background(), database, progress.LastProgressAt.Add(3*time.Second))
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if second.Created != 0 || second.DeliveryAttempts != 0 || second.Delivered != 0 {
		t.Fatalf("second tick must not redeliver accepted nudge: %#v", second)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("callback calls = %d, want 1", got)
	}
}
