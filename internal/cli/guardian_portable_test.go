package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"github.com/JustSebNL/timekeeper/internal/storeclient"
)

// The test executable also serves as an actual native runtime adapter. It
// receives the same argv/environment contract as a vendor's supervisor bridge.
func TestMain(m *testing.M) {
    if os.Getenv("TIMEKEEPER_TEST_FASE_ADAPTER")=="1"{
        var out,errOut strings.Builder
        code:=RunTracked([]string{"agent","identity"},&out,&errOut,os.Getenv("TK_BASE_URL"))
        if err:=os.WriteFile(os.Getenv("TK_TEST_WAKE_RECEIPT"),[]byte(out.String()),0600);err!=nil{os.Exit(2)}
        os.Exit(code)
    }
	if os.Getenv("TIMEKEEPER_TEST_ADAPTER") == "1" {
		data, _ := json.Marshal(map[string]string{"agent": os.Getenv("TIMEKEEPER_AGENT_ID"), "session": os.Getenv("TIMEKEEPER_SESSION_ID"), "runtime": os.Getenv("TIMEKEEPER_AGENT_RUNTIME"), "hermes_session": os.Getenv("HERMES_SESSION_ID"), "nudge": os.Getenv("AGENT_LAST_NUDGE_ID"), "sprint": os.Getenv("AGENT_LAST_SPRINT_ID"), "project": os.Getenv("PROJECT_ID"), "base": os.Getenv("TK_BASE_URL"), "reason": strings.Join(os.Args[1:], " ")})
		if err := os.WriteFile(os.Getenv("TK_TEST_WAKE_RECEIPT"), data, 0600); err != nil {
			os.Exit(2)
		}
		code, _ := strconv.Atoi(os.Getenv("TK_TEST_WAKE_EXIT"))
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestGuardianNativeExecutableAdapter(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	receipt := filepath.Join(t.TempDir(), "receipt.json")
	t.Setenv("TIMEKEEPER_TEST_ADAPTER", "1")
	t.Setenv("TK_TEST_WAKE_RECEIPT", receipt)
	t.Setenv("TK_TEST_WAKE_EXIT", "0")
	t.Setenv("TIMEKEEPER_SESSION_ID", "wrong-poller-session")
	t.Setenv("HERMES_SESSION_ID", "wrong-poller-hermes-session")
	ctx := &AgentContext{AgentID: "native-agent", SessionID: "watched-session", Runtime: "custom", LastNudgeID: 42, LastSprintID: "SP-2", LastProjectID: "P-1", BaseURL: "http://127.0.0.1:1618", Adapter: path}
	if err := runGuardianWorkCycle("native-agent", ctx, nil, false, 1, 1); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["nudge"] != "42" || got["project"] != "P-1" || got["reason"] != "Continue the already-authorized TimeKeeper work." {
		t.Fatalf("native adapter contract: %s", data)
	}
	if got["agent"] != "native-agent" || got["session"] != "watched-session" || got["runtime"] != "custom" || got["hermes_session"] != "" {
		t.Fatalf("lost watched session or leaked poller runtime session: %s", data)
	}
}

func TestGuardianWindowsWSLContext(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows to WSL boundary")
	}
	if err := exec.Command("wsl.exe", "--", "true").Run(); err != nil {
		t.Skip("WSL unavailable")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "adapter.sh")
	body := "#!/bin/sh\nprintf '%s|%s|%s|%s' \"$AGENT_ID\" \"$AGENT_LAST_NUDGE_ID\" \"$PROJECT_ID\" \"$TK_BASE_URL\" > \"$TIMEKEEPER_STATE_DIR/wsl-receipt\"\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WSLENV", "")
	t.Setenv("TIMEKEEPER_STATE_DIR", dir)
	ctx := &AgentContext{AgentID: "wsl-agent", LastNudgeID: 42, LastProjectID: "P-1", BaseURL: "http://127.0.0.1:1618", Adapter: script}
	if err := runGuardianWorkCycle("wsl-agent", ctx, nil, false, 1, 1); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "wsl-receipt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "wsl-agent|42|P-1|http://127.0.0.1:1618" {
		t.Fatalf("lost WSL context: %q", data)
	}
}

