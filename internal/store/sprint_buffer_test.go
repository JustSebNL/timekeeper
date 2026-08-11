// Copyright (c) 2026 Seb. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestCreateSprintRejectsFractionalBufferPercent(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Whole buffer"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Bounded work"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, err = database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "No fractions", EstimatedDurationSeconds: 600, BufferPct: 12.5})
	if err == nil || !strings.Contains(err.Error(), "whole number") {
		t.Fatalf("fractional buffer error = %v", err)
	}
}
