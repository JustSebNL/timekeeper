package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/desktop"
	"github.com/JustSebNL/timekeeper/internal/version"
)

func TestDesktopProjectExecutionTreeUsesReadOnlyCoreBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/version":
			_, _ = fmt.Fprintf(w, `{"service":"TimeKeeper","version":%q}`, version.Get().Version)
		case "/api/v1/projects/P-8/execution-tree":
			if r.Method != http.MethodGet {
				t.Fatalf("method=%s", r.Method)
			}
			_, _ = w.Write([]byte(`{"project":{"project_id":"P-8","project_name":"Workspace","status":"Open"},"categories":[{"category":{"category_id":"C-9","name":"Delivery","status":"Open"},"categories":[],"tasks":[]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())
	tree, err := app.ProjectExecutionTree("P-8")
	if err != nil {
		t.Fatal(err)
	}
	if tree.Project.ProjectID != "P-8" || len(tree.Categories) != 1 || tree.Categories[0].Category.Name != "Delivery" {
		t.Fatalf("ProjectExecutionTree() = %+v", tree)
	}
}

func TestDesktopAgentWorkStatesBindingDelegatesToCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/agents/work-states" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"items":[{"agent_id":"agent-1","session_id":"session-1","state":"working","reported_state":"working"}]}`)
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())

	states, err := app.AgentWorkStates()
	if err != nil || len(states) != 1 || states[0].AgentID != "agent-1" || states[0].State != "working" {
		t.Fatalf("AgentWorkStates() = %+v, %v", states, err)
	}
}

func TestDesktopProjectHandoffBindingDelegatesToCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects/P-7/handoff" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"project":{"project_id":"P-7","project_name":"Example","status":"Open"},"recent_events":[{"event_id":3,"entity_type":"task","entity_id":"T-3","event_type":"task_started","message":"Task started","created_at":"2026-10-18T10:00:00Z"}],"recent_notes":[{"note_id":7,"project_id":"P-7","content":"Capture the decision","created_at":"2026-10-18T09:00:00Z"}]}`)
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())

	handoff, err := app.ProjectHandoff("P-7")
	if err != nil || handoff.Project.ProjectID != "P-7" || len(handoff.RecentEvents) != 1 || handoff.RecentEvents[0].EntityID != "T-3" || len(handoff.RecentNotes) != 1 {
		t.Fatalf("ProjectHandoff() = %+v, %v", handoff, err)
	}
}

func TestDesktopGlobalSearchBindingDelegatesToCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/search" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("q") != "queue" || query.Get("limit") != "2" || strings.Join(query["scope"], ",") != "projects,tasks" {
			t.Fatalf("search query = %v", query)
		}
		_, _ = io.WriteString(w, `{"query":"queue","scopes":["projects","tasks"],"items":[],"total":0,"limit":2,"has_more":false}`)
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())

	response, err := app.GlobalSearch(" queue ", 2, []string{"projects", "tasks"})
	if err != nil || response.Query != "queue" || response.Total != 0 || response.Limit != 2 {
		t.Fatalf("GlobalSearch() = %+v, %v", response, err)
	}
}

