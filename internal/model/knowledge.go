// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

const (
	KnowledgeSourceTrail = "trail"

	KnowledgePromotionPending         = "pending"
	KnowledgePromotionCreated         = "created"
	KnowledgePromotionFailedRetryable = "failed_retryable"
	KnowledgePromotionFailedPermanent = "failed_permanent"
	KnowledgePromotionConflict        = "conflict"
)

// KnowledgeLink records the durable relationship between a project-local
// source and a cross-project Galaxy item. The source itself is never deleted
// when promotion fails.
type KnowledgeLink struct {
	LinkID          int64     `json:"link_id"`
	SourceType      string    `json:"source_type"`
	SourceID        string    `json:"source_id"`
	GalaxyID        string    `json:"galaxy_id,omitempty"`
	PromotionKey    string    `json:"promotion_key"`
	PromotionStatus string    `json:"promotion_status"`
	LastError       string    `json:"last_error,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CreateKnowledgeLinkInput struct {
	SourceType   string `json:"source_type"`
	SourceID     string `json:"source_id"`
	GalaxyID     string `json:"galaxy_id"`
	PromotionKey string `json:"promotion_key"`
	Status       string `json:"promotion_status"`
	LastError    string `json:"last_error"`
}
