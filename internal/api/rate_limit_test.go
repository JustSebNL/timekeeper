// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestAPIRateLimitMiddleware(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Rate Limit Project"})
	if err != nil {
		t.Fatal(err)
	}

	limiter := guardian.NewRateLimiter(2, 0.1) // 2 tokens capacity
	handler := api.NewWithRateLimiter(database, limiter)

	// 1. Send 2 requests with X-Agent-ID header -> allowed (status 200)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/tasks", nil)
		req.Header.Set("X-Agent-ID", "agent-loop-1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 on request %d, got %d", i+1, rec.Code)
		}
	}

	// 2. 3rd request -> rate limited (status 429)
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/tasks", nil)
	req3.Header.Set("X-Agent-ID", "agent-loop-1")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", rec3.Code)
	}
}