func TestGuardianPowerShellReferenceReleasesLock(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows reference adapter")
	}
	supervisor, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := filepath.Abs("../../scripts/guardian/agent-wake-adapter.example.ps1")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("TIMEKEEPER_TEST_ADAPTER", "1")
	t.Setenv("AGENT_RUNTIME_SUBMIT", supervisor)
	t.Setenv("TIMEKEEPER_STATE_DIR", dir)
	t.Setenv("TK_TEST_WAKE_RECEIPT", filepath.Join(dir, "receipt.json"))
	t.Setenv("TK_TEST_WAKE_EXIT", "0")
	ctx := &AgentContext{AgentID: "reference-agent", LastNudgeID: 42, LastProjectID: "P-1", Adapter: adapter}
	for cycle := 1; cycle <= 2; cycle++ {
		if err := runGuardianWorkCycle("reference-agent", ctx, nil, true, cycle, 2); err != nil {
			t.Fatalf("cycle %d: %v", cycle, err)
		}
	}
}

func TestGuardianPortableRealAPILifecycle(t *testing.T) {
	receipt := portableTestAdapter(t)
	database, err := store.Open(filepath.Join(t.TempDir(), "guardian.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := t.Context()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Portable wake test"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Test work", EstimatedDurationSeconds: 300})
	if err != nil {
		t.Fatal(err)
	}
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Test boundary", EstimatedDurationSeconds: 300})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.TransitionSprint(ctx, sprint.SprintID, "start", ""); err != nil {
		t.Fatal(err)
	}
	progress, err := database.ReportAgentProgress(ctx, "test-agent", model.AgentPulseProgressInput{ActiveSprintID: &sprint.SprintID, LeaseDurationSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	nudges, err := database.EvaluatePulseGuardianAt(ctx, progress.LastProgressAt.Add(2*time.Second))
	if err != nil || len(nudges) != 1 {
		t.Fatalf("nudges=%+v err=%v", nudges, err)
	}
	server := httptest.NewServer(api.New(database))
	defer server.Close()
	tk := &storeclient.Client{BaseURL: server.URL, HTTPClient: server.Client()}
	state := &AgentContext{AgentID: "test-agent", LastProjectID: project.ProjectID}
	var out, errOut strings.Builder
	if code := runWakeOnce("test-agent", tk, state, false, 1, &out, &errOut); code != 0 {
		t.Fatalf("wake=%d: %s", code, errOut.String())
	}
	pending, err := database.ListPendingPulseNudges(ctx, "test-agent")
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	history, err := database.ListPulseNudgeHistory(ctx, "test-agent")
	if err != nil || len(history) != 1 || history[0].Status != "Acknowledged" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	renewed, err := database.GetAgentProgress(ctx, "test-agent")
	if err != nil || !renewed.LastProgressAt.After(progress.LastProgressAt) {
		t.Fatalf("lease was not renewed: %+v %v", renewed, err)
	}
	data, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["nudge"] != strconv.FormatInt(nudges[0].NudgeID, 10) || got["sprint"] != sprint.SprintID || state.WorkCycles != 1 {
		t.Fatalf("receipt=%s state=%+v", data, state)
	}
}

func TestGuardianPortableCommands(t *testing.T) {
	for _, tc := range []struct {
		platform, path, command string
		args                    []string
	}{
		{"windows", `C:\Agent Tools\wake.exe`, `C:\Agent Tools\wake.exe`, nil},
		{"linux", "/opt/agent/wake", "/opt/agent/wake", nil},
		{"windows", `C:\Agent Tools\wake.ps1`, "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-File", `C:\Agent Tools\wake.ps1`}},
		{"linux", "/opt/agent/wake.ps1", "pwsh", []string{"-NoProfile", "-NonInteractive", "-File", "/opt/agent/wake.ps1"}},
	} {
		t.Run(tc.platform+tc.path, func(t *testing.T) {
			command, args := guardianWorkCommand(tc.platform, tc.path)
			if command != tc.command || !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("got %s %q, want %s %q", command, args, tc.command, tc.args)
			}
		})
	}
}

// This real child adapter captures what a runtime receives, without invoking a
// model or touching the user's service. A failed dispatch cannot create receipt.
func portableTestAdapter(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	receipt := filepath.Join(dir, "receipt.json")
	t.Setenv("TK_TEST_WAKE_RECEIPT", receipt)
	script := filepath.Join(dir, "worker.sh")
	body := "#!/bin/sh\nprintf '{\"nudge\":\"%s\",\"sprint\":\"%s\",\"project\":\"%s\",\"base\":\"%s\",\"objective\":\"%s\"}' \"$AGENT_LAST_NUDGE_ID\" \"$AGENT_LAST_SPRINT_ID\" \"$PROJECT_ID\" \"$TK_BASE_URL\" \"$GUARDIAN_OBJECTIVE\" > \"$TK_TEST_WAKE_RECEIPT\"\nexit \"${TK_TEST_WAKE_EXIT:-0}\"\n"
	if runtime.GOOS == "windows" {
		script = filepath.Join(dir, "worker.ps1")
		body = "@{nudge=$env:AGENT_LAST_NUDGE_ID;sprint=$env:AGENT_LAST_SPRINT_ID;project=$env:PROJECT_ID;base=$env:TK_BASE_URL;objective=$env:GUARDIAN_OBJECTIVE} | ConvertTo-Json -Compress | Set-Content -LiteralPath $env:TK_TEST_WAKE_RECEIPT\nif ($env:TK_TEST_WAKE_EXIT) { exit [int]$env:TK_TEST_WAKE_EXIT }\n"
	}
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_WAKE_COMMAND", script)
	t.Setenv("TK_GUARDIAN_SELF_WORK", "")
	t.Setenv("TIMEKEEPER_STATE_DIR", dir)
	return receipt
}

func TestGuardianPortableWakeReceiptAndAck(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		ackStatus                      int
		adapterExit                    string
		wantCode, wantCycles, wantAcks int
	}{
		{"verified", 200, "0", 0, 1, 1},
		{"ack refused", 503, "0", 1, 0, 1},
		{"worker failed", 200, "1", 1, 0, 0},
		{"coalesced", 200, "75", 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			receipt := portableTestAdapter(t)
			t.Setenv("TK_TEST_WAKE_EXIT", tc.adapterExit)
			acks, acknowledgedEvents := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/agents/test-agent/nudges":
					_, _ = w.Write([]byte(`{"items":[{"nudge_id":42,"agent_id":"test-agent","active_sprint_id":"SP-2","status":"Pending"}]}`))
				case "/api/v1/projects/P-1/execution-tree":
					_, _ = w.Write([]byte(`{"project":{"project_id":"P-1"},"categories":[{"tasks":[{"sprints":[{"sprint_id":"SP-2","project_id":"P-1","status":"Active"}]}]}]}`))
				case "/api/v1/agents/test-agent/nudges/42/ack":
					acks++
					if _, err := os.Stat(receipt); err != nil {
						t.Error("acknowledgement preceded worker receipt")
					}
					w.WriteHeader(tc.ackStatus)
					_, _ = w.Write([]byte(`{"nudge_id":42,"agent_id":"test-agent","status":"Acknowledged"}`))
				case "/api/v1/agents/test-agent/nudges/42/events":
					var event map[string]string
					_ = json.NewDecoder(r.Body).Decode(&event)
					if event["event_type"] == "acknowledged" {
						acknowledgedEvents++
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			tk := &storeclient.Client{BaseURL: server.URL, HTTPClient: server.Client()}
			ctx := &AgentContext{AgentID: "test-agent", LastNudgeID: 7, LastSprintID: "SP-old", LastProjectID: "P-1"}
			var out, errOut strings.Builder
			code := runWakeOnce("test-agent", tk, ctx, false, 1, &out, &errOut)
			if code != tc.wantCode || ctx.WorkCycles != tc.wantCycles || acks != tc.wantAcks {
				t.Fatalf("code=%d cycles=%d acks=%d; stderr=%s", code, ctx.WorkCycles, acks, errOut.String())
			}
			if acknowledgedEvents != tc.wantCycles {
				t.Fatalf("false acknowledgement events: %d", acknowledgedEvents)
			}
			data, err := os.ReadFile(receipt)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]string
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got["nudge"] != "42" || got["sprint"] != "SP-2" || got["project"] != "P-1" || got["base"] != server.URL || got["objective"] == "" {
				t.Fatalf("incorrect runtime context: %s", data)
			}
		})
	}
}

