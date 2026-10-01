// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package model

import "time"

// HandoffSnapshot represents a compact, deterministic operational and context snapshot of a Project for agent context windows and handoffs.
type HandoffSnapshot struct {
	Project            Project             `json:"project"`
	OperationalSummary ProjectOperationalSummary `json:"operational_summary"`
	ActiveSprint       *Sprint             `json:"active_sprint,omitempty"`
	OnHoldSprints      []Sprint            `json:"on_hold_sprints,omitempty"`
	NextRunnableSprint *Sprint             `json:"next_runnable_sprint,omitempty"`
	RecentNotes        []ProjectNote       `json:"recent_notes,omitempty"`
	RecentMessages     []ProjectMessage    `json:"recent_messages,omitempty"`
	RecentEvents       []ProjectEvent      `json:"recent_events,omitempty"`
	AttentionItems     []string            `json:"attention_items,omitempty"`
	Markdown           string              `json:"markdown,omitempty"`
	GeneratedAt        time.Time           `json:"generated_at"`
}
