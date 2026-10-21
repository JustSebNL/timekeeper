package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestExecutionBatchAPIQueuesBeforeRegistrationAndDeduplicates(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "registration-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Queued API project"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Queue"})
	if err != nil {
		t.Fatal(err)
	}
	input := model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks: []model.ExecutionBatchTask{{
			CategoryID: category.CategoryID,
			Name:       "Stacked API Task",
			Sprints:    []model.CreateSprintInput{{Name: "Queued Sprint", EstimatedDurationSeconds: 60}},
		}},
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	handler := api.New(database)
	path := "/api/v1/projects/" + project.ProjectID + "/execution-batches"
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "batch-offer-001")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("first offer status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	queueID := recorder.Header().Get("X-TimeKeeper-Registration-Queue-ID")
	if queueID == "" {
		t.Fatal("successful API registration did not expose its durable queue ID")
	}
	var created model.ExecutionBatchResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Counts != (model.BatchCounts{Tasks: 1, Sprints: 1, TotalObjects: 2}) {
		t.Fatalf("created result=%+v", created.Counts)
	}

	queue, err := database.ListRegistrationQueue(ctx, project.ProjectID, "", 10)
	if err != nil || len(queue) != 1 {
		t.Fatalf("registration queue=%+v err=%v", queue, err)
	}
	if queue[0].QueueID != queueID || queue[0].Status != model.RegistrationQueueStatusRegistered || queue[0].Kind != model.RegistrationQueueKindExecutionBatch {
		t.Fatalf("queue item=%+v", queue[0])
	}

	request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "batch-offer-001")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("X-TimeKeeper-Registration-Queue-ID") != queueID {
		t.Fatalf("idempotent retry status=%d queue=%q body=%s", recorder.Code, recorder.Header().Get("X-TimeKeeper-Registration-Queue-ID"), recorder.Body.String())
	}
	tasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("idempotent API retry duplicated Tasks: count=%d err=%v", len(tasks), err)
	}

	changed := input
	changed.Tasks = append([]model.ExecutionBatchTask(nil), input.Tasks...)
	changed.Tasks[0].Name = "Changed payload"
	changedBody, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(changedBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "batch-offer-001")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("reused key with changed payload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestTaskSubtaskAndSprintAPIOffersStackInRegistrationAndExecutionQueues(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "api-offer-queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "API offer queue"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.New(database)
	post := func(path, body, key string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", key)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	taskRecorder := post("/api/v1/projects/"+project.ProjectID+"/tasks", `{"category_id":"`+category.CategoryID+`","name":"Queued API Task"}`, "task-offer")
	if taskRecorder.Code != http.StatusCreated {
		t.Fatalf("Task offer status=%d body=%s", taskRecorder.Code, taskRecorder.Body.String())
	}
	var task model.Task
	if err := json.Unmarshal(taskRecorder.Body.Bytes(), &task); err != nil || task.TaskID == "" {
		t.Fatalf("Task offer result=%+v err=%v", task, err)
	}
	if taskRecorder.Header().Get("X-TimeKeeper-Registration-Queue-ID") == "" {
		t.Fatal("Task API response missing registration queue reference")
	}

	subtaskRecorder := post("/api/v1/tasks/"+task.TaskID+"/subtasks", `{"name":"Queued API Subtask"}`, "subtask-offer")
	if subtaskRecorder.Code != http.StatusCreated {
		t.Fatalf("Subtask offer status=%d body=%s", subtaskRecorder.Code, subtaskRecorder.Body.String())
	}
	var subtask model.Subtask
	if err := json.Unmarshal(subtaskRecorder.Body.Bytes(), &subtask); err != nil || subtask.SubtaskID == "" {
		t.Fatalf("Subtask offer result=%+v err=%v", subtask, err)
	}

	taskSprintRecorder := post("/api/v1/tasks/"+task.TaskID+"/sprints", `{"name":"Task Sprint","estimated_duration_seconds":60}`, "task-sprint-offer")
	if taskSprintRecorder.Code != http.StatusCreated {
		t.Fatalf("Task Sprint offer status=%d body=%s", taskSprintRecorder.Code, taskSprintRecorder.Body.String())
	}
	var taskSprint model.Sprint
	if err := json.Unmarshal(taskSprintRecorder.Body.Bytes(), &taskSprint); err != nil || taskSprint.SprintID == "" {
		t.Fatalf("Task Sprint offer result=%+v err=%v", taskSprint, err)
	}

	subtaskSprintRecorder := post("/api/v1/subtasks/"+subtask.SubtaskID+"/sprints", `{"name":"Subtask Sprint","estimated_duration_seconds":60}`, "subtask-sprint-offer")
	if subtaskSprintRecorder.Code != http.StatusCreated {
		t.Fatalf("Subtask Sprint offer status=%d body=%s", subtaskSprintRecorder.Code, subtaskSprintRecorder.Body.String())
	}
	var subtaskSprint model.Sprint
	if err := json.Unmarshal(subtaskSprintRecorder.Body.Bytes(), &subtaskSprint); err != nil || subtaskSprint.SprintID == "" {
		t.Fatalf("Subtask Sprint offer result=%+v err=%v", subtaskSprint, err)
	}

	queueRecorder := httptest.NewRecorder()
	handler.ServeHTTP(queueRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/registration-queue", nil))
	if queueRecorder.Code != http.StatusOK {
		t.Fatalf("registration queue status=%d body=%s", queueRecorder.Code, queueRecorder.Body.String())
	}
	var queueResponse struct {
		Items []model.RegistrationQueueItem `json:"items"`
	}
	if err := json.Unmarshal(queueRecorder.Body.Bytes(), &queueResponse); err != nil {
		t.Fatal(err)
	}
	if len(queueResponse.Items) != 4 {
		t.Fatalf("stacked registration items=%+v", queueResponse.Items)
	}
	for index, kind := range []string{model.RegistrationQueueKindTask, model.RegistrationQueueKindSubtask, model.RegistrationQueueKindSprint, model.RegistrationQueueKindSprint} {
		if queueResponse.Items[index].Kind != kind || queueResponse.Items[index].Status != model.RegistrationQueueStatusRegistered {
			t.Fatalf("queue item[%d]=%+v want kind=%s Registered", index, queueResponse.Items[index], kind)
		}
	}

	claim := httptest.NewRecorder()
	claimRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/sprints/claim-next", bytes.NewBufferString("{}"))
	claimRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(claim, claimRequest)
	if claim.Code != http.StatusOK {
		t.Fatalf("claim first queued Sprint status=%d body=%s", claim.Code, claim.Body.String())
	}
	var claimed model.Sprint
	if err := json.Unmarshal(claim.Body.Bytes(), &claimed); err != nil {
		t.Fatal(err)
	}
	if claimed.SprintID != taskSprint.SprintID {
		t.Fatalf("first claim=%s, want oldest stacked Sprint %s", claimed.SprintID, taskSprint.SprintID)
	}
	claim = httptest.NewRecorder()
	claimRequest = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/sprints/claim-next", bytes.NewBufferString("{}"))
	claimRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(claim, claimRequest)
	if claim.Code != http.StatusOK {
		t.Fatalf("claim second queued Sprint status=%d body=%s", claim.Code, claim.Body.String())
	}
	if err := json.Unmarshal(claim.Body.Bytes(), &claimed); err != nil {
		t.Fatal(err)
	}
	if claimed.SprintID != subtaskSprint.SprintID {
		t.Fatalf("second claim=%s, want next stacked Sprint %s", claimed.SprintID, subtaskSprint.SprintID)
	}
}

func TestRegistrationQueueAPIListsTheAcceptedOffer(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "registration-list-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Queue list project"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Queue"})
	if err != nil {
		t.Fatal(err)
	}
	item, duplicate, err := database.EnqueueRegistration(ctx, project.ProjectID, model.RegistrationQueueKindExecutionBatch, "list-offer", model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks:   []model.ExecutionBatchTask{{CategoryID: category.CategoryID, Name: "Visible queued Task"}},
	})
	if err != nil || duplicate {
		t.Fatalf("enqueue=%+v duplicate=%v err=%v", item, duplicate, err)
	}
	handler := api.New(database)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/registration-queue", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("queue list status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []model.RegistrationQueueItem `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].QueueID != item.QueueID || response.Items[0].Status != model.RegistrationQueueStatusQueued {
		t.Fatalf("queue list items=%+v", response.Items)
	}
}

func TestRegistrationQueueAPIListsFIFOPageMetadata(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "registration-page-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Queue page project"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Page"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := database.EnqueueRegistration(ctx, project.ProjectID, model.RegistrationQueueKindExecutionBatch, "page-1", model.ExecutionBatchInput{Version: "timekeeper-execution-batch/v1", Tasks: []model.ExecutionBatchTask{{CategoryID: category.CategoryID, Name: "First"}}})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := database.EnqueueRegistration(ctx, project.ProjectID, model.RegistrationQueueKindExecutionBatch, "page-2", model.ExecutionBatchInput{Version: "timekeeper-execution-batch/v1", Tasks: []model.ExecutionBatchTask{{CategoryID: category.CategoryID, Name: "Second"}}})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.New(database)
	readPage := func(offset int) struct {
		Items      []model.RegistrationQueueItem `json:"items"`
		HasMore    bool                          `json:"has_more"`
		NextOffset *int                          `json:"next_offset"`
	} {
		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/registration-queue?limit=1&offset=%d", project.ProjectID, offset), nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("queue page offset=%d status=%d body=%s", offset, recorder.Code, recorder.Body.String())
		}
		var response struct {
			Items      []model.RegistrationQueueItem `json:"items"`
			HasMore    bool                          `json:"has_more"`
			NextOffset *int                          `json:"next_offset"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	firstPage := readPage(0)
	if len(firstPage.Items) != 1 || firstPage.Items[0].QueueID != first.QueueID || !firstPage.HasMore || firstPage.NextOffset == nil || *firstPage.NextOffset != 1 {
		t.Fatalf("first API page=%+v", firstPage)
	}
	secondPage := readPage(*firstPage.NextOffset)
	if len(secondPage.Items) != 1 || secondPage.Items[0].QueueID != second.QueueID || secondPage.HasMore || secondPage.NextOffset != nil {
		t.Fatalf("second API page=%+v", secondPage)
	}
}