func TestDesktopFileHistoryBindingsDelegateToCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/projects/P-7/file-root":
			_, _ = io.WriteString(w, `{"project_id":"P-7","root_path":"C:\\\\workspace"}`)
		case "GET /api/v1/projects/P-7/files":
			_, _ = io.WriteString(w, `{"items":[{"file_id":"F-1","project_id":"P-7","current_path":"src/main.go","file_type":"go","current_version_id":"FV-1","tracking_policy":"project_scan"}]}`)
		case "GET /api/v1/files/F-1/versions":
			_, _ = io.WriteString(w, `{"items":[{"version_id":"FV-1","file_id":"F-1","version_number":1,"change_kind":"file_added","snapshot_type":"full","blob_ref":"sha256:a","size_bytes":5,"status":"current"}]}`)
		case "GET /api/v1/files/F-1/versions/FV-1":
			_, _ = io.WriteString(w, `{"version":{"version_id":"FV-1","file_id":"F-1","version_number":1,"change_kind":"file_added"},"content":"hello"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())

	root, err := app.ProjectFileRoot("P-7")
	if err != nil || root.RootPath == "" {
		t.Fatalf("ProjectFileRoot() = %+v, %v", root, err)
	}
	files, err := app.ProjectFiles("P-7")
	if err != nil || len(files) != 1 || files[0].FileID != "F-1" {
		t.Fatalf("ProjectFiles() = %+v, %v", files, err)
	}
	versions, err := app.FileVersions("F-1")
	if err != nil || len(versions) != 1 || versions[0].VersionID != "FV-1" {
		t.Fatalf("FileVersions() = %+v, %v", versions, err)
	}
	content, err := app.ReadFileVersion("F-1", "FV-1")
	if err != nil || content.Content != "hello" || content.Version.VersionID != "FV-1" {
		t.Fatalf("ReadFileVersion() = %+v, %v", content, err)
	}
}

func TestDesktopFileHistoryMutationBindingsDelegateToCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "PUT /api/v1/projects/P-7/file-root":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["root_path"] != `D:\\workspace` {
				t.Errorf("root payload=%+v err=%v", body, err)
			}
			_, _ = io.WriteString(w, `{"project_id":"P-7","root_path":"D:\\\\workspace"}`)
		case "POST /api/v1/projects/P-7/files/scan":
			_, _ = io.WriteString(w, `{"project_id":"P-7","root_path":"D:\\\\workspace","files_discovered":1,"files_added":0,"versions_added":0,"files_skipped":0}`)
		case "POST /api/v1/files/F-1/versions":
			var body desktop.FileHistorySaveInput
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ExpectedCurrentVersionID != "FV-1" || body.ChangeKind != "bugfix" || body.Content != "package updated\\n" {
				t.Errorf("save payload=%+v err=%v", body, err)
			}
			_, _ = io.WriteString(w, `{"file":{"file_id":"F-1","current_version_id":"FV-2"},"version":{"version_id":"FV-2","file_id":"F-1","version_number":2,"parent_version_id":"FV-1","change_kind":"bugfix"}}`)
		case "POST /api/v1/files/F-1/restore":
			var body desktop.FileHistoryRestoreInput
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.VersionID != "FV-1" || body.ExpectedCurrentVersionID != "FV-2" {
				t.Errorf("restore payload=%+v err=%v", body, err)
			}
			_, _ = io.WriteString(w, `{"file":{"file_id":"F-1","current_version_id":"FV-3"},"version":{"version_id":"FV-3","file_id":"F-1","version_number":3,"parent_version_id":"FV-2","restores_version_id":"FV-1","change_kind":"reversion"}}`)
		case "GET /api/v1/files/F-1/compare":
			if r.URL.Query().Get("from") != "FV-1" || r.URL.Query().Get("to") != "FV-2" {
				t.Errorf("compare query=%s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"from":{"version_id":"FV-1","file_id":"F-1"},"to":{"version_id":"FV-2","file_id":"F-1"},"from_content":"old","to_content":"new","hunks":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())

	root, err := app.SetProjectFileRoot("P-7", `D:\\workspace`)
	if err != nil || root.RootPath == "" {
		t.Fatalf("SetProjectFileRoot() = %+v, %v", root, err)
	}
	if result, err := app.ScanProjectFiles("P-7"); err != nil || result.FilesDiscovered != 1 {
		t.Fatalf("ScanProjectFiles() = %+v, %v", result, err)
	}
	saved, err := app.SaveFileVersion("F-1", desktop.FileHistorySaveInput{Content: "package updated\\n", ExpectedCurrentVersionID: "FV-1", ChangeKind: "bugfix"})
	if err != nil || saved.Version.VersionID != "FV-2" {
		t.Fatalf("SaveFileVersion() = %+v, %v", saved, err)
	}
	compared, err := app.CompareFileVersions("F-1", "FV-1", "FV-2")
	if err != nil || compared.From.VersionID != "FV-1" || compared.To.VersionID != "FV-2" || compared.FromContent != "old" || compared.ToContent != "new" {
		t.Fatalf("CompareFileVersions() = %+v, %v", compared, err)
	}
	restored, err := app.RestoreFileVersion("F-1", desktop.FileHistoryRestoreInput{VersionID: "FV-1", ExpectedCurrentVersionID: "FV-2"})
	if err != nil || restored.Version.VersionID != "FV-3" || restored.Version.RestoresVersionID != "FV-1" {
		t.Fatalf("RestoreFileVersion() = %+v, %v", restored, err)
	}
}

func TestDesktopPluginCatalogBindingDelegatesToCore(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/plugins" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"schema_version":"timekeeper-plugin-index/v1","generated_at":"2026-10-18T20:00:00Z","plugins":[{"id":"extension.example","version":"1.2.0","name":"Example extension","runtime":"wasi","entrypoint":"plugin.wasm","protocol":"timekeeper-plugin/v1","capabilities":["knowledge.search"],"requires":["files.read"]}]}`)
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())

	catalog, err := app.PluginCatalog()
	if err != nil || catalog.SchemaVersion != "timekeeper-plugin-index/v1" || len(catalog.Plugins) != 1 || catalog.Plugins[0].ID != "extension.example" {
		t.Fatalf("PluginCatalog() = %+v, %v", catalog, err)
	}
	if fmt.Sprint(calls) != "[GET /api/v1/plugins]" {
		t.Fatalf("Core calls = %v", calls)
	}
}

