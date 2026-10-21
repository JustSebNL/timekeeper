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

func TestExecutionBatchValidateIsReadOnlyAndReturnsActionableIssues(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "execution-batch-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Batch API"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"` + category.CategoryID + `","name":"New Task","subtasks":[{"name":"Child","sprints":[{"name":"Build","estimated_duration_seconds":60}]}]}]}`
	response := serveExecutionBatchRequest(database, http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/execution-batches/validate", body)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result model.BatchValidationResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Counts != (model.BatchCounts{Tasks: 1, Subtasks: 1, Sprints: 1, TotalObjects: 3}) {
		t.Fatalf("unexpected smoke result: %#v", result)
	}
	afterTasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil || len(afterTasks) != 0 {
		t.Fatalf("validation wrote Tasks=%d err=%v", len(afterTasks), err)
	}
	afterEvents, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil || len(afterEvents) != len(beforeEvents) {
		t.Fatalf("validation wrote Project events: before=%d after=%d err=%v", len(beforeEvents), len(afterEvents), err)
	}

	invalid := `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-999999","name":"Wrong category"}]}`
	response = serveExecutionBatchRequest(database, http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/execution-batches/validate", invalid)
	if response.Code != http.StatusOK {
		t.Fatalf("invalid document status=%d body=%s", response.Code, response.Body.String())
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Valid || len(result.Issues) != 1 || result.Issues[0].Path != "tasks[0]" || result.Issues[0].Field != "category_id" || result.Issues[0].Suggestion == "" {
		t.Fatalf("validation must return actionable issue: %#v", result)
	}
}

func TestExecutionBatchApplyAndLegacyTaskBatchValidateRoutes(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "execution-batch-api-apply.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Batch API apply"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"` + category.CategoryID + `","name":"Parent","subtasks":[{"name":"Child","sprints":[{"name":"Sprint","estimated_duration_seconds":120}]}]}]}`
	response := serveExecutionBatchRequest(database, http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/execution-batches", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("apply status=%d body=%s", response.Code, response.Body.String())
	}
	var created model.ExecutionBatchResult
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Counts.TotalObjects != 3 || len(created.Tasks) != 1 || len(created.Subtasks) != 1 || len(created.Sprints) != 1 || created.Sprints[0].SubtaskID != created.Subtasks[0].SubtaskID {
		t.Fatalf("apply response omitted created hierarchy: %#v", created)
	}

	legacy := `{"tasks":[{"category_id":"` + category.CategoryID + `","name":"Legacy Task"}]}`
	response = serveExecutionBatchRequest(database, http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/tasks/batch/validate", legacy)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy validation status=%d body=%s", response.Code, response.Body.String())
	}
	var validation model.BatchValidationResult
	if err := json.NewDecoder(response.Body).Decode(&validation); err != nil || !validation.Valid || validation.Counts.Tasks != 1 {
		t.Fatalf("legacy batch validation=%#v err=%v", validation, err)
	}
}

func TestExecutionBatchRejectsUnknownAndTrailingJSONAndDoesNotApplyInvalid(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "execution-batch-api-invalid.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Batch API invalid"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/projects/" + project.ProjectID + "/execution-batches"
	for _, body := range []string{
		`{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-1","name":"Unknown key","surprise":true}]}`,
		`{"version":"timekeeper-execution-batch/v1","tasks":[]} {}`,
	} {
		response := serveExecutionBatchRequest(database, http.MethodPost, path, body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("malformed/unknown document status=%d body=%s", response.Code, response.Body.String())
		}
	}
	invalid := `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-999999","name":"No such category"}]}`
	response := serveExecutionBatchRequest(database, http.MethodPost, path, invalid)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid apply status=%d body=%s", response.Code, response.Body.String())
	}
	var rejection struct {
		Valid  bool                    `json:"valid"`
		Issues []model.ValidationIssue `json:"issues"`
	}
	if err := json.NewDecoder(response.Body).Decode(&rejection); err != nil || rejection.Valid || len(rejection.Issues) == 0 {
		t.Fatalf("invalid apply must return structured feedback: %#v err=%v", rejection, err)
	}
	afterTasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil || len(afterTasks) != 0 {
		t.Fatalf("invalid apply wrote Tasks=%d err=%v", len(afterTasks), err)
	}
}

func serveExecutionBatchRequest(database *store.Store, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	return response
}
