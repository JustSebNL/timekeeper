// Copyright (c) 2026 Seb. All rights reserved.

package model

import "time"

// ProjectEvent is an immutable, system-recorded change in a project's execution history.
type ProjectEvent struct {
	EventID    int64     `json:"event_id"`
	ProjectID  string    `json:"project_id"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	EventType  string    `json:"event_type"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}
