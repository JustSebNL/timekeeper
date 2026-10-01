// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestCreateTasksBatchStore(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Batch Store Test"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category1, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Backend"})
	if err != nil {
		t.Fatalf("create category 1: %v", err)
	}
	category2, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Frontend"})
	if err != nil {
		t.Fatalf("create category 2: %v", err)
	}

	inputs := []model.CreateTaskInput{
		{CategoryID: category1.CategoryID, Name: "Task 1", EstimatedDurationSeconds: 600},
		{CategoryID: category1.CategoryID, Name: "Task 2", EstimatedDurationSeconds: 1200},
		{CategoryID: category2.CategoryID, Name: "Task 3", EstimatedDurationSeconds: 1800},
	}

	tasks, err := database.CreateTasksBatch(ctx, project.ProjectID, inputs)
	if err != nil {
		t.Fatalf("CreateTasksBatch: %v", err)
	}

	if len(tasks) != 3 {
		t.Fatalf("expected 3 created tasks, got %d", len(tasks))
	}
	if tasks[0].Name != "Task 1" || tasks[1].Name != "Task 2" || tasks[2].Name != "Task 3" {
		t.Fatalf("unexpected task names: %#v", tasks)
	}

	allTasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(allTasks) != 3 {
		t.Fatalf("expected ListTasks to return 3 items, got %d", len(allTasks))
	}
}
