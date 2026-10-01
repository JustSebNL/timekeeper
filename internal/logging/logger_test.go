// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLoggerExposesRequestIDAndStructuredErrorReason(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{AddSource: false}))
	handler := (&RequestLogger{Logger: logger}).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_project_message","message":"message body is required"}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/projects/P-1/messages", nil)
	req.Header.Set("X-TimeKeeper-Client", "0.0.1-alpha (abc12345)")
	req.Header.Set("X-TimeKeeper-Command", "msg add")
	req.Header.Set("X-Agent-ID", "agent-7")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	rid := response.Header().Get("X-Request-ID")
	if rid == "" {
		t.Fatal("expected response request ID")
	}
	if !strings.Contains(logs.String(), `"rid":"`+rid+`"`) {
		t.Fatalf("log did not include response request ID %q: %s", rid, logs.String())
	}
	for _, want := range []string{`"error_code":"invalid_project_message"`, `"error_message":"message body is required"`, `"client_version":"0.0.1-alpha (abc12345)"`, `"command":"msg add"`, `"agent_id":"agent-7"`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log did not include %s: %s", want, logs.String())
		}
	}

	var envelope map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
}
