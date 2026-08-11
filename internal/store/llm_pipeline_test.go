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

func TestCreateLLMPipelineRejectsExcessiveSystemPrompt(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	_, err = database.CreateLLMPipeline(context.Background(), model.CreateLLMPipelineInput{Name: "Too large", Provider: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "qwen3:4b", SystemPrompt: strings.Repeat("x", 10001)})
	if err == nil {
		t.Fatal("expected oversized system prompt rejection")
	}
}

func TestCreateLLMPipelinePersistsLocalPlanningConfiguration(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	pipeline, err := database.CreateLLMPipeline(context.Background(), model.CreateLLMPipelineInput{
		Name: "Local planner", Provider: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "qwen3:4b", SystemPrompt: "Return a strict draft.",
	})
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	if pipeline.PipelineID == 0 || pipeline.Provider != "ollama" || pipeline.Model != "qwen3:4b" || pipeline.SystemPrompt != "Return a strict draft." {
		t.Fatalf("pipeline=%#v", pipeline)
	}
}
