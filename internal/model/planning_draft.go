// Copyright (c) 2026 Seb. All rights reserved.

package model

import "time"

// PlanningDraft is a durable, review-only local-LLM proposal. RawJSON is kept
// so a user approves exactly the generated artifact, not a reconstructed copy.
type PlanningDraft struct {
	DraftID    int64      `json:"draft_id"`
	ProjectID  string     `json:"project_id"`
	PipelineID int64      `json:"pipeline_id"`
	Status     string     `json:"status"`
	Summary    string     `json:"summary"`
	RawJSON    string     `json:"raw_json"`
	CreatedAt  time.Time  `json:"created_at"`
	AppliedAt  *time.Time `json:"applied_at,omitempty"`
}
