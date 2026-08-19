// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestPulseGuardianNudgeHistoryRetainsAcknowledgedEvidence(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	server := api.New(database)

	progressRequest := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent:focus/progress", bytes.NewBufferString(`{"lease_duration_seconds":1}`))
	progressRequest.Header.Set("Content-Type", "application/json")
	progressResponse := httptest.NewRecorder()
	server.ServeHTTP(progressResponse, progressRequest)
	if progressResponse.Code != http.StatusCreated {
		t.Fatalf("register progress status = %d: %s", progressResponse.Code, progressResponse.Body.String())
	}
	var progress model.AgentPulseProgress
	if err := json.NewDecoder(progressResponse.Body).Decode(&progress); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	created, err := database.EvaluatePulseGuardianAt(context.Background(), progress.LastProgressAt.Add(2*time.Second))
	if err != nil || len(created) != 1 {
		t.Fatalf("create expired nudge = %#v, %v", created, err)
	}

	ackRequest := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent:focus/nudges/"+strconv.FormatInt(created[0].NudgeID, 10)+"/ack", bytes.NewBufferString(`{}`))
	ackRequest.Header.Set("Content-Type", "application/json")
	ackResponse := httptest.NewRecorder()
	server.ServeHTTP(ackResponse, ackRequest)
	if ackResponse.Code != http.StatusOK {
		t.Fatalf("ack status = %d: %s", ackResponse.Code, ackResponse.Body.String())
	}

	historyResponse := httptest.NewRecorder()
	server.ServeHTTP(historyResponse, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent:focus/nudges/history", nil))
	if historyResponse.Code != http.StatusOK {
		t.Fatalf("history status = %d: %s", historyResponse.Code, historyResponse.Body.String())
	}
	var history struct {
		Items []model.PulseNudge `json:"items"`
	}
	if err := json.NewDecoder(historyResponse.Body).Decode(&history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history.Items) != 1 || history.Items[0].NudgeID != created[0].NudgeID || history.Items[0].Status != "Acknowledged" || history.Items[0].AcknowledgedAt == nil {
		t.Fatalf("nudge history = %#v", history.Items)
	}
}

func TestPulseGuardianAcknowledgementRejectsUnknownInput(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	server := api.New(database)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent:focus/nudges/1/ack", bytes.NewBufferString(`{"unexpected":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown acknowledgement body status = %d: %s", response.Code, response.Body.String())
	}
}
