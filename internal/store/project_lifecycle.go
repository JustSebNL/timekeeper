// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const projectLifecycleSelect = `SELECT project_id, project_number, item_address, project_name,
	project_description, project_goal, status, priority, palette_id,
	reported_completion_pct, calculated_completion_pct, created_at, updated_at, project_alias,
	planned_start_date, target_end_date, actual_started_at, completed_at, cancelled_at,
	lifecycle_phase, schedule_timezone, deadline_warning_days
	FROM projects WHERE project_id = ?`

type projectQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readProject(ctx context.Context, queryer projectQueryer, projectID string) (model.Project, error) {
	project, err := scanProject(queryer.QueryRowContext(ctx, projectLifecycleSelect, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Project{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
	}
	return project, err
}

// UpdateProjectSchedule validates and persists the date-only Project schedule.
func (s *Store) UpdateProjectSchedule(ctx context.Context, projectID string, input model.UpdateProjectScheduleInput) (model.Project, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return model.Project{}, fmt.Errorf("%w: project id is required", ErrValidation)
	}
	if input.PlannedStartDate == nil && input.TargetEndDate == nil && input.ScheduleTimezone == nil && input.DeadlineWarningDays == nil {
		return model.Project{}, fmt.Errorf("%w: at least one schedule field is required", ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Project{}, fmt.Errorf("begin project schedule update: %w", err)
	}
	defer tx.Rollback()
	project, err := readProject(ctx, tx, projectID)
	if err != nil {
		return model.Project{}, err
	}

	plannedStart, targetEnd := project.PlannedStartDate, project.TargetEndDate
	timezone, warningDays := project.ScheduleTimezone, project.DeadlineWarningDays
	if input.PlannedStartDate != nil {
		plannedStart = strings.TrimSpace(*input.PlannedStartDate)
	}
	if input.TargetEndDate != nil {
		targetEnd = strings.TrimSpace(*input.TargetEndDate)
	}
	if input.ScheduleTimezone != nil {
		timezone = strings.TrimSpace(*input.ScheduleTimezone)
	}
	if input.DeadlineWarningDays != nil {
		warningDays = *input.DeadlineWarningDays
	}
	if !model.IsValidProjectDate(plannedStart) || !model.IsValidProjectDate(targetEnd) {
		return model.Project{}, fmt.Errorf("%w: project dates must use YYYY-MM-DD", ErrValidation)
	}
	if plannedStart != "" && targetEnd != "" && plannedStart > targetEnd {
		return model.Project{}, fmt.Errorf("%w: planned start date must be on or before target end date", ErrValidation)
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return model.Project{}, fmt.Errorf("%w: schedule timezone must be an IANA timezone", ErrValidation)
	}
	if warningDays < 0 || warningDays > 365 {
		return model.Project{}, fmt.Errorf("%w: deadline warning days must be between 0 and 365", ErrValidation)
	}
	changed := plannedStart != project.PlannedStartDate || targetEnd != project.TargetEndDate || timezone != project.ScheduleTimezone || warningDays != project.DeadlineWarningDays
	if !changed {
		return project, nil
	}
	now := time.Now().UTC().Round(0)
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET planned_start_date = ?, target_end_date = ?,
		schedule_timezone = ?, deadline_warning_days = ?, updated_at = ? WHERE project_id = ?`,
		plannedStart, targetEnd, timezone, warningDays, stamp, projectID); err != nil {
		return model.Project{}, fmt.Errorf("update project schedule: %w", err)
	}
	detail := fmt.Sprintf("Project schedule updated: planned_start_date %q -> %q; target_end_date %q -> %q; timezone %q -> %q; deadline_warning_days %d -> %d.", project.PlannedStartDate, plannedStart, project.TargetEndDate, targetEnd, project.ScheduleTimezone, timezone, project.DeadlineWarningDays, warningDays)
	if err := recordProjectEvent(ctx, tx, projectID, "project", projectID, "project_schedule_updated", detail, now); err != nil {
		return model.Project{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Project{}, fmt.Errorf("commit project schedule update: %w", err)
	}
	return readProject(ctx, s.db, projectID)
}

