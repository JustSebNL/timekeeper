// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package model

import "time"

type ProjectMilestoneStatus string

const (
	MilestonePlanned    ProjectMilestoneStatus = "Planned"
	MilestoneInProgress ProjectMilestoneStatus = "In Progress"
	MilestoneCompleted  ProjectMilestoneStatus = "Completed"
	MilestoneCancelled  ProjectMilestoneStatus = "Cancelled"
)

// ProjectMilestone is a dated deliverable checkpoint; linked Tasks remain the execution source.
type ProjectMilestone struct {
	MilestoneID    string                 `json:"milestone_id"`
	ProjectID      string                 `json:"project_id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description,omitempty"`
	TargetDate     string                 `json:"target_date"`
	Status         ProjectMilestoneStatus `json:"status"`
	LinkedTaskID   string                 `json:"linked_task_id,omitempty"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	DeadlineState  string                 `json:"deadline_state,omitempty"`
	DaysToDeadline *int                   `json:"days_to_deadline,omitempty"`
}

type CreateProjectMilestoneInput struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	TargetDate   string `json:"target_date"`
	LinkedTaskID string `json:"linked_task_id"`
}

type UpdateProjectMilestoneInput struct {
	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	TargetDate   *string `json:"target_date,omitempty"`
	LinkedTaskID *string `json:"linked_task_id,omitempty"`
}

type UpdateProjectMilestoneStatusInput struct {
	Status ProjectMilestoneStatus `json:"status"`
	Reason string                 `json:"reason"`
}

func (m ProjectMilestone) ScheduleState(now time.Time, timezone string, warningDays int) (string, *int) {
	if m.Status == MilestoneCompleted {
		return "completed", nil
	}
	if m.Status == MilestoneCancelled {
		return "cancelled", nil
	}
	location := time.UTC
	if timezone != "" {
		if loaded, err := time.LoadLocation(timezone); err == nil {
			location = loaded
		}
	}
	target, err := time.Parse("2006-01-02", m.TargetDate)
	if err != nil {
		return "unscheduled", nil
	}
	localNow := now.In(location)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	targetDay := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	days := int(targetDay.Sub(today).Hours() / 24)
	if days < 0 {
		return "overdue", &days
	}
	if days <= warningDays {
		return "due_soon", &days
	}
	return "on_track", &days
}
