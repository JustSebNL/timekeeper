// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

// Sprint is a bounded, timed execution cut within a task.
type Sprint struct {
	SprintID                 string  `json:"sprint_id"`
	ProjectID                string  `json:"project_id"`
	CategoryID               string  `json:"category_id"`
	TaskID                   string  `json:"task_id"`
	SubtaskID                string  `json:"subtask_id,omitempty"`
	ItemAddress              string  `json:"item_address"`
	Name                     string  `json:"name"`
	Description              string  `json:"description,omitempty"`
	Goal                     string  `json:"goal,omitempty"`
	Status                   string  `json:"status"`
	Priority                 string  `json:"priority"`
	EstimatedDurationSeconds int64   `json:"estimated_duration_seconds"`
	BufferPct                float64 `json:"buffer_pct"`
	BufferDurationSeconds    int64   `json:"buffer_duration_seconds"`
	ActiveDurationSeconds    int64   `json:"active_duration_seconds"`
	HoldDurationSeconds      int64   `json:"hold_duration_seconds"`
	// HoldReason records why a Sprint is On Hold. It intentionally remains broad:
	// run out of road and other items must catch up first, a user decision,
	// just waiting for input, or any other real blocker. No controlled vocabulary.
	HoldReason string     `json:"hold_reason,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateSprintInput holds client-supplied sprint fields.
type CreateSprintInput struct {
	Name                     string  `json:"name"`
	Description              string  `json:"description"`
	Goal                     string  `json:"goal"`
	Priority                 string  `json:"priority"`
	EstimatedDurationSeconds int64   `json:"estimated_duration_seconds"`
	BufferPct                float64 `json:"buffer_pct"`
}

// CreateSprintTimeExtensionInput records justified additional planned work without changing the original estimate.
type CreateSprintTimeExtensionInput struct {
	DurationSeconds int64  `json:"duration_seconds"`
	Reason          string `json:"reason"`
}

// SprintTimeExtension is an immutable justified addition to a Sprint plan.
type SprintTimeExtension struct {
	ExtensionID     int64     `json:"extension_id"`
	SprintID        string    `json:"sprint_id"`
	DurationSeconds int64     `json:"duration_seconds"`
	Reason          string    `json:"reason"`
	CreatedAt       time.Time `json:"created_at"`
}

// TimeEntry is an immutable completed interval associated with a sprint.
type TimeEntry struct {
	TimeEntryID     int64     `json:"time_entry_id"`
	SprintID        string    `json:"sprint_id"`
	EntryType       string    `json:"entry_type"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds int64     `json:"duration_seconds"`
	Reason          string    `json:"reason,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
