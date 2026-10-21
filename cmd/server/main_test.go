// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestSecurityHeadersConstrainDashboardExecution(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	csp := response.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "'unsafe-inline'") || !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "style-src 'self'") || !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "connect-src 'self'") || !strings.Contains(csp, "base-uri 'none'") || !strings.Contains(csp, "object-src 'none'") {
		t.Fatalf("CSP = %q", csp)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := response.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q", got)
	}
	if got := response.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header().Get("Cross-Origin-Resource-Policy"); got != "same-origin" {
		t.Fatalf("Cross-Origin-Resource-Policy = %q", got)
	}
	if got := response.Header().Get("Permissions-Policy"); got != "camera=(), geolocation=(), microphone=()" {
		t.Fatalf("Permissions-Policy = %q", got)
	}
}

func TestDashboardProvidesCategoryMetadataControl(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{"function renderCategoryMetadata", "Category context", "/api/v1/categories/"} {
		if !strings.Contains(text, required) {
			t.Fatalf("dashboard missing %q", required)
		}
	}
}

func TestDashboardRecursivelyRendersNestedCategories(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{"function renderCategoryNode", "renderCategoryNode(child", "parent_category_id: categoryNode.category.category_id", "New child category"} {
		if !strings.Contains(text, required) {
			t.Fatalf("dashboard missing %q", required)
		}
	}
}

func TestDashboardProvidesCategoryStatusControl(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, marker := range []string{"function renderCategoryStatus", "Category status", "/api/v1/categories/", "/status"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("dashboard missing %q", marker)
		}
	}
}

func TestDashboardProvidesSubtaskStatusControl(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, marker := range []string{"function renderSubtaskStatus", "Subtask status", "/api/v1/subtasks/", "/status"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("dashboard missing %q", marker)
		}
	}
}

func TestDashboardProvidesSprintTimeEntryHistory(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, marker := range []string{"function sprintTimeEntryPanel", "/time-entries", "Recorded intervals", "entry.entry_type"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("dashboard missing %q", marker)
		}
	}
}

func TestDashboardProvidesSprintExtensionEntryAndHistory(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, marker := range []string{"function sprintExtensionPanel", "/extensions", "Extend Sprint", "Extension reason"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("dashboard missing %q", marker)
		}
	}
}

func TestDashboardUsesOneLocalPipelineFormImplementation(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	if strings.Count(string(body), "system_prompt:") != 1 || !strings.Contains(string(body), "function renderLocalPipelineForm") {
		t.Fatal("dashboard pipeline creation must share one form implementation")
	}
}

func TestDashboardCanAddAnotherLocalPipeline(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	if !strings.Contains(string(body), "Add another local planner") {
		t.Fatal("dashboard must allow adding a local pipeline after one already exists")
	}
}

func TestDashboardPipelineConfigurationSupportsOptionalSystemPrompt(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	if !strings.Contains(string(body), "Optional planning instructions") || !strings.Contains(string(body), "system_prompt") {
		t.Fatal("dashboard pipeline configuration must expose an optional persisted system prompt")
	}
}

func TestDashboardProvidesProjectMetadataEditor(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "Project context") || !strings.Contains(text, "/metadata") || !strings.Contains(text, "Update context") {
		t.Fatal("dashboard must provide durable Project goal/description editing")
	}
}

func TestDashboardDoesNotDuplicateControlRenderers(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	text := string(body)
	for _, name := range []string{"renderProjectNotes", "renderProjectStatus"} {
		if count := strings.Count(text, "function "+name); count != 1 {
			t.Fatalf("%s declarations = %d, want 1", name, count)
		}
	}
}

func TestDashboardOperationalSummaryUsesAPIFields(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "summary.held_sprints") || !strings.Contains(text, "summary.recorded_work_seconds") || !strings.Contains(text, "summary.recorded_hold_seconds") || !strings.Contains(text, "summary.extension_duration_seconds") || strings.Contains(text, "summary.on_hold_sprints") || strings.Contains(text, "summary.active_duration_seconds") {
		t.Fatal("dashboard operational summary must use the API's held_sprints and recorded_work_seconds fields")
	}
}

func TestDashboardProvidesLocalPipelineConfiguration(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "Configure local planner") || !strings.Contains(text, "/api/v1/llm-pipelines") || !strings.Contains(text, "openai-compatible") {
		t.Fatal("dashboard must provide explicit local-only pipeline configuration")
	}
}

