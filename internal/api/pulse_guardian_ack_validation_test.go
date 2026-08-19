// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
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

func TestPulseGuardianAcknowledgementRequiresAnEmptyJSONObject(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	server := api.New(database)

	progress, err := database.ReportAgentProgress(context.Background(), "agent:focus", model.AgentPulseProgressInput{LeaseDurationSeconds: 1})
	if err != nil {
		t.Fatalf("register progress: %v", err)
	}
	created, err := database.EvaluatePulseGuardianAt(context.Background(), progress.LastProgressAt.Add(2*time.Second))
	if err != nil || len(created) != 1 {
		t.Fatalf("create nudge = %#v, %v", created, err)
	}
	path := "/api/v1/agents/agent:focus/nudges/" + strconv.FormatInt(created[0].NudgeID, 10) + "/ack"
	for _, body := range []string{`null`, `[]`, `{"unexpected":true}`} {
		t.Run(body, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("body %s acknowledgement status = %d: %s", body, response.Code, response.Body.String())
			}
		})
	}
}
