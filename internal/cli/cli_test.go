// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
)

func TestRunExportPrintsPortableProjectSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects/P-10000/export" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"format":"timekeeper-project-export/v1","project_id":"P-10000","notes":[]}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"export", "P-10000"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "{\n  \"format\": \"timekeeper-project-export/v1\",\n  \"notes\": [],\n  \"project_id\": \"P-10000\"\n}\n" {
		t.Fatalf("export output = %q", got)
	}
}

func TestRunDoctorReportsHealthyConfiguredAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"doctor"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "[ok] API reachable: "+server.URL) || !strings.Contains(got, "[ok] health status: ok") || !strings.Contains(got, "Time Keeper is ready.") {
		t.Fatalf("doctor output = %q", got)
	}
}

func TestRunDoctorReportsRecoveryHintWhenAPIIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"doctor"}, &out, &errOut, url)
	if code != 1 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := errOut.String(); !strings.Contains(got, "[failed] API unreachable: "+url+"/health") || !strings.Contains(got, "Start Time Keeper, then run tk doctor again.") {
		t.Fatalf("doctor stderr = %q", got)
	}
}

func TestRunHelpListsImplementedExtensionAndPlanningCommands(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"help"}, &out, &errOut, "http://127.0.0.1:1618"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	for _, command := range []string{"Usage: tk [--url <api-base-url>] <command>", "sp extend <sprint-id> <duration> <reason>", "sp extensions <sprint-id>", "sp entries <sprint-id>", "p edit <project-id> <goal> <description>", "t edit <task-id> <goal> <description>", "p status <project-id>", "c status <category-id>", "t status <task-id>", "st status <subtask-id>", "llm new", "plan <generate|apply>"} {
		if !strings.Contains(out.String(), command) {
			t.Fatalf("help missing %q: %s", command, out.String())
		}
	}
}

func TestRunSummaryShowsProjectOperationalSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects/P-10000/operational-summary" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"project_id":"P-10000","total_sprints":3,"active_sprints":1,"held_sprints":1,"estimated_duration_seconds":3600,"buffer_duration_seconds":600,"extension_duration_seconds":300,"planned_duration_seconds":4500,"recorded_work_seconds":120}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"summary", "P-10000"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "P-10000	sprints=3	active=1	held=1	estimated=3600s	buffer=600s	extensions=300s	planned=4500s	recorded-work=120s\n" {
		t.Fatalf("summary output = %q", got)
	}
}

func TestRunEventsListsProjectExecutionHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects/P-10000/events" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"event_id":4,"event_type":"sprint_started","entity_id":"S-10003","message":"Sprint started."}]}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"events", "P-10000"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "4\tsprint_started\tS-10003\tSprint started.\n" {
		t.Fatalf("events output = %q", got)
	}
}

func TestRunNotesListsNewestProjectObservations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects/P-10000/notes" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"note_id":8,"project_id":"P-10000","content":"Newest","actor_id":"agent-1"},{"note_id":7,"project_id":"P-10000","content":"Earlier"}]}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"notes", "P-10000"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "8\tP-10000\tagent-1\tNewest\n7\tP-10000\t\tEarlier\n" {
		t.Fatalf("notes output = %q", got)
	}
}

func TestRunNoteCreatesProjectObservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/P-10000/notes" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content != "Validated the tree." {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"note_id":7,"project_id":"P-10000","content":"Validated the tree."}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"note", "P-10000", "Validated the tree."}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "7\tP-10000\tValidated the tree.\n" {
		t.Fatalf("note output = %q", got)
	}
}

func TestRunSprintStartCallsLifecycleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sprints/SP-10003/start" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sprint_id":"SP-10003","name":"Design","status":"Active","item_address":"10000.10001.10002.10003"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"sp", "start", "SP-10003"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "SP-10003\tActive\tDesign\t10000.10001.10002.10003\n" {
		t.Fatalf("sprint action output = %q", got)
	}
}