func TestDashboardProvidesPlanningDraftReviewAndApplyControls(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "Generate plan") || !strings.Contains(text, "planning-drafts/generate") || !strings.Contains(text, "Apply approved draft") {
		t.Fatal("dashboard must provide explicit local planning review/apply controls")
	}
}

func TestDashboardProvidesTaskStatusControl(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	if !strings.Contains(string(body), "Task status") || !strings.Contains(string(body), "/api/v1/tasks/") {
		t.Fatal("dashboard must expose Task status updates through the API")
	}
}

func TestDashboardProvidesProjectStatusControl(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	if !strings.Contains(string(body), "Project status") || !strings.Contains(string(body), "/status") {
		t.Fatal("dashboard must expose Project status updates through the API")
	}
}

func TestDashboardShowsOnlyLegalSprintActions(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read current dashboard: %v", err)
	}
	if strings.Contains(string(body), "'On Hold': ['resume', 'complete']") {
		t.Fatal("dashboard must not present complete for an On Hold Sprint")
	}
	if !strings.Contains(string(body), "'On Hold': ['resume', 'cancel']") {
		t.Fatal("dashboard must present resume and reasoned cancellation for an On Hold Sprint")
	}
}

func TestValidateServerConfigRejectsUnreadableDashboard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows readability is controlled by ACLs, not os.FileMode permissions")
	}
	uiPath := filepath.Join(t.TempDir(), "private.html")
	if err := os.WriteFile(uiPath, []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatalf("write ui fixture: %v", err)
	}
	if err := os.Chmod(uiPath, 0o000); err != nil {
		t.Fatalf("make ui unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(uiPath, 0o600) })
	if err := validateServerConfig("127.0.0.1:1618", uiPath); err == nil {
		t.Fatal("unreadable dashboard must fail configuration validation")
	}
}

func TestValidateServerConfigRestrictsServingToLoopback(t *testing.T) {
	uiPath := filepath.Join(t.TempDir(), "timekeeper.html")
	if err := os.WriteFile(uiPath, []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatalf("write ui fixture: %v", err)
	}
	for _, asset := range []string{"timekeeper.css", "timekeeper.js"} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(uiPath), asset), []byte("/* fixture */"), 0o600); err != nil {
			t.Fatalf("write %s fixture: %v", asset, err)
		}
	}
	for _, test := range []struct {
		addr string
		want bool
	}{
		{addr: "127.0.0.1:1618", want: true},
		{addr: "[::1]:1618", want: true},
		{addr: "localhost:1618", want: false},
		{addr: "0.0.0.0:1618", want: false},
		{addr: ":1618", want: false},
		{addr: "example.com:1618", want: false},
		{addr: "127.0.0.1:not-a-port", want: false},
	} {
		err := validateServerConfig(test.addr, uiPath)
		if (err == nil) != test.want {
			t.Fatalf("validateServerConfig(%q) error = %v, want allowed=%v", test.addr, err, test.want)
		}
	}
}

func TestRunBackupCreatesSnapshotAndReportsPath(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Backup command"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "backup.db")
	message, err := runBackup(ctx, database, destination)
	if err != nil {
		t.Fatalf("run backup: %v", err)
	}
	if message != "Time Keeper backup created and integrity-verified: "+destination {
		t.Fatalf("message = %q", message)
	}
	if err := store.CheckDatabaseFileIntegrity(ctx, destination); err != nil {
		t.Fatalf("backup candidate integrity: %v", err)
	}
	copy, err := store.Open(destination)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = copy.Close() })
	projects, err := copy.ListProjects(ctx)
	if err != nil || len(projects) != 1 || projects[0].ProjectName != "Backup command" {
		t.Fatalf("backup projects = %#v, err = %v", projects, err)
	}
}

func TestDashboardAvoidsHTMLStringInsertion(t *testing.T) {
	var all strings.Builder
	for _, asset := range []string{"index.html", "timekeeper.js"} {
		body, err := os.ReadFile(filepath.Join("..", "..", "web", asset))
		if err != nil {
			t.Fatalf("read dashboard asset %s: %v", asset, err)
		}
		all.Write(body)
	}
	if strings.Contains(all.String(), "innerHTML") {
		t.Fatal("dashboard must create even static DOM nodes without innerHTML")
	}
}

