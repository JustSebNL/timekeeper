package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestTaskBatchSmokePassThenApplyForNestedAndLegacyFiles(t *testing.T) {
	for _, test := range []struct {
		name, file, validatePath, applyPath string
	}{
		{name: "nested", file: `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Parent","subtasks":[{"name":"Child"}]}]}`, validatePath: "/execution-batches/validate", applyPath: "/execution-batches"},
		{name: "legacy", file: `{"tasks":[{"category_id":"C-10","name":"Legacy"}]}`, validatePath: "/tasks/batch/validate", applyPath: "/tasks/batch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
			path := writeExecutionBatchFixture(t, "batch.json", test.file)
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				if strings.HasSuffix(r.URL.Path, "/validate") {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(model.BatchValidationResult{Valid: true, Counts: model.BatchCounts{Tasks: 1, Subtasks: 1, TotalObjects: 2}, Issues: []model.ValidationIssue{}})
					return
				}
				w.WriteHeader(http.StatusCreated)
				if test.name == "nested" {
					_, _ = fmt.Fprint(w, `{"tasks":[{"task_id":"T-10","name":"Parent","item_address":"P-1.C-1.10"}],"subtasks":[{"subtask_id":"ST-11","name":"Child","item_address":"P-1.C-1.10.11"}],"sprints":[],"counts":{"tasks":1,"subtasks":1,"sprints":0,"total_objects":2}}`)
				} else {
					_, _ = fmt.Fprint(w, `{"tasks":[{"task_id":"T-12","name":"Legacy","item_address":"P-1.C-1.12"}],"count":1}`)
				}
			}))
			defer server.Close()
			var stdout, stderr strings.Builder
			if code := taskBatch("P-1", []string{path}, &stdout, &stderr, server.URL); code != 0 {
				t.Fatalf("taskBatch code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if len(requests) != 2 || !strings.HasSuffix(requests[0], test.validatePath) || !strings.HasSuffix(requests[1], test.applyPath) {
				t.Fatalf("expected validation then apply, requests=%v", requests)
			}
			if !strings.Contains(stdout.String(), "✓ Smoke test passed") {
				t.Fatalf("successful validation must print the checkmark: %q", stdout.String())
			}
		})
	}
}

