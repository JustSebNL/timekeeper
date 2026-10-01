// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestGetHandoffSnapshot(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{
		Name:  "Handoff Test Project",
		Alias: "handoff-alias",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Core"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{
		CategoryID: category.CategoryID,
		Name:       "Handoff Task",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{
		Name:                    "Handoff Sprint",
		EstimatedDurationSeconds: 1800,
	})
	if err != nil {
		t.Fatalf("create sprint: %v", err)
	}

	transSp, err := database.TransitionSprint(ctx, sprint.SprintID, "start", "")
	if err != nil {
		t.Fatalf("start sprint: %v", err)
	}
	if transSp.Status != "Active" {
		t.Fatalf("expected transitioned sprint status Active, got %s", transSp.Status)
	}

	note, err := database.CreateProjectNote(ctx, project.ProjectID, model.CreateProjectNoteInput{
		Content: "Checkpoint: completed phase 1",
	}, "agent-1")
	if err != nil {
		t.Fatalf("record note: %v", err)
	}
	_ = note

	snapshot, err := database.GetHandoffSnapshot(ctx, "handoff-alias")
	if err != nil {
		t.Fatalf("GetHandoffSnapshot failed: %v", err)
	}

	if snapshot.Project.ProjectID != project.ProjectID {
		t.Errorf("expected project ID %s, got %s", project.ProjectID, snapshot.Project.ProjectID)
	}
	if snapshot.ActiveSprint == nil || snapshot.ActiveSprint.SprintID != sprint.SprintID {
		t.Errorf("expected active sprint %s, got %#v", sprint.SprintID, snapshot.ActiveSprint)
	}
	if len(snapshot.RecentNotes) == 0 {
		t.Errorf("expected non-empty recent notes")
	}
	if snapshot.Markdown == "" {
		t.Errorf("expected non-empty markdown text")
	}
}
