// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

// Project is the top-level unit of tracked work.
type Project struct {
	ProjectID               string    `json:"project_id"`
	ProjectNumber           int64     `json:"project_number"`
	ItemAddress             string    `json:"item_address"`
	ProjectName             string    `json:"project_name"`
	ProjectDescription      string    `json:"project_description,omitempty"`
	ProjectGoal             string    `json:"project_goal,omitempty"`
	Status                  string    `json:"status"`
	Priority                string    `json:"priority"`
	PaletteID               int       `json:"palette_id"`
	ReportedCompletionPct   float64   `json:"reported_completion_pct"`
	CalculatedCompletionPct float64   `json:"calculated_completion_pct"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	ProjectAlias            string    `json:"project_alias,omitempty"`
}

// CreateProjectInput holds client-supplied top-level project fields.
type CreateProjectInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Goal        string `json:"goal"`
	Priority    string `json:"priority"`
	PaletteID   int    `json:"palette_id"`
	Alias       string `json:"alias"`
}

// UpdateProjectMetadataInput holds editable durable top-level Project context.
type UpdateProjectMetadataInput struct {
	Goal        string `json:"goal"`
	Description string `json:"description"`
}

// UpdateProjectStatusInput holds an explicit top-level project workflow state.
type UpdateProjectStatusInput struct {
	Status string `json:"status"`
}

// UpdateProjectAliasInput holds an optional human-readable alias for CLI resolution.
type UpdateProjectAliasInput struct {
	Alias string `json:"alias"`
}
