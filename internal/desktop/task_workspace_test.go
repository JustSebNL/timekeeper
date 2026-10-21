package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoreClientTaskWorkspaceReadsAndWritesThroughCore(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/tasks/T-1":
			_, _ = io.WriteString(w, `{"task":{"task_id":"T-1","name":"Task","status":"Open"},"subtasks":[],"sprints":[{"sprint_id":"SP-2","name":"Sprint","status":"Open"}],"children":{"subtask_ids":[],"sprint_ids":["SP-2"],"open_subtasks":0,"open_sprints":1,"can_complete":false}}`)
		case "POST /api/v1/tasks/T-1/status":
			var input struct {
				Status string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Status != "On Hold" {
				t.Errorf("task status payload = %+v, err=%v", input, err)
			}
			_, _ = io.WriteString(w, `{"task_id":"T-1","name":"Task","status":"On Hold"}`)
		case "POST /api/v1/sprints/SP-2/hold":
			var input struct {
				Reason string `json:"reason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Reason != "waiting for review" {
				t.Errorf("Sprint transition payload = %+v, err=%v", input, err)
			}
			_, _ = io.WriteString(w, `{"sprint_id":"SP-2","name":"Sprint","status":"On Hold","hold_reason":"waiting for review"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := client.TaskDetail(context.Background(), "T-1")
	if err != nil {
		t.Fatalf("TaskDetail: %v", err)
	}
	if detail.Task.TaskID != "T-1" || len(detail.Sprints) != 1 || detail.Children.CanComplete {
		t.Fatalf("TaskDetail() = %+v", detail)
	}
	task, err := client.UpdateTaskStatus(context.Background(), "T-1", "On Hold")
	if err != nil || task.Status != "On Hold" {
		t.Fatalf("UpdateTaskStatus() = %+v, %v", task, err)
	}
	sprint, err := client.TransitionSprint(context.Background(), "SP-2", "hold", "waiting for review")
	if err != nil || sprint.Status != "On Hold" || sprint.HoldReason != "waiting for review" {
		t.Fatalf("TransitionSprint() = %+v, %v", sprint, err)
	}
	wantCalls := []string{
		"GET /api/v1/tasks/T-1",
		"POST /api/v1/tasks/T-1/status",
		"POST /api/v1/sprints/SP-2/hold",
	}
	if fmt.Sprint(calls) != fmt.Sprint(wantCalls) {
		t.Fatalf("Core calls = %v, want %v", calls, wantCalls)
	}
}

func TestCoreClientTaskWorkspaceRejectsInvalidInputsBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.TaskDetail(context.Background(), " "); err == nil {
		t.Fatal("empty Task ID must be rejected")
	}
	if _, err := client.UpdateTaskStatus(context.Background(), "T-1", "Failed"); err == nil {
		t.Fatal("unsupported Task status must be rejected")
	}
	if _, err := client.TransitionSprint(context.Background(), "SP-1", "fail", "reason"); err == nil {
		t.Fatal("unsupported Sprint action must be rejected")
	}
	if requests != 0 {
		t.Fatalf("invalid inputs made %d Core requests", requests)
	}
}

func TestCoreClientTaskWorkspaceErrorsDoNotExposeCoreResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `private diagnostic secret=do-not-forward`)
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.TaskDetail(context.Background(), "T-1")
	if err == nil || !strings.Contains(err.Error(), "HTTP 409") || strings.Contains(err.Error(), "do-not-forward") {
		t.Fatalf("TaskDetail error = %v, want status-only HTTP 409", err)
	}
}
