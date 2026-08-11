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
	if err != nil || len(events) != 1 || events[0].EventType != "subtask_status_changed" {
		t.Fatalf("events=%#v err=%v", events, err)
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
