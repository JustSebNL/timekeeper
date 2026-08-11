// Copyright (c) 2026 Seb. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestApplyPlanningDraftMaterializesHierarchyOnlyOnce(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Apply"})
	pipeline, _ := database.CreateLLMPipeline(ctx, model.CreateLLMPipelineInput{Name: "Planner", Provider: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "qwen3:4b"})
	raw := `{"version":"timekeeper-planning-draft/v1","summary":"Materialize.","categories":[{"name":"Delivery","tasks":[{"name":"Ship","estimated_duration_seconds":3600,"buffer_pct":10,"sprints":[{"name":"Slice","estimated_duration_seconds":1800,"buffer_pct":10}],"subtasks":[{"name":"Routes","estimated_duration_seconds":900,"sprints":[{"name":"Route slice","estimated_duration_seconds":900,"buffer_pct":0}]}]}]}]}`
	draft, _ := database.CreatePlanningDraft(ctx, project.ProjectID, pipeline.PipelineID, raw)
	tree, err := database.ApplyPlanningDraft(ctx, project.ProjectID, draft.DraftID)
	if err != nil {
		t.Fatalf("apply=%v", err)
	}
	if len(tree.Categories) != 1 || len(tree.Categories[0].Tasks) != 1 || len(tree.Categories[0].Tasks[0].Sprints) != 1 || len(tree.Categories[0].Tasks[0].Subtasks) != 1 {
		t.Fatalf("tree=%#v", tree)
	}
	if _, err := database.ApplyPlanningDraft(ctx, project.ProjectID, draft.DraftID); err == nil {
		t.Fatal("applied draft must be single use")
	}
}
