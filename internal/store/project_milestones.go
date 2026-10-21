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

const projectMilestoneSelect = `SELECT m.milestone_id, m.project_id, m.milestone_name,
	m.milestone_description, m.target_date, m.status, COALESCE(m.linked_task_id, ''),
	m.completed_at, m.created_at, m.updated_at, p.schedule_timezone, p.deadline_warning_days
	FROM project_milestones m JOIN projects p ON p.project_id = m.project_id
	WHERE m.project_id = ? AND m.milestone_id = ?`

type milestoneScanner interface{ Scan(...any) error }

func scanProjectMilestone(scanner milestoneScanner) (model.ProjectMilestone, error) {
	var milestone model.ProjectMilestone
	var completedAt sql.NullString
	var createdAt, updatedAt, timezone string
	var warningDays int
	if err := scanner.Scan(&milestone.MilestoneID, &milestone.ProjectID, &milestone.Name,
		&milestone.Description, &milestone.TargetDate, &milestone.Status, &milestone.LinkedTaskID,
		&completedAt, &createdAt, &updatedAt, &timezone, &warningDays); err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("scan Project milestone: %w", err)
	}
	var err error
	milestone.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("parse milestone created_at: %w", err)
	}
	milestone.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("parse milestone updated_at: %w", err)
	}
	if completedAt.Valid {
		completed, parseErr := time.Parse(time.RFC3339Nano, completedAt.String)
		if parseErr != nil {
			return model.ProjectMilestone{}, fmt.Errorf("parse milestone completed_at: %w", parseErr)
		}
		milestone.CompletedAt = &completed
	}
	milestone.DeadlineState, milestone.DaysToDeadline = milestone.ScheduleState(time.Now(), timezone, warningDays)
	return milestone, nil
}

func readProjectMilestone(ctx context.Context, queryer projectQueryer, projectID, milestoneID string) (model.ProjectMilestone, error) {
	milestone, err := scanProjectMilestone(queryer.QueryRowContext(ctx, projectMilestoneSelect, projectID, milestoneID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.ProjectMilestone{}, fmt.Errorf("%w: milestone %s in project %s", ErrNotFound, milestoneID, projectID)
	}
	return milestone, err
}

func validateMilestoneTask(ctx context.Context, queryer projectQueryer, projectID, taskID string) error {
	if taskID == "" {
		return nil
	}
	var exists int
	if err := queryer.QueryRowContext(ctx, "SELECT 1 FROM tasks WHERE task_id = ? AND project_id = ?", taskID, projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: linked Task must belong to the same Project", ErrValidation)
		}
		return fmt.Errorf("validate milestone Task link: %w", err)
	}
	return nil
}