// TransitionProjectPhase advances a Project through its bounded lifecycle or deliberately replans one phase backward.
func (s *Store) TransitionProjectPhase(ctx context.Context, projectID string, input model.TransitionProjectPhaseInput) (model.Project, error) {
	projectID = strings.TrimSpace(projectID)
	input.Phase = model.ProjectPhase(strings.TrimSpace(string(input.Phase)))
	input.Reason = strings.TrimSpace(input.Reason)
	if projectID == "" || !model.IsValidProjectPhase(input.Phase) {
		return model.Project{}, fmt.Errorf("%w: project id and valid lifecycle phase are required", ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Project{}, fmt.Errorf("begin Project phase transition: %w", err)
	}
	defer tx.Rollback()
	project, err := readProject(ctx, tx, projectID)
	if err != nil {
		return model.Project{}, err
	}
	if project.LifecyclePhase == input.Phase {
		return project, nil
	}
	if !allowedProjectPhaseTransition(project.LifecyclePhase, input.Phase) {
		return model.Project{}, fmt.Errorf("%w: cannot transition Project phase from %s to %s", ErrValidation, project.LifecyclePhase, input.Phase)
	}
	if phaseRank(input.Phase) < phaseRank(project.LifecyclePhase) && input.Reason == "" {
		return model.Project{}, fmt.Errorf("%w: a reason is required when moving a Project phase backward", ErrValidation)
	}
	if (project.Status == "Completed" || project.Status == "Cancelled") && input.Phase != model.ProjectPhaseCloseout {
		return model.Project{}, fmt.Errorf("%w: reopen the Project before changing its terminal lifecycle phase", ErrValidation)
	}
	now := time.Now().UTC().Round(0)
	stamp := now.Format(time.RFC3339Nano)
	if input.Phase == model.ProjectPhaseExecution && project.ActualStartedAt == nil {
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET lifecycle_phase = ?, actual_started_at = ?, updated_at = ? WHERE project_id = ?`, input.Phase, stamp, stamp, projectID); err != nil {
			return model.Project{}, fmt.Errorf("persist Project execution start: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE projects SET lifecycle_phase = ?, updated_at = ? WHERE project_id = ?`, input.Phase, stamp, projectID); err != nil {
		return model.Project{}, fmt.Errorf("persist Project phase: %w", err)
	}
	message := fmt.Sprintf("Project lifecycle phase changed from %s to %s.", project.LifecyclePhase, input.Phase)
	if input.Reason != "" {
		message += " Reason: " + input.Reason
	}
	if err := recordProjectEvent(ctx, tx, projectID, "project", projectID, "project_phase_changed", message, now); err != nil {
		return model.Project{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Project{}, fmt.Errorf("commit Project phase transition: %w", err)
	}
	return readProject(ctx, s.db, projectID)
}

func phaseRank(phase model.ProjectPhase) int {
	for i, candidate := range model.ValidProjectPhases {
		if candidate == phase {
			return i
		}
	}
	return -1
}

func allowedProjectPhaseTransition(from, to model.ProjectPhase) bool {
	switch from {
	case model.ProjectPhaseIntake:
		return to == model.ProjectPhasePlanning
	case model.ProjectPhasePlanning:
		return to == model.ProjectPhaseIntake || to == model.ProjectPhaseExecution
	case model.ProjectPhaseExecution:
		return to == model.ProjectPhasePlanning || to == model.ProjectPhaseValidation
	case model.ProjectPhaseValidation:
		return to == model.ProjectPhaseExecution || to == model.ProjectPhaseCloseout
	case model.ProjectPhaseCloseout:
		return to == model.ProjectPhaseValidation
	default:
		return false
	}
}
