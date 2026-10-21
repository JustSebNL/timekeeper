// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

// Task is a substantial outcome within a project category.
type Task struct {
	TaskID                   string    `json:"task_id"`
	ProjectID                string    `json:"project_id"`
	CategoryID               string    `json:"category_id"`
	ItemAddress              string    `json:"item_address"`
	Name                     string    `json:"name"`
	Description              string    `json:"description,omitempty"`
	Goal                     string    `json:"goal,omitempty"`
	Status                   string    `json:"status"`
	Priority                 string    `json:"priority"`
	EstimatedDurationSeconds int64     `json:"estimated_duration_seconds"`
	ReportedCompletionPct    float64   `json:"reported_completion_pct"`
	CalculatedCompletionPct  float64   `json:"calculated_completion_pct"`
	AutoClose                bool      `json:"auto_close"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// CreateTaskInput holds client-supplied task fields.
type CreateTaskInput struct {
	CategoryID               string `json:"category_id"`
	Name                     string `json:"name"`
	Description              string `json:"description"`
	Goal                     string `json:"goal"`
	Priority                 string `json:"priority"`
	EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
	AutoClose                *bool  `json:"auto_close,omitempty"`
}

// CreateTasksBatchInput holds a list of client-supplied task inputs for bulk ingestion.
type CreateTasksBatchInput struct {
	Tasks []CreateTaskInput `json:"tasks"`
}

// UpdateTaskMetadataInput holds editable Task context; workflow state is updated separately.
type UpdateTaskMetadataInput struct {
	Goal        string `json:"goal"`
	Description string `json:"description"`
}

// PatchTaskMetadataInput holds a partial Task context update. A nil field is
// "leave unchanged", which is the only way to edit one long field without
// first re-reading and resubmitting the other.
type PatchTaskMetadataInput struct {
	Goal        *string `json:"goal,omitempty"`
	Description *string `json:"description,omitempty"`
}

// UpdateTaskStatusInput holds an explicit Task workflow state.
type UpdateTaskStatusInput struct {
	Status string `json:"status"`
}
