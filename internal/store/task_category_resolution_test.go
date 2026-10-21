// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestCreateTaskCategoryResolutionAndFallback(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	p, err := st.CreateProject(ctx, model.CreateProjectInput{
		Name: "Resolution Test Project",
	})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// 1. Create a task with empty CategoryID -> should resolve to default category C-00000 ("General")
	task1, err := st.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{
		Name: "Task with Empty Category",
	})
	if err != nil {
		t.Fatalf("CreateTask empty category failed: %v", err)
	}
	if task1.CategoryID != "C-00000" {
		t.Errorf("expected CategoryID C-00000, got %s", task1.CategoryID)
	}
	expectedAddr := fmt.Sprintf("%s.00000.%s", p.ItemAddress, task1.TaskID[2:])
	if task1.ItemAddress != expectedAddr {
		t.Errorf("expected ItemAddress %s, got %s", expectedAddr, task1.ItemAddress)
	}

	// 2. Create a custom category
	cat, err := st.CreateCategory(ctx, p.ProjectID, model.CreateCategoryInput{
		Name: "Frontend Core",
	})
	if err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}

	// 3. Create a task matching category name by exact/case-insensitive name
	task2, err := st.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{
		Name:       "Task with Category Name",
		CategoryID: "frontend core",
	})
	if err != nil {
		t.Fatalf("CreateTask by category name failed: %v", err)
	}
	if task2.CategoryID != cat.CategoryID {
		t.Errorf("expected CategoryID %s, got %s", cat.CategoryID, task2.CategoryID)
	}

	// 4. An unknown category must be rejected instead of silently moving the task
	// into the default category.
	_, err = st.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{
		Name:       "Task with Nonexistent Category",
		CategoryID: "NonExistentCategoryName",
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected unknown category to be rejected as not found, got %v", err)
	}
}
