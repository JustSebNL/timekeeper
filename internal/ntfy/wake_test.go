package ntfy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWakeAckNudgeEndpointFormat(t *testing.T) {
	ackCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/test-agent/nudges":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"nudge_id": 42, "agent_id": "test-agent", "kind": "wake"},
				},
			})
		case "/api/v1/agents/test-agent/nudges/42/ack":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			ackCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	res, err := Wake(context.Background(), server.URL, "test-agent")
	if err != nil {
		t.Fatalf("unexpected wake error: %v", err)
	}
	if !ackCalled {
		t.Fatal("expected ack endpoint /api/v1/agents/test-agent/nudges/42/ack to be called")
	}
	if res.NudgeID != 42 {
		t.Fatalf("expected NudgeID 42, got %d", res.NudgeID)
	}
}