func TestTaskBatchSmokeFailurePrintsRepairsAndNeverApplies(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	path := writeExecutionBatchFixture(t, "batch.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Parent"}]}`)
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		_ = json.NewEncoder(w).Encode(model.BatchValidationResult{Valid: false, Counts: model.BatchCounts{Tasks: 1, TotalObjects: 1}, Issues: []model.ValidationIssue{
			{Path: "tasks[0]", Field: "category_id", Code: "foreign_category", Message: "Category belongs to a different Project", Suggestion: "Choose a Category from the target Project."},
			{Path: "tasks[0].sprints[0]", Field: "estimated_duration_seconds", Code: "invalid_estimate", Message: "Sprint estimate must be greater than zero", Suggestion: "Set a positive seconds value."},
		}})
	}))
	defer server.Close()
	var stdout, stderr strings.Builder
	if code := taskBatch("P-1", []string{path}, &stdout, &stderr, server.URL); code == 0 {
		t.Fatalf("invalid smoke test must fail: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if len(requests) != 1 || !strings.HasSuffix(requests[0], "/execution-batches/validate") {
		t.Fatalf("smoke failure must not call apply: %v", requests)
	}
	feedback := stdout.String() + stderr.String()
	for _, expected := range []string{"✗ Smoke test failed", "tasks[0].category_id", "foreign_category", "Choose a Category", "tasks[0].sprints[0].estimated_duration_seconds", "invalid_estimate", "Set a positive seconds"} {
		if !strings.Contains(feedback, expected) {
			t.Errorf("failure feedback missing %q: %s", expected, feedback)
		}
	}
	if strings.Contains(feedback, "✓") {
		t.Fatalf("failed smoke test printed a pass mark: %s", feedback)
	}
}

func TestTaskBatchCheckOnlyStopsAfterPassingSmokeTest(t *testing.T) {
	path := writeExecutionBatchFixture(t, "legacy.csv", "name,category_id\nLegacy,C-10\n")
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		_ = json.NewEncoder(w).Encode(model.BatchValidationResult{Valid: true, Counts: model.BatchCounts{Tasks: 1, TotalObjects: 1}, Issues: []model.ValidationIssue{}})
	}))
	defer server.Close()
	var stdout, stderr strings.Builder
	if code := taskBatch("P-1", []string{path, "--check-only"}, &stdout, &stderr, server.URL); code != 0 {
		t.Fatalf("check-only code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if len(requests) != 1 || !strings.HasSuffix(requests[0], "/tasks/batch/validate") {
		t.Fatalf("check-only must never apply: %v", requests)
	}
	if !strings.Contains(stdout.String(), "✓ Smoke test passed") || !strings.Contains(stdout.String(), "check-only") {
		t.Fatalf("check-only success should show pass mark and no-write notice: %q", stdout.String())
	}
}

func TestTaskBatchLocalParseFailureShowsRepairAndMakesNoRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"version":"timekeeper-execution-batch/v1","tasks":[{"name":"Missing category_id"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	var stdout, stderr strings.Builder
	if code := taskBatch("P-1", []string{path}, &stdout, &stderr, server.URL); code == 0 {
		t.Fatalf("invalid local batch must fail: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if requests != 0 || !strings.Contains(stderr.String(), "category_id") {
		t.Fatalf("local parser/structure error should be actionable before any request: requests=%d stderr=%q", requests, stderr.String())
	}
}

func TestTaskBatchRejectsMalformedLegacyFormatBeforeValidation(t *testing.T) {
	path := writeExecutionBatchFixture(t, "bad.csv", "name,unexpected\nTask,ignored\n")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	var stdout, stderr strings.Builder
	if code := taskBatch("P-1", []string{path}, &stdout, &stderr, server.URL); code == 0 {
		t.Fatalf("malformed CSV must fail before submission: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if requests != 0 || !strings.Contains(stderr.String(), "unknown column") {
		t.Fatalf("format error must be actionable and make no API request: requests=%d stderr=%q", requests, stderr.String())
	}
}

func TestTaskBatchPersistsLocallyWhenCoreIsUnreachable(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	path := writeExecutionBatchFixture(t, "offline-batch.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Offline offer"}]}`)
	var stdout, stderr strings.Builder
	code := taskBatch("P-10000", []string{path}, &stdout, &stderr, "http://127.0.0.1:1")
	if code != 0 {
		t.Fatalf("durable local acceptance code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	entries, err := listRegistrationOutbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].State.Status != registrationOutboxPending {
		t.Fatalf("offline batch was not retained pending: %+v", entries)
	}
	if entries[0].Offer.Kind != "execution_batch" || entries[0].Offer.ValidationPath != "/api/v1/projects/P-10000/execution-batches/validate" || entries[0].Offer.SubmitPath != "/api/v1/projects/P-10000/execution-batches" {
		t.Fatalf("offline batch routes or kind=%+v", entries[0].Offer)
	}
	if !strings.Contains(stdout.String(), entries[0].Offer.LocalID) || !strings.Contains(stdout.String(), "queued locally") {
		t.Fatalf("CLI did not report durable local queue acceptance: %q", stdout.String())
	}
}

func TestTaskBatchJSONOfflineOfferOutputsOnlyLocalReceiptJSON(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	path := writeExecutionBatchFixture(t, "offline-json-batch.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Offline JSON batch"}]}`)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--json", "t", "batch", "P-10000", path}, &stdout, &stderr, "http://127.0.0.1:1")
	if code != 0 {
		t.Fatalf("offline JSON batch offer code=%d out=%q err=%q", code, stdout.String(), stderr.String())
	}
	var receipt struct {
		LocalQueueID string `json:"local_queue_id"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil {
		t.Fatalf("offline batch stdout must be one JSON receipt: %v; raw=%q", err, stdout.String())
	}
	if receipt.LocalQueueID == "" || receipt.Status != registrationOutboxPending {
		t.Fatalf("offline batch JSON receipt=%+v", receipt)
	}
}

