// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package model

type ProjectDeadlineItem struct {
	ProjectID      string `json:"project_id"`
	ProjectName    string `json:"project_name"`
	EntityType     string `json:"entity_type"`
	EntityID       string `json:"entity_id"`
	Name           string `json:"name"`
	TargetDate     string `json:"target_date"`
	Status         string `json:"status"`
	DeadlineState  string `json:"deadline_state"`
	DaysToDeadline *int   `json:"days_to_deadline,omitempty"`
}

type ProjectDeadlineWatch struct {
	Items []ProjectDeadlineItem `json:"items"`
}