func TestDashboardServesCurrentFrameworkNeutralUI(t *testing.T) {
	uiPath := filepath.Join("..", "..", "web", "index.html")
	if _, err := os.Stat(uiPath); err != nil {
		t.Fatalf("current dashboard missing: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	dashboard(uiPath).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d", response.Code)
	}
	body := response.Body.String()
	for _, marker := range []string{"data-timekeeper-ui=\"v2\"", `id="focus-card"`, `id="focus"`, `id="usage-card"`, `id="usage-timeline"`, `id="usage-session-list"`, "/timekeeper.css", "/timekeeper.js"} {
		if !strings.Contains(body, marker) {
			t.Fatalf("dashboard shell missing required marker %q", marker)
		}
	}
	var assets strings.Builder
	for _, route := range []string{"/timekeeper.css", "/timekeeper.js", "/vendor/bootstrap-5.3.8.min.css"} {
		assetRequest := httptest.NewRequest(http.MethodGet, route, nil)
		assetResponse := httptest.NewRecorder()
		dashboard(uiPath).ServeHTTP(assetResponse, assetRequest)
		if assetResponse.Code != http.StatusOK {
			t.Fatalf("dashboard asset %s status = %d", route, assetResponse.Code)
		}
		assets.Write(assetResponse.Body.Bytes())
	}
	for _, marker := range []string{"/api/v1/projects", "/api/v1/pulse", "/execution-tree", "/operational-summary", "/usage-summary", "/events", "/notes", "renderExecutionTree", "renderOperationalSummary", "renderPulse", "renderFocus", "renderUsage", "planned_duration_seconds", "renderProjectEvents", "renderProjectNotes", "createInlineForm", "Buffer percent", "sprintActionButton", "durationToSeconds", "sprint.status", "X-Agent-ID"} {
		if !strings.Contains(assets.String(), marker) {
			t.Fatalf("dashboard assets missing required marker %q", marker)
		}
	}
}