func TestGuardianUnavailableAPIIsNotIdle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer server.Close()
	tk := &storeclient.Client{BaseURL: server.URL, HTTPClient: server.Client()}
	var out, errOut strings.Builder
	if code := runWakeOnce("agent", tk, &AgentContext{AgentID: "agent"}, false, 1, &out, &errOut); code == 0 {
		t.Fatalf("API failure reported as success: %s", out.String())
	}
}

func TestGuardianScopeMismatchNeverLaunches(t *testing.T) {
	receipt := portableTestAdapter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/work-state") {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/nudges") {
			_, _ = w.Write([]byte(`{"items":[{"nudge_id":42,"agent_id":"test-agent","active_sprint_id":"SP-other","status":"Pending"}]}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/execution-tree") {
			_, _ = w.Write([]byte(`{"project":{"project_id":"P-1"},"categories":[]}`))
			return
		}
		t.Error("unexpected mutation: " + r.URL.Path)
	}))
	defer server.Close()
	tk := &storeclient.Client{BaseURL: server.URL, HTTPClient: server.Client()}
	code := runWakeOnce("test-agent", tk, &AgentContext{AgentID: "test-agent", LastProjectID: "P-1"}, false, 1, &strings.Builder{}, &strings.Builder{})
	if code != 1 {
		t.Fatalf("scope mismatch accepted: %d", code)
	}
	if _, err := os.Stat(receipt); !os.IsNotExist(err) {
		t.Fatal("worker launched for another project's Sprint")
	}
}

func TestGuardianWakeOptionsPreserveExplicitBudget(t *testing.T) {
	opts, err := parseGuardianWakeOptions([]string{"--agent", "test-agent", "--project", "P-1", "--adapter", "C:/Agent Tools/wake.exe", "--max-cycles", "1", "--continuous", "--work-timeout", "2m"})
	if err != nil || opts.maxCycles != 1 || opts.projectID != "P-1" || opts.workTimeout != 2*time.Minute {
		t.Fatalf("options=%+v err=%v", opts, err)
	}
	for _, args := range [][]string{{"--agent", "../escape"}, {"--agent", "a", "--unknown"}, {"--agent", "a", "--work-timeout", "0s"}} {
		if _, err := parseGuardianWakeOptions(args); err == nil {
			t.Fatalf("accepted invalid options %q", args)
		}
	}
}

func TestGuardianAdapterTimeout(t *testing.T) {
	dir := t.TempDir()
	path, body := filepath.Join(dir, "sleep.sh"), "#!/bin/sh\nexec sleep 10\n"
	if runtime.GOOS == "windows" {
		path, body = filepath.Join(dir, "sleep.ps1"), "Start-Sleep -Seconds 10\n"
	}
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err := runGuardianWorkCycle("test-agent", &AgentContext{Adapter: path, WorkTimeout: 100 * time.Millisecond}, nil, false, 1, 1)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("adapter wait not bounded: %v", err)
	}
}

func TestGuardianUntilIdleChecksBeforeEveryCycle(t *testing.T) {
	portableTestAdapter(t)
	checks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks++
		if checks == 1 {
			_, _ = w.Write([]byte(`{"open_sprints":0,"active_sprints":1}`))
		} else {
			_, _ = w.Write([]byte(`{"open_sprints":0,"active_sprints":0}`))
		}
	}))
	defer server.Close()
	tk := &storeclient.Client{BaseURL: server.URL, HTTPClient: server.Client()}
	ctx := &AgentContext{AgentID: "test-agent", LastProjectID: "P-1"}
	completed := runWorkUntilIdle("test-agent", tk, ctx, nil, 4, &strings.Builder{}, &strings.Builder{})
	if completed != 1 {
		t.Fatalf("continued %d times after queue became idle; want 1", completed)
	}
}
