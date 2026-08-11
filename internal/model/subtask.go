// Copyright (c) 2026 Seb. All rights reserved.

package model

import "time"

// Subtask is an executable unit scoped beneath one task.
type Subtask struct {
	SubtaskID                string    `json:"subtask_id"`
	ProjectID                string    `json:"project_id"`
	CategoryID               string    `json:"category_id"`
	TaskID                   string    `json:"task_id"`
	ItemAddress              string    `json:"item_address"`
	Name                     string    `json:"name"`
	Description              string    `json:"description,omitempty"`
	Goal                     string    `json:"goal,omitempty"`
	Status                   string    `json:"status"`
	Priority                 string    `json:"priority"`
	EstimatedDurationSeconds int64     `json:"estimated_duration_seconds"`
	ReportedCompletionPct    float64   `json:"reported_completion_pct"`
	CalculatedCompletionPct  float64   `json:"calculated_completion_pct"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// UpdateSubtaskStatusInput holds an explicit Subtask workflow state.
type UpdateSubtaskStatusInput struct {
	Status string `json:"status"`
}

// CreateSubtaskInput holds client-supplied subtask fields.
type CreateSubtaskInput struct {
	Name                     string `json:"name"`
	Description              string `json:"description"`
	Goal                     string `json:"goal"`
	Priority                 string `json:"priority"`
	EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
}
