// Copyright (c) 2026 Seb. All rights reserved.

package api_test

import (
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

func TestProjectExecutionTreePreservesDirectAndNestedSprintOwnership(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "HSAM"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(context.Background(), project.ProjectID, model.CreateCategoryInput{Name: "Memory"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(context.Background(), project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Build recall", EstimatedDurationSeconds: 1800})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	directSprint, err := database.CreateSprint(context.Background(), task.TaskID, model.CreateSprintInput{Name: "Direct spike", EstimatedDurationSeconds: 300})
	if err != nil {
		t.Fatalf("create direct sprint: %v", err)
	}
	subtask, err := database.CreateSubtask(context.Background(), task.TaskID, model.CreateSubtaskInput{Name: "Implement vector recall", EstimatedDurationSeconds: 900})
	if err != nil {
		t.Fatalf("create subtask: %v", err)
	}
	nestedSprint, err := database.CreateSubtaskSprint(context.Background(), subtask.SubtaskID, model.CreateSprintInput{Name: "Implement retrieval", EstimatedDurationSeconds: 600})
	if err != nil {
		t.Fatalf("create nested sprint: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/execution-tree", nil)
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("execution tree status = %d, body = %s", response.Code, response.Body.String())
	}
	var tree struct {
		Project struct {
			ProjectID string `json:"project_id"`
		} `json:"project"`
		Categories []struct {
			Category struct {
				CategoryID string `json:"category_id"`
			} `json:"category"`
			Tasks []struct {
				Task struct {
					TaskID string `json:"task_id"`
				} `json:"task"`
				Sprints []struct {
					SprintID string `json:"sprint_id"`
				} `json:"sprints"`
				Subtasks []struct {
					Subtask struct {
						SubtaskID string `json:"subtask_id"`
					} `json:"subtask"`
					Sprints []struct {
						SprintID string `json:"sprint_id"`
					} `json:"sprints"`
				} `json:"subtasks"`
			} `json:"tasks"`
		} `json:"categories"`
	}
	if err := json.NewDecoder(response.Body).Decode(&tree); err != nil {
		t.Fatalf("decode execution tree: %v", err)
	}
	if tree.Project.ProjectID != project.ProjectID || len(tree.Categories) != 1 || tree.Categories[0].Category.CategoryID != category.CategoryID || len(tree.Categories[0].Tasks) != 1 {
		t.Fatalf("unexpected tree root: %#v", tree)
	}
	taskNode := tree.Categories[0].Tasks[0]
	if taskNode.Task.TaskID != task.TaskID || len(taskNode.Sprints) != 1 || taskNode.Sprints[0].SprintID != directSprint.SprintID || len(taskNode.Subtasks) != 1 {
		t.Fatalf("unexpected task node: %#v", taskNode)
	}
	subtaskNode := taskNode.Subtasks[0]
	if subtaskNode.Subtask.SubtaskID != subtask.SubtaskID || len(subtaskNode.Sprints) != 1 || subtaskNode.Sprints[0].SprintID != nestedSprint.SprintID {
		t.Fatalf("unexpected subtask node: %#v", subtaskNode)
	}
}