// CreateProjectMilestone creates a dated checkpoint under its owning Project.
func (s *Store) CreateProjectMilestone(ctx context.Context, projectID string, input model.CreateProjectMilestoneInput) (model.ProjectMilestone, error) {
	projectID = strings.TrimSpace(projectID)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.TargetDate = strings.TrimSpace(input.TargetDate)
	input.LinkedTaskID = strings.TrimSpace(input.LinkedTaskID)
	if projectID == "" || input.Name == "" || !model.IsValidProjectDate(input.TargetDate) || input.TargetDate == "" {
		return model.ProjectMilestone{}, fmt.Errorf("%w: Project, milestone name, and YYYY-MM-DD target date are required", ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("begin milestone creation: %w", err)
	}
	defer tx.Rollback()
	if _, err := readProject(ctx, tx, projectID); err != nil {
		return model.ProjectMilestone{}, err
	}
	if err := validateMilestoneTask(ctx, tx, projectID, input.LinkedTaskID); err != nil {
		return model.ProjectMilestone{}, err
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.ProjectMilestone{}, err
	}
	now := time.Now().UTC().Round(0)
	milestoneID := fmt.Sprintf("MS-%d", number)
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_milestones (
		milestone_id, project_id, milestone_name, milestone_description, target_date,
		status, linked_task_id, completed_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`, milestoneID, projectID, input.Name, input.Description,
		input.TargetDate, model.MilestonePlanned, nullString(input.LinkedTaskID), stamp, stamp); err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("insert Project milestone: %w", err)
	}
	if err := recordProjectEvent(ctx, tx, projectID, "milestone", milestoneID, "project_milestone_created", fmt.Sprintf("Milestone %q created for %s.", input.Name, input.TargetDate), now); err != nil {
		return model.ProjectMilestone{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("commit milestone creation: %w", err)
	}
	return readProjectMilestone(ctx, s.db, projectID, milestoneID)
}

// ListProjectMilestones returns milestones in deadline order for one Project.
func (s *Store) ListProjectMilestones(ctx context.Context, projectID string) ([]model.ProjectMilestone, error) {
	projectID = strings.TrimSpace(projectID)
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return nil, fmt.Errorf("read milestone Project: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT m.milestone_id, m.project_id, m.milestone_name,
		m.milestone_description, m.target_date, m.status, COALESCE(m.linked_task_id, ''),
		m.completed_at, m.created_at, m.updated_at, p.schedule_timezone, p.deadline_warning_days
		FROM project_milestones m JOIN projects p ON p.project_id = m.project_id
		WHERE m.project_id = ? ORDER BY CASE WHEN m.status IN ('Completed', 'Cancelled') THEN 1 ELSE 0 END,
		m.target_date, m.created_at, m.milestone_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list Project milestones: %w", err)
	}
	defer rows.Close()
	milestones := make([]model.ProjectMilestone, 0)
	for rows.Next() {
		milestone, err := scanProjectMilestone(rows)
		if err != nil {
			return nil, err
		}
		milestones = append(milestones, milestone)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Project milestones: %w", err)
	}
	return milestones, nil
}

// UpdateProjectMilestone edits checkpoint details without changing its workflow status.
func (s *Store) UpdateProjectMilestone(ctx context.Context, projectID, milestoneID string, input model.UpdateProjectMilestoneInput) (model.ProjectMilestone, error) {
	projectID, milestoneID = strings.TrimSpace(projectID), strings.TrimSpace(milestoneID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("begin milestone update: %w", err)
	}
	defer tx.Rollback()
	milestone, err := readProjectMilestone(ctx, tx, projectID, milestoneID)
	if err != nil {
		return model.ProjectMilestone{}, err
	}
	name, description, targetDate, linkedTaskID := milestone.Name, milestone.Description, milestone.TargetDate, milestone.LinkedTaskID
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		description = strings.TrimSpace(*input.Description)
	}
	if input.TargetDate != nil {
		targetDate = strings.TrimSpace(*input.TargetDate)
	}
	if input.LinkedTaskID != nil {
		linkedTaskID = strings.TrimSpace(*input.LinkedTaskID)
	}
	if name == "" || targetDate == "" || !model.IsValidProjectDate(targetDate) {
		return model.ProjectMilestone{}, fmt.Errorf("%w: milestone name and YYYY-MM-DD target date are required", ErrValidation)
	}
	if err := validateMilestoneTask(ctx, tx, projectID, linkedTaskID); err != nil {
		return model.ProjectMilestone{}, err
	}
	changed := name != milestone.Name || description != milestone.Description || targetDate != milestone.TargetDate || linkedTaskID != milestone.LinkedTaskID
	if !changed {
		return milestone, nil
	}
	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, `UPDATE project_milestones SET milestone_name = ?, milestone_description = ?,
		target_date = ?, linked_task_id = ?, updated_at = ? WHERE project_id = ? AND milestone_id = ?`,
		name, description, targetDate, nullString(linkedTaskID), now.Format(time.RFC3339Nano), projectID, milestoneID); err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("update Project milestone: %w", err)
	}
	message := fmt.Sprintf("Milestone updated: name %q -> %q; target_date %q -> %q; linked_task_id %q -> %q.", milestone.Name, name, milestone.TargetDate, targetDate, milestone.LinkedTaskID, linkedTaskID)
	if err := recordProjectEvent(ctx, tx, projectID, "milestone", milestoneID, "project_milestone_updated", message, now); err != nil {
		return model.ProjectMilestone{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("commit milestone update: %w", err)
	}
	return readProjectMilestone(ctx, s.db, projectID, milestoneID)
}

// UpdateProjectMilestoneStatus applies an audited milestone state transition.
func (s *Store) UpdateProjectMilestoneStatus(ctx context.Context, projectID, milestoneID string, input model.UpdateProjectMilestoneStatusInput) (model.ProjectMilestone, error) {
	projectID, milestoneID = strings.TrimSpace(projectID), strings.TrimSpace(milestoneID)
	input.Status = model.ProjectMilestoneStatus(strings.TrimSpace(string(input.Status)))
	input.Reason = strings.TrimSpace(input.Reason)
	if !validMilestoneStatus(input.Status) {
		return model.ProjectMilestone{}, fmt.Errorf("%w: invalid milestone status", ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("begin milestone status update: %w", err)
	}
	defer tx.Rollback()
	milestone, err := readProjectMilestone(ctx, tx, projectID, milestoneID)
	if err != nil {
		return model.ProjectMilestone{}, err
	}
	if milestone.Status == input.Status {
		return milestone, nil
	}
	if !allowedMilestoneTransition(milestone.Status, input.Status) {
		return model.ProjectMilestone{}, fmt.Errorf("%w: cannot transition milestone from %s to %s", ErrValidation, milestone.Status, input.Status)
	}
	reasonRequired := input.Status == model.MilestoneCancelled || input.Status == model.MilestonePlanned || milestone.Status == model.MilestoneCompleted || milestone.Status == model.MilestoneCancelled
	if reasonRequired && input.Reason == "" {
		return model.ProjectMilestone{}, fmt.Errorf("%w: a reason is required for this milestone transition", ErrValidation)
	}
	now := time.Now().UTC().Round(0)
	stamp := now.Format(time.RFC3339Nano)
	var completedAt any
	switch input.Status {
	case model.MilestoneCompleted:
		completedAt = stamp
	case model.MilestoneInProgress, model.MilestonePlanned, model.MilestoneCancelled:
		completedAt = nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE project_milestones SET status = ?, completed_at = ?, updated_at = ?
		WHERE project_id = ? AND milestone_id = ?`, input.Status, completedAt, stamp, projectID, milestoneID); err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("update milestone status: %w", err)
	}
	message := fmt.Sprintf("Milestone status changed from %s to %s.", milestone.Status, input.Status)
	if input.Reason != "" {
		message += " Reason: " + input.Reason
	}
	if err := recordProjectEvent(ctx, tx, projectID, "milestone", milestoneID, "project_milestone_status_changed", message, now); err != nil {
		return model.ProjectMilestone{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.ProjectMilestone{}, fmt.Errorf("commit milestone status: %w", err)
	}
	return readProjectMilestone(ctx, s.db, projectID, milestoneID)
}

func validMilestoneStatus(status model.ProjectMilestoneStatus) bool {
	return status == model.MilestonePlanned || status == model.MilestoneInProgress || status == model.MilestoneCompleted || status == model.MilestoneCancelled
}

func allowedMilestoneTransition(from, to model.ProjectMilestoneStatus) bool {
	switch from {
	case model.MilestonePlanned:
		return to == model.MilestoneInProgress || to == model.MilestoneCompleted || to == model.MilestoneCancelled
	case model.MilestoneInProgress:
		return to == model.MilestonePlanned || to == model.MilestoneCompleted || to == model.MilestoneCancelled
	case model.MilestoneCompleted:
		return to == model.MilestoneInProgress
	case model.MilestoneCancelled:
		return to == model.MilestonePlanned
	default:
		return false
	}
}
