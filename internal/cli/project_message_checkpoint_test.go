// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
)

func TestMessageCheckpointCLIPostsGroupedEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/messages/checkpoint") {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Entries []map[string]any `json:"entries"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || len(payload.Entries) != 2 {
			t.Fatalf("checkpoint payload=%s err=%v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"root":{"message_id":20,"ref":"M-20"},"items":[{"message_id":20,"ref":"M-20"},{"message_id":21,"ref":"M-21","parent_message_id":20}]}`))
	}))
	defer server.Close()

	var out, errOut strings.Builder
	code := cli.Run([]string{"msg", "P-1", "checkpoint", "--author", "agent-a", "--entry", "decision|Save before dispatch.|queue", "--entry", "lesson|Keep only the M-ref in context.|handoff"}, &out, &errOut, server.URL)
	if code != 0 || !strings.Contains(out.String(), "M-20") || !strings.Contains(out.String(), "M-21") {
		t.Fatalf("checkpoint code=%d out=%q err=%s", code, out.String(), errOut.String())
	}
}
