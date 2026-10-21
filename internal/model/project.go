// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import (
	"strings"
	"time"
)

// ProjectPhase is the lifecycle stage, independent of the operational Project status.
type ProjectPhase string

const (
	ProjectPhaseIntake     ProjectPhase = "intake"
	ProjectPhasePlanning   ProjectPhase = "planning"
	ProjectPhaseExecution  ProjectPhase = "execution"
	ProjectPhaseValidation ProjectPhase = "validation"
	ProjectPhaseCloseout   ProjectPhase = "closeout"
)

var ValidProjectPhases = []ProjectPhase{
	ProjectPhaseIntake, ProjectPhasePlanning, ProjectPhaseExecution, ProjectPhaseValidation, ProjectPhaseCloseout,
}

func IsValidProjectPhase(phase ProjectPhase) bool {
	for _, valid := range ValidProjectPhases {
		if phase == valid {
			return true
		}
	}
	return false
}

func IsValidProjectDate(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func (p Project) ScheduleState(now time.Time) (string, *int) {
	if p.Status == "Completed" {
		return "completed", nil
	}
	if p.Status == "Cancelled" {
		return "cancelled", nil
	}
	if p.TargetEndDate == "" {
		return "unscheduled", nil
	}
	location := time.UTC
	if strings.TrimSpace(p.ScheduleTimezone) != "" {
		if loaded, err := time.LoadLocation(p.ScheduleTimezone); err == nil {
			location = loaded
		}
	}
	target, err := time.Parse("2006-01-02", p.TargetEndDate)
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
	if days <= p.DeadlineWarningDays {
		return "due_soon", &days
	}
	return "on_track", &days
}

// Project is the top-level unit of tracked work.
type Project struct {
	ProjectID               string       `json:"project_id"`
	ProjectNumber           int64        `json:"project_number"`
	ItemAddress             string       `json:"item_address"`
	ProjectName             string       `json:"project_name"`
	ProjectDescription      string       `json:"project_description,omitempty"`
	ProjectGoal             string       `json:"project_goal,omitempty"`
	PlannedStartDate        string       `json:"planned_start_date,omitempty"`
	TargetEndDate           string       `json:"target_end_date,omitempty"`
	ActualStartedAt         *time.Time   `json:"actual_started_at,omitempty"`
	CompletedAt             *time.Time   `json:"completed_at,omitempty"`
	CancelledAt             *time.Time   `json:"cancelled_at,omitempty"`
	LifecyclePhase          ProjectPhase `json:"lifecycle_phase"`
	ScheduleTimezone        string       `json:"schedule_timezone"`
	DeadlineWarningDays     int          `json:"deadline_warning_days"`
	DeadlineState           string       `json:"deadline_state,omitempty"`
	DaysToDeadline          *int         `json:"days_to_deadline,omitempty"`
	Status                  string       `json:"status"`
	Priority                string       `json:"priority"`
	PaletteID               int          `json:"palette_id"`
	ReportedCompletionPct   float64      `json:"reported_completion_pct"`
	CalculatedCompletionPct float64      `json:"calculated_completion_pct"`
	CreatedAt               time.Time    `json:"created_at"`
	UpdatedAt               time.Time    `json:"updated_at"`
	ProjectAlias            string       `json:"project_alias,omitempty"`
}

// CreateProjectInput holds client-supplied top-level project fields.
type CreateProjectInput struct {
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Goal                string       `json:"goal"`
	Priority            string       `json:"priority"`
	PaletteID           int          `json:"palette_id"`
	Alias               string       `json:"alias"`
	PlannedStartDate    string       `json:"planned_start_date"`
	TargetEndDate       string       `json:"target_end_date"`
	LifecyclePhase      ProjectPhase `json:"lifecycle_phase"`
	ScheduleTimezone    string       `json:"schedule_timezone"`
	DeadlineWarningDays int          `json:"deadline_warning_days"`
}

// UpdateProjectMetadataInput holds editable durable top-level Project context.
type UpdateProjectMetadataInput struct {
	Goal        string `json:"goal"`
	Description string `json:"description"`
}

// UpdateProjectStatusInput holds an explicit top-level project workflow state.
type UpdateProjectStatusInput struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type UpdateProjectScheduleInput struct {
	PlannedStartDate    *string `json:"planned_start_date,omitempty"`
	TargetEndDate       *string `json:"target_end_date,omitempty"`
	ScheduleTimezone    *string `json:"schedule_timezone,omitempty"`
	DeadlineWarningDays *int    `json:"deadline_warning_days,omitempty"`
}

type TransitionProjectPhaseInput struct {
	Phase  ProjectPhase `json:"phase"`
	Reason string       `json:"reason"`
}

// UpdateProjectAliasInput holds an optional human-readable alias for CLI resolution.
type UpdateProjectAliasInput struct {
	Alias string `json:"alias"`
}
