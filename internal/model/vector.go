// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package model

// VectorSearchResult represents a ranked match from semantic vector search.
type VectorSearchResult struct {
	EntityID   string  `json:"entity_id"`
	EntityType string  `json:"entity_type"`
	Distance   float64 `json:"distance"`
	Score      float64 `json:"score"`
	Content    string  `json:"content,omitempty"`
}
