// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package knowledge

import (
	"context"
	"time"
)

const (
	ProviderNameObsidian = "obsidian"
	ProtocolVersion      = "timekeeper-knowledge/v1"
	DefaultCallTimeout   = 5 * time.Second
	MaxQueryLength       = 512
	MaxResultLimit       = 50
	MaxCandidateTitle    = 240
	MaxCandidateContent  = 10000
	MaxPromotionKey      = 512
)

type ItemType string

const (
	ItemTypeKnowledge ItemType = "knowledge"
	ItemTypeDecision  ItemType = "decision"
	ItemTypePattern   ItemType = "pattern"
	ItemTypeFailure   ItemType = "failure"
	ItemTypeInsight   ItemType = "insight"
	ItemTypeProcedure ItemType = "procedure"
	ItemTypeReference ItemType = "reference"
	ItemTypeRAGSource ItemType = "rag-source"
	ItemTypeSkill     ItemType = "skill"
	ItemTypeHarness   ItemType = "harness"
	ItemTypeResearch  ItemType = "research"
)

type ReviewStatus string

const (
	ReviewStatusInbox    ReviewStatus = "inbox"
	ReviewStatusDraft    ReviewStatus = "draft"
	ReviewStatusVerified ReviewStatus = "verified"
	ReviewStatusArchived ReviewStatus = "archived"
	ReviewStatusRejected ReviewStatus = "rejected"
)

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityGalaxy  Visibility = "galaxy"
)

type ProviderStatus struct {
	Available       bool      `json:"available"`
	Provider        string    `json:"provider"`
	ProtocolVersion string    `json:"protocol_version"`
	VaultID         string    `json:"vault_id,omitempty"`
	Capabilities    []string  `json:"capabilities,omitempty"`
	LastCheckedAt   time.Time `json:"last_checked_at"`
	Message         string    `json:"message,omitempty"`
}

type KnowledgeQuery struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type KnowledgeResult struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Type         ItemType     `json:"type"`
	Summary      string       `json:"summary,omitempty"`
	Topics       []string     `json:"topics,omitempty"`
	Tags         []string     `json:"tags,omitempty"`
	ReviewStatus ReviewStatus `json:"review_status"`
	Visibility   Visibility   `json:"visibility"`
}

type KnowledgeItem struct {
	KnowledgeResult
	Content     string    `json:"content"`
	ContentHash string    `json:"content_hash,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type KnowledgeCandidate struct {
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	Type         ItemType   `json:"type"`
	Visibility   Visibility `json:"visibility"`
	PromotionKey string     `json:"promotion_key"`
	Origin       Origin     `json:"origin"`
}

type Origin struct {
	System    string `json:"system,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	TrailID   string `json:"trail_id,omitempty"`
}

type KnowledgeProvider interface {
	Status(context.Context) (ProviderStatus, error)
	Search(context.Context, KnowledgeQuery) ([]KnowledgeResult, error)
	Get(context.Context, string) (KnowledgeItem, error)
	Create(context.Context, KnowledgeCandidate, string) (KnowledgeItem, error)
}
