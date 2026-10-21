// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestGuardianStatusReportsRegisteredCallbacks(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	guardianURL := "http://127.0.0.1:1619/v1/recover"
	if _, err := database.ReportAgentProgress(context.Background(), "xatia", model.AgentPulseProgressInput{
		LeaseDurationSeconds: 1800,
		GuardianURL:          &guardianURL,
	}); err != nil {
		t.Fatalf("register guardian url: %v", err)
	}
	recoveryURL := "http://127.0.0.1:1618/api/v1/guardian/recover"
	if _, err := database.ReportAgentProgress(context.Background(), "canonical-agent", model.AgentPulseProgressInput{
		LeaseDurationSeconds: 1800,
		GuardianURL:          &recoveryURL,
	}); err != nil {
		t.Fatalf("register canonical guardian url: %v", err)
	}
	workerURL := "http://127.0.0.1:1619/pulse"
	if _, err := database.ReportAgentProgress(context.Background(), "worker-agent", model.AgentPulseProgressInput{
		LeaseDurationSeconds: 1800,
		GuardianURL:          &workerURL,
	}); err != nil {
		t.Fatalf("register worker guardian url: %v", err)
	}

	handler := api.NewWithRuntime(database, api.RuntimeStatus{
		PulseGuardianEnabled:         true,
		PulseGuardianIntervalSeconds: 300,
		RecoveryPolicy:               "local-artifact",
		GuardianRecoveryURL:          recoveryURL,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/guardian/status", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("guardian status status = %d; body = %s", rec.Code, rec.Body.String())
	}
	var status struct {
		PulseGuardianEnabled bool                       `json:"pulse_guardian_enabled"`
		RecoveryPolicy       string                     `json:"recovery_policy"`
		GuardianRecoveryURL  string                     `json:"guardian_recovery_url"`
		RegisteredCallbacks  []model.RegisteredGuardian `json:"registered_callbacks"`
		CallbackMigration    struct {
			Registered                  int  `json:"registered"`
			CanonicalRecovery           int  `json:"canonical_recovery"`
			LegacyRecovery              int  `json:"legacy_recovery"`
			WorkerOrCustom              int  `json:"worker_or_custom"`
			LegacyReceiverRequired      bool `json:"legacy_receiver_required"`
			CanonicalRecoveryConfigured bool `json:"canonical_recovery_configured"`
		} `json:"callback_migration"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode guardian status: %v", err)
	}
	if !status.PulseGuardianEnabled {
		t.Fatalf("expected guardian enabled")
	}
	if status.RecoveryPolicy != "local-artifact" {
		t.Fatalf("expected recovery policy, got %q", status.RecoveryPolicy)
	}
	if status.GuardianRecoveryURL != recoveryURL {
		t.Fatalf("expected canonical recovery URL %q, got %q", recoveryURL, status.GuardianRecoveryURL)
	}
	if len(status.RegisteredCallbacks) != 3 {
		t.Fatalf("expected three registered callbacks, got %#v", status.RegisteredCallbacks)
	}
	if status.CallbackMigration.Registered != 3 || status.CallbackMigration.CanonicalRecovery != 1 || status.CallbackMigration.LegacyRecovery != 1 || status.CallbackMigration.WorkerOrCustom != 1 {
		t.Fatalf("unexpected callback migration telemetry: %#v", status.CallbackMigration)
	}
	if !status.CallbackMigration.LegacyReceiverRequired || !status.CallbackMigration.CanonicalRecoveryConfigured {
		t.Fatalf("unexpected migration flags: %#v", status.CallbackMigration)
	}
}

func TestCanonicalGuardianRecoveryWritesArtifactAndAcknowledges(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	activeSprintID := "SP-10004"
	progress, err := database.ReportAgentProgress(context.Background(), "xatia", model.AgentPulseProgressInput{
		ActiveSprintID:       &activeSprintID,
		LeaseDurationSeconds: 1,
	})
	if err != nil {
		t.Fatalf("register progress: %v", err)
	}
	created, err := database.EvaluatePulseGuardianAt(context.Background(), progress.LastProgressAt.Add(2*time.Second))
	if err != nil || len(created) != 1 {
		t.Fatalf("create expired nudge = %#v, %v", created, err)
	}

	stateDir := t.TempDir()
	recovery, err := guardian.NewRecoveryHandler(guardian.RecoveryHandlerConfig{
		StateDir:    stateDir,
		Action:      guardian.RecoveryActionLocalArtifact,
		PolicyLabel: "local-artifact",
		Acknowledge: func(ctx context.Context, nudge model.PulseNudge) error {
			_, err := database.AcknowledgePulseNudge(ctx, nudge.NudgeID, nudge.AgentID)
			return err
		},
	})
	if err != nil {
		t.Fatalf("create recovery handler: %v", err)
	}
	handler := api.NewWithGuardianRecovery(database, api.RuntimeStatus{
		GuardianRecoveryURL: "http://127.0.0.1:1618/api/v1/guardian/recover",
	}, nil, nil, nil, recovery)

	signal := model.PulseGuardianSignal{
		Format: "timekeeper-pulse-guardian/v1",
		Action: "recover_attention",
		Nudge:  created[0],
	}
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatalf("encode signal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/guardian/recover", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Header().Get("X-Timekeeper-Pulse-Accepted") != "v1" {
		t.Fatalf("recovery response = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}

	artifact := filepath.Join(stateDir, "recovery", "xatia", fmt.Sprintf("%d.json", created[0].NudgeID))
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("recovery artifact not written: %v", err)
	}
	pending, err := database.ListPendingPulseNudges(context.Background(), "xatia")
	if err != nil {
		t.Fatalf("list pending nudges: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("nudge remained pending: %#v", pending)
	}
}

func TestCanonicalGuardianRecoveryRequiresConfiguration(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/guardian/recover", bytes.NewBufferString(`{"format":"timekeeper-pulse-guardian/v1"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured recovery status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Error.Code != "guardian_recovery_unavailable" {
		t.Fatalf("unconfigured recovery response = %s, err=%v", response.Body.String(), err)
	}
}