func TestRunSprintNewPassesOptionalBufferPercent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tasks/T-10002/sprints" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Name                     string `json:"name"`
			EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
			BufferPct                int64  `json:"buffer_pct"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "Design" || body.EstimatedDurationSeconds != 600 || body.BufferPct != 25 {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sprint_id":"SP-10003","name":"Design","item_address":"10000.10001.10002.10003","status":"Open"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"sp", "new", "task", "T-10002", "Design", "10m", "25"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "SP-10003\tOpen\tDesign\t10000.10001.10002.10003\n" {
		t.Fatalf("sprint output = %q", got)
	}
}

func TestRunPlanListPrintsReviewArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects/P-10000/planning-drafts" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"items":[{"draft_id":9,"status":"Review","summary":"Generated.","raw_json":"{\"version\":\"timekeeper-planning-draft/v1\"}"}]}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"plan", "list", "P-10000"}, &out, &errOut, server.URL); code != 0 || !strings.Contains(out.String(), "9\tReview\tGenerated.") || !strings.Contains(out.String(), "timekeeper-planning-draft/v1") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunPlanGenerateRequestsReviewDraft(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/P-10000/planning-drafts/generate" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"draft_id":9,"status":"Review","summary":"Generated."}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"plan", "generate", "P-10000", "7"}, &out, &errOut, server.URL); code != 0 || out.String() != "9\tReview\tGenerated.\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunLLMNewSupportsOptionalSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SystemPrompt string `json:"system_prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SystemPrompt != "Return strict JSON." {
			t.Fatalf("body=%#v err=%v", body, err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"pipeline_id":8,"name":"Planner","provider":"ollama","model":"qwen3:4b"}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"llm", "new", "Planner", "ollama", "http://127.0.0.1:11434", "qwen3:4b", "Return strict JSON."}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunLLMNewConfiguresLocalPipeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/llm-pipelines" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"pipeline_id":7,"name":"Planner","provider":"ollama","model":"qwen3:4b"}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"llm", "new", "Planner", "ollama", "http://127.0.0.1:11434", "qwen3:4b"}, &out, &errOut, server.URL); code != 0 || out.String() != "7\tPlanner\tollama\tqwen3:4b\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunTaskStatusUpdatesExplicitTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tasks/T-10002/status" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"T-10002","status":"Completed"}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"t", "status", "T-10002", "Completed"}, &out, &errOut, server.URL); code != 0 || out.String() != "T-10002\tCompleted\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunProjectEditUpdatesGoalAndDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/P-10000/metadata" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Goal        string `json:"goal"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Goal != "Ship" || body.Description != "Useful tracker" {
			t.Fatalf("body=%#v err=%v", body, err)
		}
		_, _ = w.Write([]byte(`{"project_id":"P-10000","project_goal":"Ship","project_description":"Useful tracker"}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"p", "edit", "P-10000", "Ship", "Useful tracker"}, &out, &errOut, server.URL); code != 0 || out.String() != "P-10000\tShip\tUseful tracker\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunProjectStatusUpdatesExplicitProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/P-10000/status" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request = %s %s content-type=%q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status != "On Hold" {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"project_id":"P-10000","status":"On Hold"}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"p", "status", "P-10000", "On Hold"}, &out, &errOut, server.URL)
	if code != 0 || out.String() != "P-10000\tOn Hold\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunTaskEditUpdatesMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tasks/T-10002/metadata" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["goal"] != "Ship safely" || payload["description"] != "Verify durable behavior." {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"T-10002","goal":"Ship safely","description":"Verify durable behavior."}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"t", "edit", "T-10002", "Ship safely", "Verify durable behavior."}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	if got := out.String(); got != "T-10002\tShip safely\tVerify durable behavior.\n" {
		t.Fatalf("output=%q", got)
	}
}

func TestRunCategoryNewSendsOptionalParent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/P-10000/categories" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		var input map[string]string
		_ = json.NewDecoder(r.Body).Decode(&input)
		if input["name"] != "Database" || input["parent_category_id"] != "C-10001" {
			t.Fatalf("input=%#v", input)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"category_id":"C-10002","name":"Database","item_address":"10000.10002"}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"c", "new", "P-10000", "Database", "C-10001"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	if out.String() != "C-10002\tDatabase\t10000.10002\n" {
		t.Fatalf("output=%q", out.String())
	}
}

func TestRunCategoryStatusUpdatesWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/categories/C-10001/status" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type=%q", got)
		}
		if body, _ := io.ReadAll(r.Body); string(body) != `{"status":"Completed"}` {
			t.Fatalf("body=%s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"category_id":"C-10001","status":"Completed"}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"c", "status", "C-10001", "Completed"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	if got := out.String(); got != "C-10001\tCompleted\n" {
		t.Fatalf("output=%q", got)
	}
}

func TestRunSubtaskStatusUpdatesWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/subtasks/ST-10004/status" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["status"] != "Completed" {
			t.Fatalf("body=%#v", body)
		}
		_, _ = w.Write([]byte(`{"subtask_id":"ST-10004","status":"Completed"}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"st", "status", "ST-10004", "Completed"}, &out, &errOut, server.URL); code != 0 || out.String() != "ST-10004\tCompleted\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunSprintEntriesListsRecordedIntervals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/sprints/SP-10004/time-entries" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"items":[{"time_entry_id":3,"sprint_id":"SP-10004","entry_type":"work","started_at":"2026-08-10T10:00:00Z","ended_at":"2026-08-10T10:10:00Z","duration_seconds":600,"reason":"Sprint placed on hold"}]}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"sp", "entries", "SP-10004"}, &out, &errOut, server.URL); code != 0 || out.String() != "3\tSP-10004\twork\t600\tSprint placed on hold\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunSprintExtensionsListsImmutableHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/sprints/SP-10004/extensions" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"items":[{"extension_id":2,"sprint_id":"SP-10004","duration_seconds":600,"reason":"Migration grew"}]}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"sp", "extensions", "SP-10004"}, &out, &errOut, server.URL); code != 0 || out.String() != "2\tSP-10004\t600\tMigration grew\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunSprintExtendRecordsJustifiedAdditionalTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sprints/SP-10004/extensions" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		var body struct {
			DurationSeconds int64  `json:"duration_seconds"`
			Reason          string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DurationSeconds != 600 || body.Reason != "Migration grew" {
			t.Fatalf("body=%#v err=%v", body, err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"extension_id":2,"sprint_id":"SP-10004","duration_seconds":600,"reason":"Migration grew"}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"sp", "extend", "SP-10004", "10m", "Migration grew"}, &out, &errOut, server.URL); code != 0 || out.String() != "2\tSP-10004\t600\tMigration grew\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunSprintNewCreatesBelowExplicitSubtask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/subtasks/ST-10003/sprints" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sprint_id":"SP-10004","name":"Implement","item_address":"10000.10001.10002.10003.10004","status":"Open","subtask_id":"ST-10003"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"sp", "new", "subtask", "ST-10003", "Implement", "10m"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "SP-10004\tOpen\tImplement\t10000.10001.10002.10003.10004\n" {
		t.Fatalf("sprint output = %q", got)
	}
}

