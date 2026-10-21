// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

// A full-replacement edit forces a caller to resubmit every field, which is
// how long descriptions get silently cleared during iterative audits.
func TestPatchTaskMetadataPreservesOmittedFields(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Patch Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Work"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{
		Name: "Audited task", CategoryID: category.CategoryID,
		Goal:        "Original goal",
		Description: "Original long description",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	handler := api.New(database)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+task.TaskID+"/metadata", bytes.NewBufferString(`{"description":"Revised description"}`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("patch status = %d body = %s", response.Code, response.Body.String())
	}
	var patched model.Task
	if err := json.Unmarshal(response.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode patched task: %v", err)
	}
	if patched.Description != "Revised description" {
		t.Fatalf("description not applied: %q", patched.Description)
	}
	if patched.Goal != "Original goal" {
		t.Fatalf("omitted goal was cleared: %q", patched.Goal)
	}

	empty := httptest.NewRecorder()
	emptyRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+task.TaskID+"/metadata", bytes.NewBufferString(`{}`))
	emptyRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(empty, emptyRequest)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty patch status = %d body = %s", empty.Code, empty.Body.String())
	}
}
