package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
)

func TestSprintNewReportsStructuredAPIError(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tasks/T-10231/sprints" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("X-Request-ID", "req-10244")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"code":"sprint_limit_reached","message":"a task may have at most four direct sprints\nnext line"}}`)
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"sp", "new", "task", "T-10231", "fifth", "15m"}, &out, &errOut, server.URL)
	if code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	for _, want := range []string{"create sprint", "sprint_limit_reached", "at most four direct sprints", "next line", "request_id=req-10244"} {
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("stderr=%q missing %q", errOut.String(), want)
		}
	}
	if strings.Count(errOut.String(), "\n") != 1 {
		t.Fatalf("structured error injected extra lines: %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected stdout=%q", out.String())
	}
}

func TestSprintHoldReasonReportsStructuredAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sprints/SP-10428/hold-reason" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("X-Request-ID", "req-10428")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprint(w, `{"error":{"code":"invalid_hold_reason","message":"a hold reason may be updated only while a sprint is On Hold"}}`)
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"sp", "reason", "SP-10428", "new reason"}, &out, &errOut, server.URL)
	if code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	for _, want := range []string{"update sprint hold reason", "409 Conflict", "invalid_hold_reason", "only while a sprint is On Hold", "request_id=req-10428"} {
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("stderr=%q missing %q", errOut.String(), want)
		}
	}
	if strings.Count(errOut.String(), "\n") != 1 {
		t.Fatalf("structured error injected extra lines: %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected stdout=%q", out.String())
	}
}

// Every CLI command that talks to the API must surface the server's structured
// reason, not just the HTTP status line. These cover call sites that were still
// reducing a named constraint to "Time Keeper API returned 409 Conflict".
func TestCommandsConvertedFromBareStatusReportStructuredAPIError(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		method    string
		path      string
		status    int
		body      string
		operation string
	}{
		{
			name:      "task status",
			args:      []string{"t", "status", "T-10550", "Completed"},
			method:    http.MethodPost,
			path:      "/api/v1/tasks/T-10550/status",
			status:    http.StatusConflict,
			body:      `{"error":{"code":"invalid_transition","message":"subtask ST-10551 is not terminal"}}`,
			operation: "set task status",
		},
		{
			name:      "claim next sprint",
			args:      []string{"sp", "next", "P-10370"},
			method:    http.MethodPost,
			path:      "/api/v1/projects/P-10370/sprints/claim-next",
			status:    http.StatusServiceUnavailable,
			body:      `{"error":{"code":"database_busy","message":"The database is busy writing. Retry the request."}}`,
			operation: "claim next sprint",
		},
		{
			name:      "list projects",
			args:      []string{"list"},
			method:    http.MethodGet,
			path:      "/api/v1/projects",
			status:    http.StatusInternalServerError,
			body:      `{"error":{"code":"internal_error","message":"Internal server error."}}`,
			operation: "list projects",
		},
		{
			name:      "project events",
			args:      []string{"events", "P-10370"},
			method:    http.MethodGet,
			path:      "/api/v1/projects/P-10370/events",
			status:    http.StatusNotFound,
			body:      `{"error":{"code":"project_not_found","message":"Project was not found."}}`,
			operation: "list project events",
		},
		{
			name:      "record project note",
			args:      []string{"note", "P-10370", "hello"},
			method:    http.MethodPost,
			path:      "/api/v1/projects/P-10370/notes",
			status:    http.StatusBadRequest,
			body:      `{"error":{"code":"validation_error","message":"note content is required"}}`,
			operation: "record project note",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method || r.URL.Path != tc.path {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("X-Request-ID", "req-10642")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()

			var out, errOut bytes.Buffer
			code := cli.Run(tc.args, &out, &errOut, server.URL)
			if code != 1 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
			}
			var envelope struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(tc.body), &envelope); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			for _, want := range []string{tc.operation, envelope.Error.Code, envelope.Error.Message, "request_id=req-10642"} {
				if !strings.Contains(errOut.String(), want) {
					t.Fatalf("stderr=%q missing %q", errOut.String(), want)
				}
			}
			if strings.Count(errOut.String(), "\n") != 1 {
				t.Fatalf("structured error injected extra lines: %q", errOut.String())
			}
		})
	}
}

func TestSprintNextRetryAfterIsVisibleToCallers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"error":{"code":"database_busy","message":"The database is busy writing. Retry the request."}}`)
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"sp", "next", "P-10370"}, &out, &errOut, server.URL)
	if code != 1 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"claim next sprint", "503", "database_busy", "Retry the request"} {
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("stderr=%q missing %q", errOut.String(), want)
		}
	}
}

func TestSprintNewSanitizesAndBoundsPlainAPIError(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, "upstream failed\r\n\n"+strings.Repeat("x", 5000))
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"sp", "new", "task", "T-10231", "fifth", "15m"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("durable retry outbox should accept the failed delivery: code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "queued locally") || !strings.Contains(errOut.String(), "502 Bad Gateway") || !strings.Contains(errOut.String(), "upstream failed") || !strings.HasSuffix(errOut.String(), "\n") {
		t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
	}
	if strings.Count(errOut.String(), "\n") != 1 {
		t.Fatalf("error body injected extra lines: %q", errOut.String())
	}
	if len(errOut.String()) > 2200 {
		t.Fatalf("error body was not bounded: %d bytes", len(errOut.String()))
	}
	var queueOut, queueErr bytes.Buffer
	if code := cli.Run([]string{"queue", "list"}, &queueOut, &queueErr, server.URL); code != 0 || !strings.Contains(queueOut.String(), "Pending") {
		t.Fatalf("failed delivery was not durable in local queue: code=%d out=%q err=%q", code, queueOut.String(), queueErr.String())
	}
}
