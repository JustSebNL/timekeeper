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

func TestPulseGuardianProgressNudgeAndAcknowledgementAPI(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	server := api.New(database)

	progressRequest := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent:focus/progress", bytes.NewBufferString(`{"active_sprint_id":"SP-10004","lease_duration_seconds":1}`))
	progressRequest.Header.Set("Content-Type", "application/json")
	progressResponse := httptest.NewRecorder()
	server.ServeHTTP(progressResponse, progressRequest)
	if progressResponse.Code != http.StatusCreated {
		t.Fatalf("progress status = %d, body = %s", progressResponse.Code, progressResponse.Body.String())
	}
	var progress model.AgentPulseProgress
	if err := json.NewDecoder(progressResponse.Body).Decode(&progress); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	if progress.AgentID != "agent:focus" || progress.ActiveSprintID != "SP-10004" || progress.LeaseDurationSeconds != 1 || progress.LastProgressAt.IsZero() {
		t.Fatalf("progress = %#v", progress)
	}

	created, err := database.EvaluatePulseGuardianAt(context.Background(), progress.LastProgressAt.Add(2*time.Second))
	if err != nil || len(created) != 1 {
		t.Fatalf("create expired nudge = %#v, %v", created, err)
	}

	listResponse := httptest.NewRecorder()
	server.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent:focus/nudges", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list nudges status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var listed struct {
		Items []model.PulseNudge `json:"items"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatalf("decode pending nudges: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].NudgeID != created[0].NudgeID || listed.Items[0].Status != "Pending" {
		t.Fatalf("listed nudges = %#v", listed.Items)
	}

	ackRequest := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent:focus/nudges/"+strconv.FormatInt(created[0].NudgeID, 10)+"/ack", bytes.NewBufferString(`{}`))
	ackRequest.Header.Set("Content-Type", "application/json")
	ackResponse := httptest.NewRecorder()
	server.ServeHTTP(ackResponse, ackRequest)
	if ackResponse.Code != http.StatusOK {
		t.Fatalf("acknowledge status = %d, body = %s", ackResponse.Code, ackResponse.Body.String())
	}
	var acknowledged model.PulseNudge
	if err := json.NewDecoder(ackResponse.Body).Decode(&acknowledged); err != nil {
		t.Fatalf("decode acknowledged nudge: %v", err)
	}
	if acknowledged.Status != "Acknowledged" || acknowledged.AcknowledgedAt == nil || acknowledged.AcknowledgedBy != "agent:focus" {
		t.Fatalf("acknowledged = %#v", acknowledged)
	}

	listResponse = httptest.NewRecorder()
	server.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent:focus/nudges", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list acknowledged status = %d", listResponse.Code)
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatalf("decode empty pending nudges: %v", err)
	}
	if len(listed.Items) != 0 {
		t.Fatalf("pending nudges after acknowledgement = %#v", listed.Items)
	}
}
