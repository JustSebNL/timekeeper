// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
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

	handler := api.NewWithRuntime(database, api.RuntimeStatus{
		PulseGuardianEnabled:         true,
		PulseGuardianIntervalSeconds: 300,
		RecoveryPolicy:               "local-artifact",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/guardian/status", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("guardian status status = %d; body = %s", rec.Code, rec.Body.String())
	}
	var status struct {
		PulseGuardianEnabled bool `json:"pulse_guardian_enabled"`
		RecoveryPolicy        string `json:"recovery_policy"`
		RegisteredCallbacks  []model.RegisteredGuardian `json:"registered_callbacks"`
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
	if len(status.RegisteredCallbacks) != 1 || status.RegisteredCallbacks[0].GuardianURL != guardianURL {
		t.Fatalf("expected registered callback, got %#v", status.RegisteredCallbacks)
	}
}
