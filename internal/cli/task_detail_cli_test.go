package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubtaskShowReadsFocusedSubtaskDetail(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/subtasks/ST-10002" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"subtask":{"subtask_id":"ST-10002","project_id":"P-10000","category_id":"C-10001","task_id":"T-10001","item_address":"10000.10001.10001.10002","name":"Retry delivery","goal":"Make retries safe","description":"Reuse the persisted key","status":"Open","priority":"Normal","estimated_duration_seconds":900},"sprints":[{"sprint_id":"SP-10003","name":"Add retry","status":"Open","estimated_duration_seconds":300}]}`))
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	if code := Run([]string{"st", "show", "ST-10002"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("Subtask show code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	for _, want := range []string{"ST-10002", "T-10001", "Make retries safe", "Reuse the persisted key", "SP-10003", "Add retry"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("Subtask detail missing %q: %s", want, out.String())
		}
	}
	if requests != 1 {
		t.Fatalf("Subtask show made %d requests, want one focused detail read", requests)
	}
}

func TestTaskAndSubtaskShowSupportJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/tasks/T-10001":
			_, _ = writer.Write([]byte(`{"task":{"task_id":"T-10001","name":"Build queue","status":"Open"},"subtasks":[],"sprints":[],"children":{"can_complete":true}}`))
		case "/api/v1/subtasks/ST-10002":
			_, _ = writer.Write([]byte(`{"subtask":{"subtask_id":"ST-10002","task_id":"T-10001","name":"Retry delivery","status":"Open"},"sprints":[]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	for _, test := range []struct {
		args      []string
		recordKey string
		idKey     string
		idValue   string
	}{
		{args: []string{"--json", "t", "show", "T-10001"}, recordKey: "task", idKey: "task_id", idValue: "T-10001"},
		{args: []string{"--json", "st", "show", "ST-10002"}, recordKey: "subtask", idKey: "subtask_id", idValue: "ST-10002"},
	} {
		var out, errOut bytes.Buffer
		if code := Run(test.args, &out, &errOut, server.URL); code != 0 {
			t.Fatalf("JSON detail %v code=%d stdout=%q stderr=%q", test.args, code, out.String(), errOut.String())
		}
		var envelope map[string]json.RawMessage
		var record map[string]any
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Errorf("JSON detail %v is invalid JSON: %q: %v", test.args, out.String(), err)
			continue
		}
		if err := json.Unmarshal(envelope[test.recordKey], &record); err != nil {
			t.Errorf("JSON detail %v is missing %q record: %q: %v", test.args, test.recordKey, out.String(), err)
			continue
		}
		if record[test.idKey] != test.idValue {
			t.Errorf("JSON detail %v %s=%v, want %s", test.args, test.idKey, record[test.idKey], test.idValue)
		}
	}
}

func TestTreeAndSummaryHelpAreLocalAndSideEffectFree(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		http.NotFound(writer, request)
	}))
	defer server.Close()
	for _, command := range [][]string{{"tree", "--help"}, {"summary", "--help"}} {
		var out, errOut bytes.Buffer
		if code := Run(command, &out, &errOut, server.URL); code != 0 {
			t.Errorf("%v help code=%d stdout=%q stderr=%q", command, code, out.String(), errOut.String())
		}
		if !strings.Contains(out.String(), "usage: tk "+command[0]) {
			t.Errorf("%v help output=%q", command, out.String())
		}
	}
	if requests != 0 {
		t.Fatalf("local help made %d API requests", requests)
	}
}

func TestTaskShowReadsFocusedTaskDetailWithoutProjectExport(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/tasks/T-10001" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"task":{"task_id":"T-10001","project_id":"P-10000","category_id":"C-10001","item_address":"10000.10001.10001","name":"Build queue","goal":"Keep every offer durable","description":"Persist before delivery","status":"Open","priority":"High","estimated_duration_seconds":3600,"reported_completion_pct":25,"calculated_completion_pct":50,"auto_close":true},"subtasks":[{"subtask_id":"ST-10002","name":"Inspect failures","status":"Open"}],"sprints":[{"sprint_id":"SP-10003","name":"Direct work","status":"Active"}],"children":{"subtask_ids":["ST-10002"],"sprint_ids":["SP-10003"],"open_subtasks":1,"open_sprints":1,"can_complete":false}}`))
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	if code := Run([]string{"t", "show", "T-10001"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("Task show code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	for _, want := range []string{"T-10001", "Build queue", "Keep every offer durable", "Persist before delivery", "ST-10002", "SP-10003", "cannot complete"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("Task detail missing %q: %s", want, out.String())
		}
	}
	if requests != 1 {
		t.Fatalf("Task show made %d requests, want one focused detail read", requests)
	}
}
