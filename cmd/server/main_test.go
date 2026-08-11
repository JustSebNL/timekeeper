// Copyright (c) 2026 Seb. All rights reserved.

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
	if strings.Count(string(body), "system_prompt:systemPrompt.value.trim()") != 1 || !strings.Contains(string(body), "function renderLocalPipelineForm") {
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
	if !strings.Contains(string(body), "'On Hold': ['resume']") {
		t.Fatal("dashboard must present resume for an On Hold Sprint")
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
	if message != "Time Keeper backup created: "+destination {
		t.Fatalf("message = %q", message)
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
	for _, asset := range []string{"timekeeper.html", "timekeeper.js"} {
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
	uiPath := filepath.Join("..", "..", "web", "timekeeper.html")
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
	for _, marker := range []string{"data-timekeeper-ui=\"v1\"", "/timekeeper.css", "/timekeeper.js"} {
		if !strings.Contains(body, marker) {
			t.Fatalf("dashboard shell missing required marker %q", marker)
		}
	}
	var assets strings.Builder
	for _, route := range []string{"/timekeeper.css", "/timekeeper.js"} {
		assetRequest := httptest.NewRequest(http.MethodGet, route, nil)
		assetResponse := httptest.NewRecorder()
		dashboard(uiPath).ServeHTTP(assetResponse, assetRequest)
		if assetResponse.Code != http.StatusOK {
			t.Fatalf("dashboard asset %s status = %d", route, assetResponse.Code)
		}
		assets.Write(assetResponse.Body.Bytes())
	}
	for _, marker := range []string{"/api/v1/projects", "/execution-tree", "/operational-summary", "/events", "/notes", "renderExecutionTree", "renderOperationalSummary", "planned_duration_seconds", "renderProjectEvents", "renderProjectNotes", "createInlineForm", "Buffer percent", "sprintActionButton", "durationToSeconds", "sprint.status", "X-Agent-ID"} {
		if !strings.Contains(assets.String(), marker) {
			t.Fatalf("dashboard assets missing required marker %q", marker)
		}
	}
}
