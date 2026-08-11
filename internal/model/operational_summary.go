// Copyright (c) 2026 Seb. All rights reserved.

package model

// ProjectOperationalSummary is a read-only, durable snapshot of current Sprint execution state.
type ProjectOperationalSummary struct {
	ProjectID                string `json:"project_id"`
	TotalSprints             int64  `json:"total_sprints"`
	OpenSprints              int64  `json:"open_sprints"`
	ActiveSprints            int64  `json:"active_sprints"`
	HeldSprints              int64  `json:"held_sprints"`
	CompletedSprints         int64  `json:"completed_sprints"`
	EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
	BufferDurationSeconds    int64  `json:"buffer_duration_seconds"`
	ExtensionDurationSeconds int64  `json:"extension_duration_seconds"`
	PlannedDurationSeconds   int64  `json:"planned_duration_seconds"`
	RecordedWorkSeconds      int64  `json:"recorded_work_seconds"`
	RecordedHoldSeconds      int64  `json:"recorded_hold_seconds"`
}
