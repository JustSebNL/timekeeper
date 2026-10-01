// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectHandoffAPIRoute(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "API Handoff Project", Alias: "api-handoff"})
	if err != nil {
		t.Fatal(err)
	}

	handler := api.New(database)

	// Test 1: JSON output by default
	reqJSON := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/handoff", nil)
	recJSON := httptest.NewRecorder()
	handler.ServeHTTP(recJSON, reqJSON)

	if recJSON.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recJSON.Code)
	}

	var snapshot model.HandoffSnapshot
	if err := json.Unmarshal(recJSON.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if snapshot.Project.ProjectID != project.ProjectID {
		t.Errorf("expected project ID %s, got %s", project.ProjectID, snapshot.Project.ProjectID)
	}

	// Test 2: Markdown output via format=markdown query parameter
	reqMD := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/handoff?format=markdown", nil)
	recMD := httptest.NewRecorder()
	handler.ServeHTTP(recMD, reqMD)

	if recMD.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recMD.Code)
	}
	if !strings.Contains(recMD.Header().Get("Content-Type"), "text/markdown") {
		t.Errorf("expected Content-Type text/markdown, got %s", recMD.Header().Get("Content-Type"))
	}
	if !strings.Contains(recMD.Body.String(), "# Agent Handoff Snapshot:") {
		t.Errorf("expected Markdown header in body")
	}
}
