// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

// SprintRetrievalAttempt is immutable evidence that a bounded retrieval cycle
// did not produce material progress. The fourth attempt makes the Sprint dormant.
type SprintRetrievalAttempt struct {
	AttemptID     int64     `json:"attempt_id"`
	SprintID      string    `json:"sprint_id"`
	AttemptNumber int       `json:"attempt_number"`
	Reason        string    `json:"reason"`
	TimedOut      bool      `json:"timed_out"`
	CreatedAt     time.Time `json:"created_at"`
}
