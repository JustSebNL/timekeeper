// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/history"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectRootAndScanRoutesDiscoverAndCaptureProjectTextFiles(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	root := filepath.Join(workspace, "source")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "src", "main.go")
	if err := os.WriteFile(mainPath, []byte("package root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(filepath.Join(workspace, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Root scan API"})
	if err != nil {
		t.Fatal(err)
	}
	historyService, err := history.New(filepath.Join(workspace, "history"), database)
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewWithHistory(database, api.RuntimeStatus{}, nil, historyService)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/file-root", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"root_path":""`) {
		t.Fatalf("unset source root status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ProjectID+"/file-root", strings.NewReader(`{"root_path":"relative/path"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("relative source root status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/files/scan", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("scan without source root status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	rootJSON, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"root_path":` + string(rootJSON) + `}`
	request = httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ProjectID+"/file-root", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var configuredRoot model.ProjectFileRoot
	if err := json.Unmarshal(recorder.Body.Bytes(), &configuredRoot); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || configuredRoot.RootPath != root {
		t.Fatalf("set source root status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/files/scan", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"files_added":1`) || !strings.Contains(recorder.Body.String(), `"versions_added":1`) {
		t.Fatalf("scan status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	files, err := database.ListTrackedFiles(ctx, project.ProjectID)
	if err != nil || len(files) != 1 || files[0].CurrentPath != "src/main.go" {
		t.Fatalf("tracked files=%+v err=%v", files, err)
	}
}

func TestProjectFilesReturnsEmptyJSONArrayWhenNoTrackedFiles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	database, err := store.Open(filepath.Join(root, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Empty history list"})
	if err != nil {
		t.Fatal(err)
	}
	historyService, err := history.New(filepath.Join(root, "history"), database)
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewWithHistory(database, api.RuntimeStatus{}, nil, historyService)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/files", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("empty project files status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []model.TrackedFile `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Items == nil {
		t.Fatalf("empty project file list encoded as null, want []: %s", recorder.Body.String())
	}
	if len(response.Items) != 0 {
		t.Fatalf("empty project file list=%+v, want zero items", response.Items)
	}
}

func TestFileHistoryAPIListsComparesAndRestores(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	filePath := filepath.Join(root, "main.go")
	if err := os.WriteFile(filePath, []byte("package one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(filepath.Join(root, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "History API"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := history.New(root, database)
	if err != nil {
		t.Fatal(err)
	}
	tracked, first, err := service.Capture(ctx, history.CaptureInput{ProjectID: project.ProjectID, RelativePath: "main.go", FileType: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("package two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, second, err := service.Capture(ctx, history.CaptureInput{ProjectID: project.ProjectID, RelativePath: "main.go", FileType: "go"})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewWithHistory(database, api.RuntimeStatus{}, nil, service)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+tracked.FileID+"/versions", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), first.VersionID) || !strings.Contains(recorder.Body.String(), second.VersionID) {
		t.Fatalf("versions=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/files/"+tracked.FileID+"/compare?from="+first.VersionID+"&to="+second.VersionID, nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "package one") || !strings.Contains(recorder.Body.String(), "package two") {
		t.Fatalf("compare=%d body=%s", recorder.Code, recorder.Body.String())
	}

	body := `{"version_id":"` + first.VersionID + `","expected_current_version_id":"` + second.VersionID + `"}`
	request = httptest.NewRequest(http.MethodPost, "/api/v1/files/"+tracked.FileID+"/restore", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("restore=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Version model.FileVersion `json:"version"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Version.VersionNumber != 3 || response.Version.ParentVersionID != second.VersionID || response.Version.RestoresVersionID != first.VersionID || response.Version.ChangeKind != model.FileChangeReversion {
		t.Fatalf("restore response=%+v", response.Version)
	}
	content, err := os.ReadFile(filePath)
	if err != nil || string(content) != "package one\n" {
		t.Fatalf("restored file=%q err=%v", content, err)
	}

	body = `{"content":"package fix\n","expected_current_version_id":"` + response.Version.VersionID + `","change_kind":"bugfix"}`
	request = httptest.NewRequest(http.MethodPost, "/api/v1/files/"+tracked.FileID+"/versions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("typed save=%d body=%s", recorder.Code, recorder.Body.String())
	}
	response.Version = model.FileVersion{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Version.ChangeKind != model.FileChangeBugfix || response.Version.RestoresVersionID != "" {
		t.Fatalf("typed save metadata=%+v", response.Version)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/files/"+tracked.FileID+"/restore", strings.NewReader(`{"version_id":"`+first.VersionID+`","expected_current_version_id":"`+second.VersionID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale restore=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestFileSnapshotBoundariesCreateRestorableVersions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	filePath := filepath.Join(root, "main.go")
	if err := os.WriteFile(filePath, []byte("package base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(filepath.Join(root, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Snapshot steps"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := history.New(root, database)
	if err != nil {
		t.Fatal(err)
	}
	tracked, initial, err := service.Capture(ctx, history.CaptureInput{ProjectID: project.ProjectID, RelativePath: "main.go", FileType: "go"})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewWithHistory(database, api.RuntimeStatus{}, nil, service)
	capture := func(phase, content, summary string) model.FileVersion {
		t.Helper()
		if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		body := `{"phase":"` + phase + `","change_summary":"` + summary + `"}`
		request := httptest.NewRequest(http.MethodPost, "/api/v1/files/"+tracked.FileID+"/snapshots", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("snapshot %s=%d body=%s", phase, recorder.Code, recorder.Body.String())
		}
		var response struct {
			Version model.FileVersion `json:"version"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Version.SnapshotType != "full" || !strings.HasPrefix(response.Version.ChangeSummary, phase) {
			t.Fatalf("snapshot metadata=%+v", response.Version)
		}
		return response.Version
	}
	start := capture("code_start", "package start\n", "before edit")
	end := capture("code_end", "package end\n", "after edit")
	if start.ParentVersionID != initial.VersionID || end.ParentVersionID != start.VersionID {
		t.Fatalf("snapshot chain: start=%+v end=%+v", start, end)
	}
	_, restored, err := service.Restore(ctx, history.RestoreInput{FileID: tracked.FileID, VersionID: start.VersionID, ExpectedCurrentVersionID: end.VersionID})
	if err != nil {
		t.Fatal(err)
	}
	if restored.ParentVersionID != end.VersionID {
		t.Fatalf("restore did not append after latest step: %+v", restored)
	}
	content, err := os.ReadFile(filePath)
	if err != nil || string(content) != "package start\n" {
		t.Fatalf("restored step=%q err=%v", content, err)
	}
	versions, err := database.ListFileVersions(ctx, tracked.FileID)
	if err != nil || len(versions) != 4 {
		t.Fatalf("history=%+v err=%v", versions, err)
	}
}
