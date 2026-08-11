// Copyright (c) 2026 Seb. All rights reserved.

package model

import "time"

// Category is a first-class organizational section within a project.
type Category struct {
	CategoryID       string    `json:"category_id"`
	ProjectID        string    `json:"project_id"`
	ParentCategoryID string    `json:"parent_category_id,omitempty"`
	ItemAddress      string    `json:"item_address"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	Goal             string    `json:"goal,omitempty"`
	Status           string    `json:"status"`
	Priority         string    `json:"priority"`
	ProgressPct      float64   `json:"progress_pct"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// UpdateCategoryMetadataInput holds editable Category context; workflow state is updated separately.
type UpdateCategoryMetadataInput struct {
	Goal        string `json:"goal"`
	Description string `json:"description"`
}

// UpdateCategoryStatusInput holds an explicit Category workflow state.
type UpdateCategoryStatusInput struct {
	Status string `json:"status"`
}

// CreateCategoryInput holds client-supplied category fields.
type CreateCategoryInput struct {
	Name             string `json:"name"`
	ParentCategoryID string `json:"parent_category_id,omitempty"`
	Description      string `json:"description"`
	Goal             string `json:"goal"`
	Priority         string `json:"priority"`
}
