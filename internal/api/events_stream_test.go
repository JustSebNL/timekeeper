// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestEventsStreamEndpoint(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "SSE Stream Test Project"})
	if err != nil {
		t.Fatal(err)
	}

	msg, err := database.CreateProjectMessage(ctx, project.ProjectID, model.CreateProjectMessageInput{
		Body: "Initial Event Message for SSE",
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	_ = msg

	handler := api.New(database)

	reqctx, cancel := context.WithTimeout(ctx, 350*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/events/stream", nil).WithContext(reqctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "event: project_event") {
		t.Errorf("expected body to contain event: project_event, got:\n%s", rec.Body.String())
	}
}