func TestRunSprintNewCreatesBelowExplicitTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tasks/T-10002/sprints" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Name                     string `json:"name"`
			EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "Design" || body.EstimatedDurationSeconds != 600 {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sprint_id":"SP-10003","name":"Design","item_address":"10000.10001.10002.10003","status":"Open"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"sp", "new", "task", "T-10002", "Design", "10m"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "SP-10003\tOpen\tDesign\t10000.10001.10002.10003\n" {
		t.Fatalf("sprint output = %q", got)
	}
}

func TestRunSubtaskNewCreatesBelowExplicitTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tasks/T-10002/subtasks" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Name                     string `json:"name"`
			EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "Implement retrieval" || body.EstimatedDurationSeconds != 900 {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"subtask_id":"ST-10003","name":"Implement retrieval","item_address":"10000.10001.10002.10003"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"st", "new", "T-10002", "Implement retrieval", "15m"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "ST-10003\tImplement retrieval\t10000.10001.10002.10003\n" {
		t.Fatalf("subtask output = %q", got)
	}
}

func TestRunTaskNewCreatesBelowExplicitProjectAndCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/P-10000/tasks" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			CategoryID               string `json:"category_id"`
			Name                     string `json:"name"`
			EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CategoryID != "C-10001" || body.Name != "Build recall" || body.EstimatedDurationSeconds != 1800 {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task_id":"T-10002","name":"Build recall","item_address":"10000.10001.10002"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"t", "new", "P-10000", "C-10001", "Build recall", "30m"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "T-10002\tBuild recall\t10000.10001.10002\n" {
		t.Fatalf("task output = %q", got)
	}
}

func TestRunCategoryNewCreatesBelowExplicitProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/P-10000/categories" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "Memory" {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"category_id":"C-10001","name":"Memory","item_address":"10000.10001"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"c", "new", "P-10000", "Memory"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "C-10001\tMemory\t10000.10001\n" {
		t.Fatalf("category output = %q", got)
	}
}

func TestRunProjectNewCreatesThroughConfiguredAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "HSAM" {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"project_id":"P-10000","project_name":"HSAM","item_address":"10000"}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"p", "new", "HSAM"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "P-10000\tHSAM\t10000\n" {
		t.Fatalf("project output = %q", got)
	}
}

func TestRunTreeUsesConfiguredAPIAndShowsDistinctSprintOwnership(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects/P-10000/execution-tree" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"project":{"project_id":"P-10000","project_name":"HSAM","calculated_completion_pct":72.7},"categories":[{"category":{"category_id":"C-10001","name":"Memory"},"categories":[{"category":{"category_id":"C-10006","name":"Storage"},"tasks":[]}],"tasks":[{"task":{"task_id":"T-10002","name":"Recall","status":"On Hold"},"sprints":[{"sprint_id":"SP-10003","name":"Direct design","status":"Open"}],"subtasks":[{"subtask":{"subtask_id":"ST-10004","name":"Retrieval","status":"Completed"},"sprints":[{"sprint_id":"SP-10005","name":"Implement search","status":"Active"}]}]}]}]}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"tree", "P-10000"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	for _, line := range []string{
		"P-10000\t72.7%\tHSAM",
		"  C-10001	Memory",
		"    C-10006	Storage",
		"    T-10002	On Hold	Recall",
		"      SP-10003	Open	Direct design",
		"      ST-10004	Completed	Retrieval",
		"        SP-10005\tActive\tImplement search",
	} {
		if !strings.Contains(out.String(), line) {
			t.Fatalf("tree output missing %q:\n%s", line, out.String())
		}
	}
}

func TestRunListUsesConfiguredAPIAndPrintsProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"project_id":"P-10000","project_name":"HSAM","status":"Open","calculated_completion_pct":42.5,"item_address":"10000"}]}`))
	}))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"list"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "P-10000\tOpen\t42.5%\tHSAM\t10000") {
		t.Fatalf("list output = %q", got)
	}
}
