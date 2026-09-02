// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

// UsageSnapshotInput is a cumulative usage reading for one agent turn.
// Repeating the same session/turn is idempotent; a new turn must increase
// monotonically so cumulative counters can be converted into a delta.
type UsageSnapshotInput struct {
	SessionID           string    `json:"session_id"`
	AgentID             string    `json:"agent_id"`
	Model               string    `json:"model"`
	SprintID            string    `json:"sprint_id"`
	TurnSeq             int64     `json:"turn_seq"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	ContextUsed         *int64    `json:"context_used,omitempty"`
	ContextSize         *int64    `json:"context_size,omitempty"`
	Messages            int64     `json:"messages"`
	CapturedAt          time.Time `json:"captured_at,omitempty"`
}

// UsageDelta is the work represented by a newly accepted cumulative snapshot.
type UsageDelta struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	Messages            int64 `json:"messages"`
}

// UsageSession is the latest authoritative cumulative state for one agent
// session inside one Project. Token counters are intentionally separate from
// context-window occupancy; the latter is not a token usage split.
type UsageSession struct {
	SessionID           string    `json:"session_id"`
	ProjectID           string    `json:"project_id"`
	SprintID            string    `json:"sprint_id,omitempty"`
	AgentID             string    `json:"agent_id"`
	Model               string    `json:"model,omitempty"`
	TurnSeq             int64     `json:"turn_seq"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	ContextUsed         *int64    `json:"context_used,omitempty"`
	ContextSize         *int64    `json:"context_size,omitempty"`
	Messages            int64     `json:"messages"`
	LastActivityAt      time.Time `json:"last_activity_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// UsageSnapshotResult reports the accepted cumulative state and the newly
// observed delta. Duplicate snapshots return zero Delta and Duplicate=true.
type UsageSnapshotResult struct {
	Session   UsageSession `json:"session"`
	Delta     UsageDelta   `json:"delta"`
	Duplicate bool         `json:"duplicate"`
}

// UsageTotals aggregates the latest cumulative state across a Project's agent
// sessions. Cost is deliberately absent until a trusted local price source is
// defined; unknown pricing must never be guessed.
type UsageTotals struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	Messages            int64 `json:"messages"`
	SessionCount        int64 `json:"session_count"`
}

// UsageDay aggregates newly observed token deltas by the local calendar day.
type UsageDay struct {
	Date                string `json:"date"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
	Messages            int64  `json:"messages"`
	SessionCount        int64  `json:"session_count"`
}

// ProjectUsageSummary is the read-only dashboard/API projection.
type ProjectUsageSummary struct {
	ProjectID string         `json:"project_id"`
	Totals    UsageTotals    `json:"totals"`
	Sessions  []UsageSession `json:"sessions"`
	Days      []UsageDay     `json:"days"`
}
