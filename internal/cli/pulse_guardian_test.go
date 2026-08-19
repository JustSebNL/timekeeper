// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
)

func TestRunAgentProgressRegistersAnExplicitLocalGuardian(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/agents/agent:focus/progress" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			ActiveSprintID       string `json:"active_sprint_id"`
			LeaseDurationSeconds int64  `json:"lease_duration_seconds"`
			GuardianURL          string `json:"guardian_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.ActiveSprintID != "SP-10004" || body.LeaseDurationSeconds != 20 || body.GuardianURL != "http://127.0.0.1:17777/pulse" {
			t.Fatalf("progress body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"agent_id":"agent:focus","active_sprint_id":"SP-10004","lease_duration_seconds":20,"guardian_url":"http://127.0.0.1:17777/pulse","last_progress_at":"2026-08-12T10:00:00Z","updated_at":"2026-08-12T10:00:00Z"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"agent", "progress", "agent:focus", "20s", "SP-10004", "http://127.0.0.1:17777/pulse"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "agent:focus") || !strings.Contains(got, "lease=20s") || !strings.Contains(got, "guardian=http://127.0.0.1:17777/pulse") {
		t.Fatalf("progress output = %q", got)
	}
}
