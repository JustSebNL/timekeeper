// Copyright (c) 2026 Seb. All rights reserved.

package model

import "time"

// LLMPipeline is an optional, local-only planning connection. It does not grant
// a model authority to mutate Project data without an explicit apply request.
type LLMPipeline struct {
	PipelineID   int64     `json:"pipeline_id"`
	Name         string    `json:"name"`
	Provider     string    `json:"provider"`
	BaseURL      string    `json:"base_url"`
	Model        string    `json:"model"`
	SystemPrompt string    `json:"system_prompt"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateLLMPipelineInput holds an optional local planning pipeline definition.
type CreateLLMPipelineInput struct {
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	BaseURL      string `json:"base_url"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
}
