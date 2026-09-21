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
	if response.Version.VersionNumber != 3 || response.Version.ParentVersionID != second.VersionID {
		t.Fatalf("restore response=%+v", response.Version)
	}
	content, err := os.ReadFile(filePath)
	if err != nil || string(content) != "package one\n" {
		t.Fatalf("restored file=%q err=%v", content, err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/files/"+tracked.FileID+"/restore", strings.NewReader(`{"version_id":"`+first.VersionID+`","expected_current_version_id":"`+second.VersionID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale restore=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
