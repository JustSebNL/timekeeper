// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestCreatePlanningDraftPersistsValidatedReviewArtifact(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Draft target"})
	pipeline, _ := database.CreateLLMPipeline(ctx, model.CreateLLMPipelineInput{Name: "Planner", Provider: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "qwen3:4b"})
	raw := `{"version":"timekeeper-planning-draft/v1","summary":"Propose a usable first slice.","categories":[{"name":"Delivery","tasks":[{"name":"Ship","estimated_duration_seconds":3600,"buffer_pct":10,"sprints":[],"subtasks":[]}]}]}`
	draft, err := database.CreatePlanningDraft(ctx, project.ProjectID, pipeline.PipelineID, raw)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if draft.DraftID == 0 || draft.Status != "Review" || draft.Summary != "Propose a usable first slice." {
		t.Fatalf("draft=%#v", draft)
	}
}