func TestDashboardServesListedHarnessFromFriendlyQueryLink(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"harness/SKILL.md":                   "# Contract\n",
		"harness/manifest.yaml":              "version: 1\nskills:\n  - harness/skills/bug-hunter/SKILL.md\n",
		"harness/skills/bug-hunter/SKILL.md": "# Bug hunter\nTrace it.\n",
		"docs/HARNESS.md":                    "# First use\n",
		"docs/agent.md":                      "# Agent router\n",
		"web/index.html":                     "<html>dashboard</html>",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TIMEKEEPER_REPO", root)
	handler := dashboard(filepath.Join(root, "web", "index.html"))

	for _, test := range []struct {
		url  string
		want string
	}{
		{"/?bug-hunter.md", "# Bug hunter\nTrace it.\n"},
		{"/?harness=agent-router", "# Agent router\n"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.url, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/markdown; charset=utf-8" || response.Body.String() != test.want {
			t.Fatalf("GET %s status=%d content-type=%q body=%q", test.url, response.Code, response.Header().Get("Content-Type"), response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/?manifest.yaml", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/yaml; charset=utf-8" || !strings.Contains(response.Body.String(), "harness/skills/bug-hunter/SKILL.md") {
		t.Fatalf("YAML link status=%d content-type=%q body=%q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}

	for _, test := range []struct {
		url    string
		method string
		status int
	}{
		{"/?not-listed.md", http.MethodGet, http.StatusNotFound},
		{"/?../../secret.md", http.MethodGet, http.StatusNotFound},
		{"/?bug-hunter.md", http.MethodPost, http.StatusMethodNotAllowed},
	} {
		request := httptest.NewRequest(test.method, test.url, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%s %s status=%d want=%d", test.method, test.url, response.Code, test.status)
		}
	}

	request = httptest.NewRequest(http.MethodGet, "/?tab=projects", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "dashboard") {
		t.Fatalf("ordinary dashboard query status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestDashboardServesHTMLHarnessReaderToBrowsers(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"harness/SKILL.md":                   "# Contract\n",
		"harness/manifest.yaml":              "version: 1\nskills:\n  - harness/skills/bug-hunter/SKILL.md\n",
		"harness/skills/bug-hunter/SKILL.md": "# Bug hunter\nTrace <it> & \"copy\".\n",
		"docs/HARNESS.md":                    "# First use\n",
		"docs/agent.md":                      "# Agent router\n",
		"web/index.html":                     "<html>dashboard</html>",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TIMEKEEPER_REPO", root)
	handler := dashboard(filepath.Join(root, "web", "index.html"))

	// A browser Accept header (any Accept containing text/html) gets the
	// shared HTML viewer preselected to the linked harness ID.
	request := httptest.NewRequest(http.MethodGet, "/harness.html?item=bug-hunter", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("HTML reader status=%d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("HTML reader content-type=%q", got)
	}
	for _, marker := range []string{
		`<ul class="harness-menu" id="harness-menu" role="listbox">`,
		`<textarea class="harness-source" id="harness-source" readonly`,
		`<script src="/harness-viewer.js"></script>`,
		`<link rel="stylesheet" href="/harness.css">`,
		`<body data-selected-id="bug-hunter">`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("HTML reader missing marker %q in body=%q", marker, body)
		}
	}
	// The source is fetched into textarea.value by the viewer script, never
	// interpolated into the HTML shell.
	if strings.Contains(body, "Trace <it>") || strings.Contains(body, "Trace &lt;it&gt;") {
		t.Fatal("HTML shell must not embed harness source; it is loaded as text content")
	}
	// No raw-markdown fallback headers on the HTML path.
	if got := response.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("HTML reader must not set Content-Disposition, got %q", got)
	}

	// The YAML manifest also gets the HTML reader when a browser asks.
	request = httptest.NewRequest(http.MethodGet, "/?manifest.yaml", nil)
	request.Header.Set("Accept", "text/html,*/*;q=0.8")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/html; charset=utf-8" || !strings.Contains(response.Body.String(), `data-selected-id="manifest"`) {
		t.Fatalf("YAML HTML reader status=%d content-type=%q body=%q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}

	// */* and text/markdown Accept headers keep receiving the raw bytes so
	// API clients never need to parse the reader page.
	for _, test := range []struct {
		accept      string
		contentType string
	}{
		{"*/*", "text/markdown; charset=utf-8"},
		{"text/markdown", "text/markdown; charset=utf-8"},
		{"application/yaml, */*", "application/yaml; charset=utf-8"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/?bug-hunter.md", nil)
		if test.accept != "application/yaml, */*" {
			request.Header.Set("Accept", test.accept)
		} else {
			request = httptest.NewRequest(http.MethodGet, "/?manifest.yaml", nil)
			request.Header.Set("Accept", test.accept)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != test.contentType {
			t.Errorf("Accept %q status=%d content-type=%q want=%d/%q", test.accept, response.Code, response.Header().Get("Content-Type"), http.StatusOK, test.contentType)
		}
	}

	// No Accept header (curl-style) keeps the raw markdown bytes.
	request = httptest.NewRequest(http.MethodGet, "/?bug-hunter.md", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/markdown; charset=utf-8" || response.Body.String() != "# Bug hunter\nTrace <it> & \"copy\".\n" {
		t.Fatalf("no-Accept status=%d content-type=%q body=%q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}

func TestHarnessViewerCatalogAndContentAreDirectoryDriven(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"harness/SKILL.md":              "# Contract\n",
		"harness/manifest.yaml":         "version: 1\nskills:\n  - harness/skills/alpha/SKILL.md\n  - harness/skills/beta/SKILL.md\n",
		"harness/skills/alpha/SKILL.md": "---\ndescription: Alpha harness\n---\n# Alpha body\n",
		"harness/skills/beta/SKILL.md":  "---\ndescription: Beta harness\n---\n# Beta body\n",
		"harness/extra-guide.md":        "# Auto-discovered guide\n",
		"docs/HARNESS.md":               "# Getting started\n",
		"docs/agent.md":                 "# Router\n",
		"web/index.html":                "<html>dashboard</html>",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TIMEKEEPER_REPO", root)
	handler := dashboard(filepath.Join(root, "web", "index.html"))

	request := httptest.NewRequest(http.MethodGet, "/harness.html", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `id="harness-menu"`) || !strings.Contains(response.Body.String(), `id="harness-source"`) {
		t.Fatalf("viewer status=%d body=%q", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/harness/catalog", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ID":"alpha"`) || !strings.Contains(response.Body.String(), `"ID":"beta"`) || !strings.Contains(response.Body.String(), `"ID":"extra-guide"`) {
		t.Fatalf("catalog status=%d body=%q", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/harness/content?id=beta", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "---\ndescription: Beta harness\n---\n# Beta body\n" {
		t.Fatalf("content status=%d body=%q", response.Code, response.Body.String())
	}

	// A removed/renamed file left behind in the manifest must not break the
	// viewer catalog; the directory is the inventory authority.
	if err := os.Remove(filepath.Join(root, "harness/skills/beta/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/harness/catalog", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"ID":"beta"`) || !strings.Contains(response.Body.String(), `"ID":"extra-guide"`) {
		t.Fatalf("catalog after removal status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestDashboardProvidesRecentActivityPanel(t *testing.T) {
	html, err := os.ReadFile(filepath.Join("..", "..", "web", "index.html"))
	if err != nil {
		t.Fatalf("read dashboard HTML: %v", err)
	}
	for _, marker := range []string{`id="activity-card"`, `id="activity"`, "Recent activity"} {
		if !strings.Contains(string(html), marker) {
			t.Fatalf("dashboard HTML missing %q", marker)
		}
	}
	js, err := os.ReadFile(filepath.Join("..", "..", "web", "timekeeper.js"))
	if err != nil {
		t.Fatalf("read dashboard JavaScript: %v", err)
	}
	for _, marker := range []string{"function renderRecentActivity", "function loadRecentActivity", "/events"} {
		if !strings.Contains(string(js), marker) {
			t.Fatalf("dashboard JavaScript missing %q", marker)
		}
	}
}

func TestKeepAliveOnceUsesHealthEndpoint(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("keep-alive path = %q", r.URL.Path)
		}
		called = true
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)

	keepAliveOnce(context.Background(), server.Client(), strings.TrimPrefix(server.URL, "http://"))
	if !called {
		t.Fatal("keep-alive did not call /health")
	}
}

func TestWatchParentInputStopsOnEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	stopped := make(chan struct{})
	watchParentInput(ctx, reader, true, func() {
		close(stopped)
		cancel()
	})
	select {
	case <-stopped:
		t.Fatal("parent-close watcher stopped before stdin EOF")
	case <-time.After(25 * time.Millisecond):
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stdin EOF did not request shutdown")
	}
}

func TestWatchParentInputDisabledIgnoresEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	watchParentInput(ctx, reader, false, cancel)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal("disabled parent-close watcher observed stdin EOF")
	case <-time.After(40 * time.Millisecond):
	}
}

func TestServeShutdownClosesPrimaryAndProxy(t *testing.T) {
	primaryListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer primaryListener.Close()
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyListener.Close()
	primary := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })}
	proxy := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- serveUntilShutdown(ctx, primary, primaryListener, proxy, proxyListener)
	}()
	waitForServer(t, primaryListener.Addr().String())
	waitForServer(t, proxyListener.Addr().String())
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("serveUntilShutdown() = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serveUntilShutdown did not return after cancellation")
	}
	assertServerStopped(t, primaryListener.Addr().String())
	assertServerStopped(t, proxyListener.Addr().String())
}

func TestServeShutdownStopsBothListenersBeforeDraining(t *testing.T) {
	primaryListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer primaryListener.Close()
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyListener.Close()

	started := make(chan string, 4)
	release := make(chan struct{})
	releaseRequests := func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}
	defer releaseRequests()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	primary := &http.Server{Handler: handler}
	proxy := &http.Server{Handler: handler}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- serveUntilShutdown(ctx, primary, primaryListener, proxy, proxyListener)
	}()
	requestDone := make(chan struct{}, 2)
	for _, target := range []struct {
		addr string
		path string
	}{{primaryListener.Addr().String(), "/primary"}, {proxyListener.Addr().String(), "/proxy"}} {
		go func(target struct{ addr, path string }) {
			resp, err := http.Get("http://" + target.addr + target.path)
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err != nil {
				t.Errorf("active request %s: %v", target.path, err)
			}
			requestDone <- struct{}{}
		}(target)
	}
	seen := map[string]bool{}
	for len(seen) != 2 {
		select {
		case path := <-started:
			seen[path] = true
		case <-time.After(time.Second):
			t.Fatalf("active requests did not start on both listeners: %v", seen)
		}
	}
	cancel()

	probeDone := make(chan error, 1)
	go func() {
		client := http.Client{Timeout: time.Second}
		resp, err := client.Get("http://" + proxyListener.Addr().String() + "/probe")
		if resp != nil {
			_ = resp.Body.Close()
		}
		probeDone <- err
	}()
	select {
	case path := <-started:
		if path == "/probe" {
			t.Fatal("proxy listener accepted new work while the primary listener was draining")
		}
	case err := <-probeDone:
		if err == nil {
			t.Fatal("probe unexpectedly received a response from the proxy during shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("proxy listener shutdown was not observable")
	}
	releaseRequests()
	for range 2 {
		select {
		case <-requestDone:
		case <-time.After(time.Second):
			t.Fatal("active request did not drain")
		}
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("serveUntilShutdown() = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serveUntilShutdown did not return after active requests drained")
	}
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		client := &http.Client{Timeout: 100 * time.Millisecond}
		resp, err := client.Get("http://" + addr + "/")
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s did not start", addr)
}

func assertServerStopped(t *testing.T, addr string) {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + addr + "/")
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatalf("server at %s accepted a request after shutdown", addr)
	}
}