func TestTaskBatchDoesNotRetryAmbiguousApplyAndAdvisesTreeInspection(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	path := writeExecutionBatchFixture(t, "batch.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Parent"}]}`)
	applyRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/validate") {
			_ = json.NewEncoder(w).Encode(model.BatchValidationResult{Valid: true, Counts: model.BatchCounts{Tasks: 1, TotalObjects: 1}, Issues: []model.ValidationIssue{}})
			return
		}
		applyRequests++
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("test response writer does not support hijacking")
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("hijack apply response: %v", err)
		}
		_ = connection.Close()
	}))
	defer server.Close()
	var stdout, stderr strings.Builder
	if code := taskBatch("P-1", []string{path}, &stdout, &stderr, server.URL); code != 0 {
		t.Fatalf("durable outbox should preserve an ambiguous apply: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if applyRequests != 1 {
		t.Fatalf("ambiguous apply must never be retried automatically; requests=%d", applyRequests)
	}
	entries, err := listRegistrationOutbox()
	if err != nil || len(entries) != 1 || entries[0].State.Status != registrationOutboxPending {
		t.Fatalf("ambiguous apply was not retained for same-key retry: entries=%+v err=%v", entries, err)
	}
	if !strings.Contains(stdout.String(), "Apply outcome is unknown") || !strings.Contains(stderr.String(), "connection failed") {
		t.Fatalf("ambiguous apply guidance is missing: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestTaskBatchSmokeRejectsMissingProjectBeforeApply(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "missing-project-batch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Existing target"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Existing category"})
	if err != nil {
		t.Fatal(err)
	}
	path := writeExecutionBatchFixture(t, "missing-project.json", fmt.Sprintf(`{"version":"%s","tasks":[{"category_id":"%s","name":"Must not apply"}]}`, executionBatchVersion, category.CategoryID))

	var requests []string
	core := api.New(database)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		core.ServeHTTP(w, r)
	}))
	defer server.Close()
	var stdout, stderr strings.Builder
	if code := taskBatch("P-999999", []string{path}, &stdout, &stderr, server.URL); code == 0 {
		t.Fatalf("missing target Project must fail: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if len(requests) != 1 || !strings.HasSuffix(requests[0], "/execution-batches/validate") {
		t.Fatalf("missing Project must stop at one read-only validation request: %v", requests)
	}
	feedback := stdout.String() + stderr.String()
	if !strings.Contains(feedback, "project_not_found") || !strings.Contains(feedback, "target Project was not found") {
		t.Fatalf("missing Project smoke result was not actionable: %q", feedback)
	}
	tasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("failed smoke changed the existing Project: tasks=%d err=%v", len(tasks), err)
	}
}

func TestTaskBatchEndToEndWithDisposableCoreAndStore(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "batch-cli-integration.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Batch CLI integration"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Batch integration"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.New(database))
	defer server.Close()
	file := fmt.Sprintf(`{"version":"%s","tasks":[{"category_id":"%s","name":"Imported parent","subtasks":[{"name":"Imported child","sprints":[{"name":"Imported sprint","estimated_duration_seconds":120}]}]}]}`, executionBatchVersion, category.CategoryID)
	path := writeExecutionBatchFixture(t, "integration-batch.json", file)
	var stdout, stderr strings.Builder
	if code := taskBatch(project.ProjectID, []string{path, "--check-only"}, &stdout, &stderr, server.URL); code != 0 {
		t.Fatalf("end-to-end check-only code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "✓ Smoke test passed") {
		t.Fatalf("real API smoke did not print a pass mark: %q", stdout.String())
	}
	tasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("check-only wrote Tasks=%d err=%v", len(tasks), err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := taskBatch(project.ProjectID, []string{path}, &stdout, &stderr, server.URL); code != 0 {
		t.Fatalf("end-to-end apply code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	tasks, err = database.ListTasks(ctx, project.ProjectID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("successful apply created Tasks=%d err=%v", len(tasks), err)
	}
	subtasks, err := database.ListSubtasks(ctx, tasks[0].TaskID)
	if err != nil || len(subtasks) != 1 {
		t.Fatalf("successful apply created Subtasks=%d err=%v", len(subtasks), err)
	}
	sprints, err := database.ListSubtaskSprints(ctx, subtasks[0].SubtaskID)
	if err != nil || len(sprints) != 1 {
		t.Fatalf("successful apply created Subtask Sprints=%d err=%v", len(sprints), err)
	}
	if !strings.Contains(stdout.String(), "✓ Smoke test passed") || !strings.Contains(stdout.String(), tasks[0].TaskID) || !strings.Contains(stdout.String(), subtasks[0].SubtaskID) || !strings.Contains(stdout.String(), sprints[0].SprintID) {
		t.Fatalf("CLI result omitted pass state or created IDs: %q", stdout.String())
	}
}
