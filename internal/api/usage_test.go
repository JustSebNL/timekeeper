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
	if summary.Totals.InputTokens != 1200 || summary.Totals.OutputTokens != 300 || summary.Totals.Messages != 2 || summary.Totals.SessionCount != 1 {
		t.Fatalf("summary totals = %#v", summary.Totals)
	}
	if len(summary.Sessions) != 1 || summary.Sessions[0].SessionID != "session-1" || summary.Sessions[0].AgentID != "codex" {
		t.Fatalf("summary sessions = %#v", summary.Sessions)
	}
}
