// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

// Pulse is a read-only local attention snapshot. It never sends or stores notifications.
type Pulse struct {
	Format                      string           `json:"format"`
	GeneratedAt                 time.Time        `json:"generated_at"`
	RecommendedNextPulseSeconds int64            `json:"recommended_next_pulse_seconds"`
	Attention                   []PulseAttention `json:"attention"`
}

// PulseAttention explains a currently active Sprint that has exceeded its declared plan.
type PulseAttention struct {
	Kind                   string `json:"kind"`
	ProjectID              string `json:"project_id"`
	SprintID               string `json:"sprint_id"`
	ItemAddress            string `json:"item_address"`
	Name                   string `json:"name"`
	Status                 string `json:"status"`
	PlannedDurationSeconds int64  `json:"planned_duration_seconds"`
	ActiveDurationSeconds  int64  `json:"active_duration_seconds"`
	OverdueDurationSeconds int64  `json:"overdue_duration_seconds"`
}

// AgentPulseProgress is an agent's explicit material-progress lease. A lease is
// not a passive health probe: it expires if the agent stops reporting progress.
type AgentPulseProgressInput struct {
	// ActiveSprintID is optional so a bare lease renewal preserves the active
	// Sprint. Send an explicit empty string to clear the association.
	ActiveSprintID       *string `json:"active_sprint_id"`
	LeaseDurationSeconds int64   `json:"lease_duration_seconds"`
	// GuardianURL is optional so ordinary lease renewals preserve an already
	// registered Guardian. Send an explicit empty string to unregister it.
	GuardianURL *string `json:"guardian_url"`
}

type AgentPulseProgress struct {
	AgentID              string    `json:"agent_id"`
	ActiveSprintID       string    `json:"active_sprint_id"`
	LeaseDurationSeconds int64     `json:"lease_duration_seconds"`
	GuardianURL          string    `json:"guardian_url,omitempty"`
	LastProgressAt       time.Time `json:"last_progress_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// PulseNudge is a durable Guardian escalation. A Pending nudge remains visible
// until its owning agent explicitly acknowledges it.
type PulseNudge struct {
	NudgeID              int64      `json:"nudge_id"`
	AgentID              string     `json:"agent_id"`
	ActiveSprintID       string     `json:"active_sprint_id"`
	Kind                 string     `json:"kind"`
	Status               string     `json:"status"`
	DetectedAfterSeconds int64      `json:"detected_after_seconds"`
	DeliveryAttempts     int64      `json:"delivery_attempts"`
	LastDeliveryAt       *time.Time `json:"last_delivery_at,omitempty"`
	DeliveredAt          *time.Time `json:"delivered_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	AcknowledgedAt       *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy       string     `json:"acknowledged_by,omitempty"`
}

// RegisteredGuardian pairs an agent with its explicitly registered local
// recovery callback. It is read-only status evidence, not a capability grant.
type RegisteredGuardian struct {
	AgentID     string `json:"agent_id"`
	GuardianURL string `json:"guardian_url"`
}

// PulseGuardianSignal is the versioned, local-only callback payload for an
// independent Guardian. A callback recipient must perform attention recovery
// outside the watched agent's potentially hung work loop, then explicitly
// acknowledge the nudge through Time Keeper once recovery is observed.
type PulseGuardianSignal struct {
	Format string     `json:"format"`
	Action string     `json:"action"`
	Nudge  PulseNudge `json:"nudge"`
}
