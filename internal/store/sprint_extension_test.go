// Copyright (c) 2026 Seb. All rights reserved.

package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestListSprintTimeExtensionsRejectsUnknownSprint(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	_, err = database.ListSprintTimeExtensions(context.Background(), "SP-404")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error=%v", err)
	}
}

func TestAddSprintTimeExtensionPreservesOriginalEstimateAndReason(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Extension evidence"})
	category, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Delivery"})
	task, _ := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Bounded work"})
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Schema work", EstimatedDurationSeconds: 600})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}
	extension, err := database.AddSprintTimeExtension(ctx, sprint.SprintID, model.CreateSprintTimeExtensionInput{DurationSeconds: 300, Reason: "Migration uncovered an additional compatibility case."})
	if err != nil {
		t.Fatalf("add extension: %v", err)
	}
	if extension.DurationSeconds != 300 || extension.Reason != "Migration uncovered an additional compatibility case." {
		t.Fatalf("extension=%#v", extension)
	}
	stored, err := database.ListSprintTimeExtensions(ctx, sprint.SprintID)
	if err != nil || len(stored) != 1 || stored[0].ExtensionID != extension.ExtensionID {
		t.Fatalf("extensions=%#v err=%v", stored, err)
	}
	summary, err := database.ProjectOperationalSummary(ctx, project.ProjectID)
	if err != nil || summary.EstimatedDurationSeconds != 600 || summary.ExtensionDurationSeconds != 300 || summary.PlannedDurationSeconds != 900 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
}
