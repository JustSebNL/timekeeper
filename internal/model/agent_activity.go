package model

import "time"

// AgentActivityInput carries metadata, never prompts or tool output. SessionID
// identifies an individual runtime session; Runtime is descriptive, not authority.
type AgentActivityInput struct {
	SessionID            string  `json:"session_id"`
	Runtime              string  `json:"runtime,omitempty"`
	Event                string  `json:"event"`
	EventID              string  `json:"event_id,omitempty"`
	ProjectID            string  `json:"project_id,omitempty"`
	SprintID             string  `json:"sprint_id,omitempty"`
	LeaseDurationSeconds int64   `json:"lease_duration_seconds,omitempty"`
	WaitDurationSeconds  int64   `json:"wait_duration_seconds,omitempty"`
	Reason               string  `json:"reason,omitempty"`
	GuardianURL          *string `json:"guardian_url,omitempty"`
	InputTokens          int64   `json:"input_tokens,omitempty"`
	OutputTokens         int64   `json:"output_tokens,omitempty"`
	CachedTokens         int64   `json:"cached_tokens,omitempty"`
	CacheCreationTokens  int64   `json:"cache_creation_tokens,omitempty"`
}

// AgentWorkState struct remains the same...

type AgentWorkState struct {
	AgentID              string     `json:"agent_id"`
	SessionID            string     `json:"session_id"`
	Runtime              string     `json:"runtime"`
	ProjectID            string     `json:"project_id"`
	ActiveSprintID       string     `json:"active_sprint_id"`
	State                string     `json:"state"`
	ReportedState        string     `json:"reported_state"`
	Reason               string     `json:"reason"`
	WorkExpected         bool       `json:"work_expected"`
	LastContactAt        time.Time  `json:"last_contact_at"`
	LastProgressAt       time.Time  `json:"last_progress_at"`
	StateChangedAt       time.Time  `json:"state_changed_at"`
	Deadline             *time.Time `json:"deadline,omitempty"`
	WaitUntil            *time.Time `json:"wait_until,omitempty"`
	LeaseDurationSeconds int64      `json:"lease_duration_seconds"`
}
