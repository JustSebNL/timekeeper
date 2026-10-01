// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package plugin_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/plugin"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestDurableEventCatchUpReconciler(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reconciler.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	p, err := s.CreateProject(ctx, model.CreateProjectInput{Name: "Reconciler Test", Alias: "reconciler-test"})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	cat, err := s.CreateCategory(ctx, p.ProjectID, model.CreateCategoryInput{Name: "Core"})
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	task1, err := s.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{CategoryID: cat.CategoryID, Name: "Task 1", EstimatedDurationSeconds: 3600})
	if err != nil {
		t.Fatalf("failed to create task 1: %v", err)
	}

	task2, err := s.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{CategoryID: cat.CategoryID, Name: "Task 2", EstimatedDurationSeconds: 3600})
	if err != nil {
		t.Fatalf("failed to create task 2: %v", err)
	}

	// Mutate task status to generate events
	if _, err := s.UpdateTaskStatus(ctx, task1.TaskID, model.UpdateTaskStatusInput{Status: "On Hold"}); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
	if _, err := s.UpdateTaskStatus(ctx, task2.TaskID, model.UpdateTaskStatusInput{Status: "Completed"}); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	reconciler := plugin.NewReconciler(s)

	var received []model.ProjectEvent
	handler := func(ev model.ProjectEvent) error {
		received = append(received, ev)
		return nil
	}

	newID, count, err := reconciler.CatchUp(ctx, "obsidian-plugin", p.ProjectID, 0, handler)
	if err != nil {
		t.Fatalf("reconciler catch up failed: %v", err)
	}

	if count == 0 || newID <= 0 {
		t.Fatalf("expected events caught up, got count=%d newID=%d", count, newID)
	}

	if len(received) != count {
		t.Fatalf("mismatch in delivered events: got %d, count %d", len(received), count)
	}

	// Verify offset was persisted in reconciler
	if reconciler.GetOffset("obsidian-plugin") != newID {
		t.Fatalf("offset mismatch: got %d, want %d", reconciler.GetOffset("obsidian-plugin"), newID)
	}

	// Submitting catch up again with latest offset should yield 0 new events
	_, count2, err := reconciler.CatchUp(ctx, "obsidian-plugin", p.ProjectID, newID, handler)
	if err != nil {
		t.Fatalf("second catchup failed: %v", err)
	}
	if count2 != 0 {
		t.Fatalf("expected 0 events on second catchup, got %d", count2)
	}
}

func TestReconcileLoopBackgroundExecution(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reconciler_loop.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	p, err := s.CreateProject(ctx, model.CreateProjectInput{Name: "Loop Test", Alias: "loop-test"})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	cat, err := s.CreateCategory(ctx, p.ProjectID, model.CreateCategoryInput{Name: "Core"})
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	reconciler := plugin.NewReconciler(s)
	var processedCount atomic.Int64

	handler := func(ev model.ProjectEvent) error {
		return nil
	}

	// Start background loop
	go func() {
		_ = reconciler.ReconcileLoop(ctx, "plugin-loop", p.ProjectID, 20*time.Millisecond, handler, &processedCount)
	}()

	time.Sleep(50 * time.Millisecond)

	// Dynamically generate work (UpdateTaskStatus writes a project event)
	task, err := s.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{CategoryID: cat.CategoryID, Name: "Loop Task", EstimatedDurationSeconds: 3600})
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	if _, err := s.UpdateTaskStatus(ctx, task.TaskID, model.UpdateTaskStatusInput{Status: "Completed"}); err != nil {
		t.Fatalf("failed to update task status: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if processedCount.Load() == 0 {
		t.Fatal("reconcile loop did not process any events")
	}
}
