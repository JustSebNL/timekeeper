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

func TestProjectExecutionTreeServerFiltersAndLimitsBeforeResponse(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "tree-server-filter.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Tree filtering"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	var activeTasks []string
	for _, name := range []string{"First active", "Second active"} {
		task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		activeTasks = append(activeTasks, task.TaskID)
		sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: name + " sprint", EstimatedDurationSeconds: 900})
		if err != nil {
			t.Fatal(err)
		}
		startExecutionTreeFilterSprint(t, api.New(database), sprint.SprintID)
	}
	if _, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "No active Sprint"}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/execution-tree?status=Open&active=true&limit=1", nil)
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var tree model.ExecutionTree
	if err := json.NewDecoder(response.Body).Decode(&tree); err != nil {
		t.Fatal(err)
	}
	if len(tree.Categories) != 1 || len(tree.Categories[0].Tasks) != 1 {
		t.Fatalf("expected a one-Task filtered prefix, got %+v", tree)
	}
	returnedTask := tree.Categories[0].Tasks[0]
	if returnedTask.Task.TaskID != activeTasks[0] && returnedTask.Task.TaskID != activeTasks[1] {
		t.Fatalf("server returned a task outside the active filter: %s", returnedTask.Task.TaskID)
	}
	if len(returnedTask.Sprints) != 1 || returnedTask.Sprints[0].Status != "Active" {
		t.Fatalf("server did not filter to active Sprints: %+v", returnedTask)
	}
	wantHeaders := map[string]string{
		"X-TimeKeeper-Execution-Tree-Total-Tasks": "2",
		"X-TimeKeeper-Execution-Tree-Shown-Tasks": "1",
		"X-TimeKeeper-Execution-Tree-Truncated":   "true",
	}
	for name, want := range wantHeaders {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestProjectExecutionTreeServerTaskFilterPreservesNestedOwnerPath(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "tree-server-subtask-filter.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Nested tree filtering"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Target task"})
	if err != nil {
		t.Fatal(err)
	}
	subtask, err := database.CreateSubtask(ctx, task.TaskID, model.CreateSubtaskInput{Name: "Target subtask"})
	if err != nil {
		t.Fatal(err)
	}
	nestedSprint, err := database.CreateSubtaskSprint(ctx, subtask.SubtaskID, model.CreateSprintInput{Name: "Active nested Sprint", EstimatedDurationSeconds: 900})
	if err != nil {
		t.Fatal(err)
	}
	startExecutionTreeFilterSprint(t, api.New(database), nestedSprint.SprintID)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/execution-tree?task_id="+task.TaskID+"&active=true", nil)
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var tree model.ExecutionTree
	if err := json.NewDecoder(response.Body).Decode(&tree); err != nil {
		t.Fatal(err)
	}
	if len(tree.Categories) != 1 || len(tree.Categories[0].Tasks) != 1 || tree.Categories[0].Tasks[0].Task.TaskID != task.TaskID {
		t.Fatalf("Task owner path missing from filtered tree: %+v", tree)
	}
	node := tree.Categories[0].Tasks[0]
	if len(node.Subtasks) != 1 || node.Subtasks[0].Subtask.SubtaskID != subtask.SubtaskID || len(node.Subtasks[0].Sprints) != 1 || node.Subtasks[0].Sprints[0].SprintID != nestedSprint.SprintID {
		t.Fatalf("nested owner path not preserved: %+v", node)
	}
}

func TestProjectExecutionTreeRejectsInvalidFilterParameters(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "tree-server-filter-validation.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Tree filter validation"})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"depth=0", "limit=201", "active=maybe", "task_id=T-1&task_id=T-2"} {
		t.Run(query, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/execution-tree?"+query, nil)
			response := httptest.NewRecorder()
			api.New(database).ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("query %q status=%d body=%s", query, response.Code, response.Body.String())
			}
		})
	}
}

func startExecutionTreeFilterSprint(t *testing.T, handler http.Handler, sprintID string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sprints/"+sprintID+"/start", bytes.NewBufferString(`{"reason":"tree filter test"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("start Sprint %s status=%d body=%s", sprintID, response.Code, response.Body.String())
	}
}

func TestProjectExecutionTreeServerStatusFilter(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "tree-server-status-filter.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Status tree filtering"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Open task"}); err != nil {
		t.Fatal(err)
	}
	completed, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Completed task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpdateTaskStatus(ctx, completed.TaskID, model.UpdateTaskStatusInput{Status: "Completed"}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/execution-tree?status=Completed", nil)
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var tree model.ExecutionTree
	if err := json.NewDecoder(response.Body).Decode(&tree); err != nil {
		t.Fatal(err)
	}
	if len(tree.Categories) != 1 || len(tree.Categories[0].Tasks) != 1 || tree.Categories[0].Tasks[0].Task.TaskID != completed.TaskID || tree.Categories[0].Tasks[0].Task.Status != "Completed" {
		t.Fatalf("status filter returned incorrect Tasks: %+v", tree)
	}
}

func TestProjectExecutionTreeServerDepthFilter(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "tree-server-depth-filter.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Depth tree filtering"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Direct Sprint", EstimatedDurationSeconds: 900}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateSubtask(ctx, task.TaskID, model.CreateSubtaskInput{Name: "Subtask"}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/execution-tree?depth=2", nil)
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var tree model.ExecutionTree
	if err := json.NewDecoder(response.Body).Decode(&tree); err != nil {
		t.Fatal(err)
	}
	if len(tree.Categories) != 1 || len(tree.Categories[0].Tasks) != 1 || len(tree.Categories[0].Tasks[0].Sprints) != 0 || len(tree.Categories[0].Tasks[0].Subtasks) != 0 {
		t.Fatalf("depth=2 did not stop at Task owners: %+v", tree)
	}
}