func TestDesktopMemoryGalaxyBindingsDelegateToCore(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/knowledge/status":
			_, _ = io.WriteString(w, `{"available":true,"provider":"obsidian","protocol_version":"timekeeper-knowledge/v1","capabilities":["knowledge.search"],"last_checked_at":"2026-10-18T19:00:00Z"}`)
		case "POST /api/v1/knowledge/search":
			var input struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Query != "durable context" || input.Limit != 8 {
				t.Errorf("knowledge query = %+v, err=%v", input, err)
			}
			_, _ = io.WriteString(w, `{"results":[{"id":"MG-1","title":"Durable context","type":"pattern","review_status":"verified","visibility":"private"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())

	status, err := app.KnowledgeStatus()
	if err != nil || !status.Available || status.Provider != "obsidian" {
		t.Fatalf("KnowledgeStatus() = %+v, %v", status, err)
	}
	results, err := app.SearchKnowledge("durable context", 8)
	if err != nil || len(results) != 1 || results[0].ID != "MG-1" {
		t.Fatalf("SearchKnowledge() = %+v, %v", results, err)
	}
	want := []string{"GET /api/v1/knowledge/status", "POST /api/v1/knowledge/search"}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("Core calls = %v, want %v", calls, want)
	}
}

func TestDesktopMetricsWorkspaceBindingsDelegateToCore(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/projects/P-9/usage-summary":
			_, _ = io.WriteString(w, `{"project_id":"P-9","totals":{"input_tokens":10,"output_tokens":4,"messages":2,"session_count":1,"cost":{"status":"unpriced","reason":"not configured"}},"sessions":[],"days":[]}`)
		case "GET /api/v1/projects/P-9/operational-summary":
			_, _ = io.WriteString(w, `{"project_id":"P-9","total_sprints":1,"active_sprints":1,"planned_duration_seconds":3600,"recorded_work_seconds":90}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())

	usage, err := app.ProjectUsageSummary("P-9")
	if err != nil || usage.ProjectID != "P-9" || usage.Totals.InputTokens != 10 || usage.Totals.Cost.Status != "unpriced" {
		t.Fatalf("ProjectUsageSummary() = %+v, %v", usage, err)
	}
	operational, err := app.ProjectOperationalSummary("P-9")
	if err != nil || operational.ProjectID != "P-9" || operational.ActiveSprints != 1 || operational.RecordedWorkSeconds != 90 {
		t.Fatalf("ProjectOperationalSummary() = %+v, %v", operational, err)
	}
	want := []string{"GET /api/v1/projects/P-9/usage-summary", "GET /api/v1/projects/P-9/operational-summary"}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("Core calls = %v, want %v", calls, want)
	}
}

func TestDesktopTaskWorkspaceBindingsDelegateToCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/tasks/T-9":
			_, _ = io.WriteString(w, `{"task":{"task_id":"T-9","name":"Workspace","status":"Open"},"subtasks":[],"sprints":[],"children":{"subtask_ids":[],"sprint_ids":[],"open_subtasks":0,"open_sprints":0,"can_complete":true}}`)
		case "POST /api/v1/tasks/T-9/status":
			_, _ = io.WriteString(w, `{"task_id":"T-9","name":"Workspace","status":"On Hold"}`)
		case "POST /api/v1/sprints/SP-10/start":
			_, _ = io.WriteString(w, `{"sprint_id":"SP-10","name":"First cut","status":"Active"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := desktop.NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return &executionTreeSession{client: client}, nil
	})
	app.startup(context.Background())

	detail, err := app.TaskDetail("T-9")
	if err != nil || detail.Task.TaskID != "T-9" || !detail.Children.CanComplete {
		t.Fatalf("TaskDetail() = %+v, %v", detail, err)
	}
	task, err := app.UpdateTaskStatus("T-9", "On Hold")
	if err != nil || task.Status != "On Hold" {
		t.Fatalf("UpdateTaskStatus() = %+v, %v", task, err)
	}
	sprint, err := app.TransitionSprint("SP-10", "start", "")
	if err != nil || sprint.Status != "Active" {
		t.Fatalf("TransitionSprint() = %+v, %v", sprint, err)
	}
}

type executionTreeSession struct {
	client *desktop.CoreClient
}

func (s *executionTreeSession) Client() *desktop.CoreClient { return s.client }
func (s *executionTreeSession) Owned() bool                 { return false }
func (s *executionTreeSession) Close(context.Context) error { return nil }
