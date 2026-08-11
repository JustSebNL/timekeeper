// Copyright (c) 2026 Seb. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestListProjectsProjectsDerivedCompletion(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Completion list"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Complete me", EstimatedDurationSeconds: 600})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpdateTaskStatus(ctx, task.TaskID, model.UpdateTaskStatusInput{Status: "Completed"}); err != nil {
		t.Fatal(err)
	}

	projects, err := database.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].CalculatedCompletionPct != 100 {
		t.Fatalf("projects=%#v, want one Project at 100%%", projects)
	}
}
