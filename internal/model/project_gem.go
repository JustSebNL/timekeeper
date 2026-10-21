// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package model

import "time"

const (
	MaxProjectGemNameLength      = 200
	MaxProjectGemCategoryLength  = 120
	MaxProjectGemContentLength   = 20000
	MaxProjectGemTagsLength      = 500
	MaxProjectGemSourceRefLength = 2048
	MaxProjectGemAuthorLength    = 120
	DefaultProjectGemListLimit   = 50
	MaxProjectGemListLimit       = 200
)

type ProjectGem struct {
	GemID       int64      `json:"gem_id"`
	Ref         string     `json:"ref"`
	ProjectID   string     `json:"project_id"`
	ParentGemID *int64     `json:"parent_gem_id,omitempty"`
	Category    string     `json:"category"`
	Subcategory string     `json:"subcategory,omitempty"`
	Name        string     `json:"name"`
	Content     string     `json:"content"`
	Tags        string     `json:"tags,omitempty"`
	SourceRef   string     `json:"source_ref,omitempty"`
	Revision    int        `json:"revision"`
	Pinned      bool       `json:"pinned"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type CreateProjectGemInput struct {
	ParentGemID *int64 `json:"parent_gem_id,omitempty"`
	Category    string `json:"category,omitempty"`
	Subcategory string `json:"subcategory,omitempty"`
	Name        string `json:"name"`
	Content     string `json:"content"`
	Tags        string `json:"tags,omitempty"`
	SourceRef   string `json:"source_ref,omitempty"`
	CreatedBy   string `json:"created_by,omitempty"`
}

type UpdateProjectGemInput struct {
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	Name        string `json:"name"`
	Content     string `json:"content"`
	Tags        string `json:"tags"`
	SourceRef   string `json:"source_ref"`
	EditedBy    string `json:"edited_by"`
}

type ProjectGemFilter struct {
	Query          string
	Category       string
	Subcategory    string
	ParentGemID    *int64
	PinnedOnly     bool
	IncludeDeleted bool
	Limit          int
}

type ProjectGemList struct {
	Items   []ProjectGem `json:"items"`
	Total   int           `json:"total"`
	Limit   int           `json:"limit"`
	HasMore bool          `json:"has_more"`
}

type ProjectGemCategory struct {
	Category      string   `json:"category"`
	Subcategories []string `json:"subcategories"`
}

type ProjectGemRevision struct {
	RevisionID   int64     `json:"revision_id"`
	GemID        int64     `json:"gem_id"`
	ProjectID    string    `json:"project_id"`
	Revision     int       `json:"revision"`
	Category     string    `json:"category"`
	Subcategory  string    `json:"subcategory,omitempty"`
	Name         string    `json:"name"`
	Content      string    `json:"content"`
	Tags         string    `json:"tags,omitempty"`
	SourceRef    string    `json:"source_ref,omitempty"`
	EditedBy     string    `json:"edited_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
