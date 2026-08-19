// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestPulseIsAReadOnlyLocalAttentionEndpoint(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/pulse", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("pulse status = %d, body = %s", response.Code, response.Body.String())
	}
	var pulse struct {
		Format                      string     `json:"format"`
		GeneratedAt                 time.Time  `json:"generated_at"`
		RecommendedNextPulseSeconds int64      `json:"recommended_next_pulse_seconds"`
		Attention                   []struct{} `json:"attention"`
	}
	if err := json.NewDecoder(response.Body).Decode(&pulse); err != nil {
		t.Fatalf("decode pulse: %v", err)
	}
	if pulse.Format != "timekeeper-pulse/v1" || pulse.GeneratedAt.IsZero() || pulse.RecommendedNextPulseSeconds != 60 || pulse.Attention == nil || len(pulse.Attention) != 0 {
		t.Fatalf("pulse = %#v", pulse)
	}

	projects, err := database.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list projects after pulse: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("pulse must not create durable projects: %#v", projects)
	}
}
