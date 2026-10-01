// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestUsageSnapshotAndSummaryRoutes(t *testing.T) {
	database, err := store.Open(t.TempDir() + "/usage.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "API usage project"})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.New(database)

	payload := []byte(`{"session_id":"session-1","agent_id":"codex","model":"gpt-5","turn_seq":1,"input_tokens":1200,"output_tokens":300,"messages":2}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/usage-sessions/session-1/snapshots", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("snapshot status = %d, body = %s", response.Code, response.Body.String())
	}
	var snapshot map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["duplicate"] != false {
		t.Fatalf("snapshot duplicate = %#v", snapshot["duplicate"])
	}

	secondPayload := []byte(`{"agent_id":"codex","model":"gpt-5","turn_seq":2,"input_tokens":2000,"output_tokens":500,"messages":4}`)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/usage-sessions/session-1/snapshots", bytes.NewReader(secondPayload))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !bytes.Contains(response.Body.Bytes(), []byte(`"input_tokens":800`)) {
		t.Fatalf("second snapshot status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/usage-sessions/session-1/snapshots", bytes.NewReader(secondPayload))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !bytes.Contains(response.Body.Bytes(), []byte(`"duplicate":true`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"input_tokens":0`)) {
		t.Fatalf("duplicate snapshot status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/usage-sessions/session-1/snapshots?limit=10", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"items"`)) || bytes.Count(response.Body.Bytes(), []byte(`"turn_seq"`)) != 2 {
		t.Fatalf("snapshot list status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/usage-summary", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", response.Code, response.Body.String())
	}
	var summary struct {
		Totals struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			Messages     int64 `json:"messages"`
			SessionCount int64 `json:"session_count"`
		} `json:"totals"`
		Sessions []struct {
			SessionID string `json:"session_id"`
			AgentID   string `json:"agent_id"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Totals.InputTokens != 2000 || summary.Totals.OutputTokens != 500 || summary.Totals.Messages != 4 || summary.Totals.SessionCount != 1 {
		t.Fatalf("summary totals = %#v", summary.Totals)
	}
	if len(summary.Sessions) != 1 || summary.Sessions[0].SessionID != "session-1" || summary.Sessions[0].AgentID != "codex" {
		t.Fatalf("summary sessions = %#v", summary.Sessions)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/export", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"usage_summary"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"session_count":1`)) {
		t.Fatalf("export status = %d, body = %s", response.Code, response.Body.String())
	}
}
