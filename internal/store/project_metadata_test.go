// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestUpdateProjectMetadataPersistsGoalAndDescription(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Metadata target", Goal: "Old goal", Description: "Old description"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	updated, err := database.UpdateProjectMetadata(context.Background(), project.ProjectID, model.UpdateProjectMetadataInput{
		Goal: "Ship a useful local tracker", Description: "Edited durable project context",
	})
	if err != nil {
		t.Fatalf("update project metadata: %v", err)
	}
	if updated.ProjectGoal != "Ship a useful local tracker" || updated.ProjectDescription != "Edited durable project context" {
		t.Fatalf("updated=%#v", updated)
	}
	tree, err := database.ProjectExecutionTree(context.Background(), project.ProjectID)
	if err != nil {
		t.Fatalf("read execution tree: %v", err)
	}
	if tree.Project.ProjectGoal != updated.ProjectGoal || tree.Project.ProjectDescription != updated.ProjectDescription {
		t.Fatalf("tree project=%#v", tree.Project)
	}
	if _, err := database.UpdateProjectMetadata(context.Background(), project.ProjectID, model.UpdateProjectMetadataInput{Goal: updated.ProjectGoal, Description: updated.ProjectDescription}); err != nil {
		t.Fatalf("repeat identical metadata update: %v", err)
	}
	events, err := database.ListProjectEvents(context.Background(), project.ProjectID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "project_metadata_updated" {
		t.Fatalf("events=%#v", events)
	}
}
