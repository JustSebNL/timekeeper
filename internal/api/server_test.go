// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestCategoryMetadataAPIUpdatesDurably(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Category metadata"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/categories/"+category.CategoryID+"/metadata", bytes.NewBufferString(`{"goal":"Ship safely","description":"Coordinate durable work."}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var updated model.Category
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil || updated.Goal != "Ship safely" || updated.Description != "Coordinate durable work." {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	events, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil || len(events) != 1 || events[0].EventType != "category_metadata_updated" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

func TestTaskMetadataAPIUpdatesDurably(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Task metadata"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	task, _ := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Parent"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+task.TaskID+"/metadata", bytes.NewBufferString(`{"goal":"Ship safely","description":"Verify durable behavior."}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var updated model.Task
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil || updated.Goal != "Ship safely" || updated.Description != "Verify durable behavior." {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	events, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil || len(events) != 1 || events[0].EventType != "task_metadata_updated" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

// Task goal/description are intentionally unbounded: a character ceiling shapes
// what can be described. This test pins that contract so a future cap cannot
// creep back in unnoticed. The ToTb board keeps its own 1000-char cap, which is
// about keeping agent context reads small, not about content storage.
func TestTaskMetadataAPIAcceptsLargeGoalAndDescription(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Task metadata limits"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	task, _ := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Parent"})

	// Comfortably beyond both the old 1000-char goal cap and the old 10000-char
	// description cap. TrimSpace still applies (leading/trailing whitespace is
	// normalized away), so the expected values below are compared trimmed.
	rawGoal := strings.Repeat("goal ", 5000)
	rawDescription := strings.Repeat("detail ", 20000)
	largeGoal := strings.TrimSpace(rawGoal)
	largeDescription := strings.TrimSpace(rawDescription)
	if len(largeGoal) <= 1000 || len(largeDescription) <= 10000 {
		t.Fatalf("fixture too small to prove the caps are gone: goal=%d description=%d", len(largeGoal), len(largeDescription))
	}
	payload, err := json.Marshal(map[string]string{"goal": rawGoal, "description": rawDescription})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+task.TaskID+"/metadata", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var updated model.Task
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Goal != largeGoal {
		t.Fatalf("goal not stored verbatim: got %d chars, want %d", len(updated.Goal), len(largeGoal))
	}
	if updated.Description != largeDescription {
		t.Fatalf("description not stored verbatim: got %d chars, want %d", len(updated.Description), len(largeDescription))
	}
}

// An all-whitespace goal/description is still rejected: the limit is emptiness,
// not length.
func TestTaskMetadataAPIRejectsBlankContent(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Task metadata blank"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	task, _ := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Parent"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+task.TaskID+"/metadata", bytes.NewBufferString(`{"goal":"ok","description":"`+strings.Repeat(" ", 50)+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	// Blank is normalized to empty rather than rejected; the task must still load.
	if response.Code != http.StatusOK && response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCategoryStatusAPIUpdatesDurably(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Category status"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/categories/"+category.CategoryID+"/status", bytes.NewBufferString(`{"status":"On Hold"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var updated model.Category
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil || updated.Status != "On Hold" {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	events, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil || len(events) != 1 || events[0].EventType != "category_status_changed" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

func TestSubtaskStatusAPIUpdatesDurably(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Subtask status"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	task, _ := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Parent"})
	subtask, _ := database.CreateSubtask(ctx, task.TaskID, model.CreateSubtaskInput{Name: "Child"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/subtasks/"+subtask.SubtaskID+"/status", bytes.NewBufferString(`{"status":"Completed"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var updated model.Subtask
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil || updated.Status != "Completed" {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	events, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil || len(events) == 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	hasSubtaskEvent := false
	for _, evt := range events {
		if evt.EventType == "subtask_status_changed" {
			hasSubtaskEvent = true
			break
		}
	}
	if !hasSubtaskEvent {
		t.Fatalf("expected subtask_status_changed event in %#v", events)
	}
}

func TestMutationJSONBoundaryRejectsTrailingValueBeforeHandler(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(`{"name":"One object only"}{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	projects, err := database.ListProjects(context.Background())
	if err != nil || len(projects) != 0 {
		t.Fatalf("projects=%#v err=%v", projects, err)
	}
}

func TestSprintTimeExtensionAPIRecordsJustifiedAdditionalTime(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Extension API"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	task, _ := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Work"})
	sprint, _ := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Sprint", EstimatedDurationSeconds: 600})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/extensions", bytes.NewBufferString(`{"duration_seconds":300,"reason":"Additional compatibility path."}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"duration_seconds":300`) {
		t.Fatalf("extension=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSprintTimeExtensionAPIRejectsOverBudgetExtensionWithValidation(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Extension Validation API"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	task, _ := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Work"})
	sprint, _ := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Sprint", EstimatedDurationSeconds: 600})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/extensions", bytes.NewBufferString(`{"duration_seconds":601,"reason":"Over original estimate."}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("over-budget status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"validation_error"`) || !strings.Contains(response.Body.String(), "original sprint estimate") {
		t.Fatalf("over-budget body=%s", response.Body.String())
	}
}

func TestSprintTimeExtensionAPIRejectsTrailingJSON(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Extension API"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	task, _ := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Work"})
	sprint, _ := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Sprint", EstimatedDurationSeconds: 600})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprint.SprintID+"/extensions", bytes.NewBufferString(`{"duration_seconds":300,"reason":"Valid first object"}{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status=%d body=%s", response.Code, response.Body.String())
	}
	items, err := database.ListSprintTimeExtensions(ctx, sprint.SprintID)
	if err != nil || len(items) != 0 {
		t.Fatalf("extensions=%#v err=%v", items, err)
	}
}

func TestApplyPlanningDraftAPIRequiresExplicitRequest(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Apply API"})
	pipeline, _ := database.CreateLLMPipeline(ctx, model.CreateLLMPipelineInput{Name: "Planner", Provider: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "qwen3:4b"})
	draft, _ := database.CreatePlanningDraft(ctx, project.ProjectID, pipeline.PipelineID, `{"version":"timekeeper-planning-draft/v1","summary":"Apply.","categories":[{"name":"Delivery","tasks":[{"name":"Ship","estimated_duration_seconds":1,"buffer_pct":0,"sprints":[],"subtasks":[]}]}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/planning-drafts/"+strconv.FormatInt(draft.DraftID, 10)+"/apply", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"category_id"`) {
		t.Fatalf("apply=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGeneratePlanningDraftCallsConfiguredLocalPipeline(t *testing.T) {
	ctx := context.Background()
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("LLM path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"message":{"content":"{\"version\":\"timekeeper-planning-draft/v1\",\"summary\":\"Generated plan.\",\"categories\":[{\"name\":\"Delivery\",\"tasks\":[{\"name\":\"Ship\",\"estimated_duration_seconds\":1,\"buffer_pct\":0,\"sprints\":[],\"subtasks\":[]}]}]}"}}`))
	}))
	t.Cleanup(llm.Close)
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Generated"})
	pipeline, _ := database.CreateLLMPipeline(ctx, model.CreateLLMPipelineInput{Name: "Planner", Provider: "ollama", BaseURL: llm.URL, Model: "qwen3:4b"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/planning-drafts/generate", bytes.NewBufferString(`{"pipeline_id":`+strconv.FormatInt(pipeline.PipelineID, 10)+`}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"summary":"Generated plan."`) {
		t.Fatalf("generate=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPlanningDraftsAPIStoresReviewArtifact(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Draft target"})
	pipeline, _ := database.CreateLLMPipeline(ctx, model.CreateLLMPipelineInput{Name: "Planner", Provider: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "qwen3:4b"})
	body := `{"pipeline_id":` + strconv.FormatInt(pipeline.PipelineID, 10) + `,"raw_json":"{\"version\":\"timekeeper-planning-draft/v1\",\"summary\":\"Plan.\",\"categories\":[{\"name\":\"Delivery\",\"tasks\":[{\"name\":\"Ship\",\"estimated_duration_seconds\":1,\"buffer_pct\":0,\"sprints\":[],\"subtasks\":[]}]}]}"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/planning-drafts", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create=%d body=%s", response.Code, response.Body.String())
	}
	list := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/planning-drafts", nil)
	listed := httptest.NewRecorder()
	api.New(database).ServeHTTP(listed, list)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"status":"Review"`) {
		t.Fatalf("list=%d body=%s", listed.Code, listed.Body.String())
	}
}

func TestLLMPipelinesAPIConfiguresLocalPlanner(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	request := httptest.NewRequest(http.MethodPost, "/api/v1/llm-pipelines", bytes.NewBufferString(`{"name":"Planner","provider":"ollama","base_url":"http://127.0.0.1:11434","model":"qwen3:4b"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create pipeline=%d body=%s", response.Code, response.Body.String())
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/llm-pipelines", nil)
	listResponse := httptest.NewRecorder()
	api.New(database).ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"model":"qwen3:4b"`) {
		t.Fatalf("list=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
}

func TestTaskStatusTransitionRecordsProjectHistory(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Task status"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Ship"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+task.TaskID+"/status", bytes.NewBufferString(`{"status":"Completed"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status update = %d, body=%s", response.Code, response.Body.String())
	}
	events, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil || len(events) != 1 || events[0].EntityID != task.TaskID || events[0].EventType != "task_status_changed" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

func TestProjectMetadataUpdateIsDurableThroughAPI(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Metadata API"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/metadata", bytes.NewBufferString(`{"goal":"Durable goal","description":"Durable description"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"project_goal":"Durable goal"`) {
		t.Fatalf("metadata=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProjectScheduleAndPhaseAPIsRoundTrip(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Lifecycle API"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+project.ProjectID+"/schedule", bytes.NewBufferString(`{"planned_start_date":"2026-10-11","target_end_date":"2030-10-10","schedule_timezone":"America/Vancouver","deadline_warning_days":14}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("schedule update=%d body=%s", response.Code, response.Body.String())
	}
	var scheduled model.Project
	if err := json.Unmarshal(response.Body.Bytes(), &scheduled); err != nil {
		t.Fatal(err)
	}
	if scheduled.PlannedStartDate != "2026-10-11" || scheduled.TargetEndDate != "2030-10-10" || scheduled.LifecyclePhase != model.ProjectPhaseIntake || scheduled.DeadlineWarningDays != 14 || scheduled.ScheduleTimezone != "America/Vancouver" || scheduled.DeadlineState != "on_track" {
		t.Fatalf("schedule response=%+v", scheduled)
	}

	postPhase := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/phase", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		api.New(database).ServeHTTP(resp, req)
		return resp
	}
	invalid := postPhase(`{"phase":"execution","reason":"skip planning"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid phase transition=%d body=%s", invalid.Code, invalid.Body.String())
	}
	planning := postPhase(`{"phase":"planning","reason":"Schedule agreed"}`)
	if planning.Code != http.StatusOK {
		t.Fatalf("planning transition=%d body=%s", planning.Code, planning.Body.String())
	}
	execution := postPhase(`{"phase":"execution","reason":"Start delivery"}`)
	if execution.Code != http.StatusOK || !strings.Contains(execution.Body.String(), `"actual_started_at":"`) || !strings.Contains(execution.Body.String(), `"lifecycle_phase":"execution"`) {
		t.Fatalf("execution transition=%d body=%s", execution.Code, execution.Body.String())
	}
}

func TestProjectMilestoneAPIsPersistAndEnforceOwnership(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Milestone API"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Build"})
	if err != nil {
		t.Fatal(err)
	}
	otherProject, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Other milestone owner"})
	if err != nil {
		t.Fatal(err)
	}
	otherCategory, err := database.CreateCategory(ctx, otherProject.ProjectID, model.CreateCategoryInput{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err := database.CreateTask(ctx, otherProject.ProjectID, model.CreateTaskInput{CategoryID: otherCategory.CategoryID, Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/milestones", bytes.NewBufferString(`{"name":"Acceptance","description":"Verify output","target_date":"2030-10-10","linked_task_id":"`+task.TaskID+`"}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	api.New(database).ServeHTTP(created, create)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"deadline_state":"on_track"`) {
		t.Fatalf("create milestone=%d body=%s", created.Code, created.Body.String())
	}
	var milestone model.ProjectMilestone
	if err := json.Unmarshal(created.Body.Bytes(), &milestone); err != nil {
		t.Fatal(err)
	}
	list := httptest.NewRecorder()
	api.New(database).ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/milestones", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), milestone.MilestoneID) {
		t.Fatalf("list milestones=%d body=%s", list.Code, list.Body.String())
	}
	status := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/milestones/"+milestone.MilestoneID+"/status", bytes.NewBufferString(`{"status":"Completed"}`))
	status.Header.Set("Content-Type", "application/json")
	completed := httptest.NewRecorder()
	api.New(database).ServeHTTP(completed, status)
	if completed.Code != http.StatusOK || !strings.Contains(completed.Body.String(), `"status":"Completed"`) || !strings.Contains(completed.Body.String(), `"completed_at":"`) {
		t.Fatalf("complete milestone=%d body=%s", completed.Code, completed.Body.String())
	}
	foreign := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/milestones", bytes.NewBufferString(`{"name":"Wrong owner","target_date":"2030-10-10","linked_task_id":"`+otherTask.TaskID+`"}`))
	foreign.Header.Set("Content-Type", "application/json")
	foreignResponse := httptest.NewRecorder()
	api.New(database).ServeHTTP(foreignResponse, foreign)
	if foreignResponse.Code != http.StatusBadRequest {
		t.Fatalf("cross-Project link=%d body=%s", foreignResponse.Code, foreignResponse.Body.String())
	}
}

func TestProjectExportIncludesScheduleAndMilestones(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Export lifecycle", PlannedStartDate: "2026-10-11", TargetEndDate: "2030-10-10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProjectMilestone(ctx, project.ProjectID, model.CreateProjectMilestoneInput{Name: "Launch", TargetDate: "2030-10-01"}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/export", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("export=%d body=%s", response.Code, response.Body.String())
	}
	var snapshot struct {
		Tree struct {
			Project model.Project `json:"project"`
		} `json:"execution_tree"`
		Milestones []model.ProjectMilestone `json:"milestones"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Tree.Project.PlannedStartDate != "2026-10-11" || snapshot.Tree.Project.TargetEndDate != "2030-10-10" || len(snapshot.Milestones) != 1 || snapshot.Milestones[0].Name != "Launch" {
		t.Fatalf("export snapshot=%+v", snapshot)
	}
}

func TestProjectDeadlineWatchAPIIncludesProjectAndMilestoneAlerts(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Now().UTC()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Deadline watch", TargetEndDate: now.AddDate(0, 0, -1).Format("2006-01-02")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProjectMilestone(ctx, project.ProjectID, model.CreateProjectMilestoneInput{Name: "Due soon milestone", TargetDate: now.AddDate(0, 0, 1).Format("2006-01-02")}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/project-deadlines", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("deadline watch=%d body=%s", response.Code, response.Body.String())
	}
	var watch model.ProjectDeadlineWatch
	if err := json.Unmarshal(response.Body.Bytes(), &watch); err != nil {
		t.Fatal(err)
	}
	if len(watch.Items) != 2 || watch.Items[0].DeadlineState != "overdue" || watch.Items[1].DeadlineState != "due_soon" || watch.Items[1].EntityType != "milestone" {
		t.Fatalf("deadline watch=%+v", watch)
	}
}

func TestProjectStatusTransitionIsDurable(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Status management"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/status", bytes.NewBufferString(`{"status":"On Hold"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status update = %d, body = %s", response.Code, response.Body.String())
	}
	events, err := database.ListProjectEvents(context.Background(), project.ProjectID)
	if err != nil || len(events) != 1 || events[0].EventType != "project_status_changed" || events[0].Message != "Project status changed from Open to On Hold." {
		t.Fatalf("project status events = %#v, err = %v", events, err)
	}
}

func TestMutatingAPIRejectsMissingJSONContentType(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(`{"name":"Missing type"}`))
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestMutatingAPIRejectsNonJSONContentType(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(`{"name":"Wrong type"}`))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestProjectCreatedThroughAPIIsListedAfterDatabaseReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "timekeeper.db")

	firstStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	handler := api.New(firstStore)

	body := bytes.NewBufferString(`{"name":"HSAM","description":"Agent memory system","goal":"Build durable agent memory"}`)
	create := httptest.NewRequest(http.MethodPost, "/api/v1/projects", body)
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}

	var project struct {
		ProjectID   string `json:"project_id"`
		ItemAddress string `json:"item_address"`
		ProjectName string `json:"project_name"`
	}
	if err := json.NewDecoder(created.Body).Decode(&project); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if project.ProjectID != "P-10000" || project.ItemAddress != "10000" || project.ProjectName != "HSAM" {
		t.Fatalf("unexpected project: %#v", project)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	secondStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = secondStore.Close() })
	handler = api.New(secondStore)

	list := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listed.Code, listed.Body.String())
	}

	var response struct {
		Items []struct {
			ProjectID   string `json:"project_id"`
			ProjectName string `json:"project_name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listed.Body).Decode(&response); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(response.Items) != 1 || response.Items[0].ProjectID != "P-10000" || response.Items[0].ProjectName != "HSAM" {
		t.Fatalf("unexpected persisted project list: %#v", response.Items)
	}
}

func TestAPIHelpListsIdentityAdministrationAndMessageRoutes(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	request := httptest.NewRequest(http.MethodGet, "/api/help", nil)
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("help status=%d body=%s", response.Code, response.Body.String())
	}
	for _, route := range []string{
		"/api/v1/version",
		"/api/v1/agent-identities/resolve",
		"/api/v1/admin/me",
		"/api/v1/admin/groups/{groupID}/members",
		"/api/v1/projects/{projectID}/messages/checkpoint",
		"/api/v1/projects/{projectID}/messages/{messageID}/context",
		"/api/v1/project-deadlines",
		"/api/v1/search",
		"/api/v1/projects/{projectID}/schedule",
		"/api/v1/projects/{projectID}/phase",
		"/api/v1/projects/{projectID}/milestones/{milestoneID}/status",
		"/api/v1/projects/{projectID}/file-root",
		"/api/v1/projects/{projectID}/files/scan",
		"/api/v1/plugins/{pluginID}/artifact",
	} {
		if !strings.Contains(response.Body.String(), route) {
			t.Errorf("help route list omitted %q", route)
		}
	}
}
