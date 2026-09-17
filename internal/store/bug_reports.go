// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JustSebNL/timekeeper/internal/model"
)

var ErrBugReportConflict = errors.New("bug report conflicts with an existing dedupe key")

var sensitiveBugReportPattern = regexp.MustCompile(`(?i)(-----begin[^\n]*private key-----|x-api-key\s*:|authorization\s*:\s*bearer|password\s*=|cookie\s*:|connection string)`)

func (s *Store) CreateBugReport(ctx context.Context, input model.CreateBugReportInput) (model.BugReportResult, error) {
	if err := validateBugReportInput(input); err != nil {
		return model.BugReportResult{}, err
	}
	if err := s.validateBugReportReferences(ctx, input); err != nil {
		return model.BugReportResult{}, err
	}

	var existing model.BugReport
	found, existing, err := s.findBugReportByIdempotency(ctx, input.IdempotencyKey)
	if err != nil {
		return model.BugReportResult{}, err
	}
	if found {
		return model.BugReportResult{Report: existing, Duplicate: true}, nil
	}
	if strings.TrimSpace(input.DedupeKey) != "" {
		found, existing, err = s.findBugReportByDedupe(ctx, input.ProjectID, input.DedupeKey)
		if err != nil {
			return model.BugReportResult{}, err
		}
		if found {
			return model.BugReportResult{}, fmt.Errorf("%w: existing report %s", ErrBugReportConflict, existing.ReportID)
		}
	}

	stepsJSON, _ := json.Marshal(input.ReproductionSteps)
	environmentJSON, _ := json.Marshal(input.Environment)
	sourceJSON, _ := json.Marshal(input.Source)
	evidenceJSON, _ := json.Marshal(input.Evidence)
	now := time.Now().UTC().Round(0)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.BugReportResult{}, fmt.Errorf("begin bug report: %w", err)
	}
	defer tx.Rollback()
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.BugReportResult{}, err
	}
	reportID := fmt.Sprintf("BR-%d", number)
	_, err = tx.ExecContext(ctx, `INSERT INTO bug_reports (
		report_id, report_number, project_id, category_id, task_id, sprint_id, title, severity,
		expected, actual, reproduction_steps_json, environment_json, source_json, evidence_json,
		dedupe_key, idempotency_key, redaction_status, status, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'New', ?, ?)`,
		reportID, number, input.ProjectID, input.CategoryID, input.TaskID, input.SprintID,
		input.Title, input.Severity, input.Expected, input.Actual, string(stepsJSON), string(environmentJSON),
		string(sourceJSON), string(evidenceJSON), input.DedupeKey, input.IdempotencyKey, input.RedactionStatus,
		stamp(now), stamp(now))
	if err != nil {
		return model.BugReportResult{}, fmt.Errorf("insert bug report: %w", err)
	}
	if err := recordProjectEvent(ctx, tx, input.ProjectID, "bug_report", reportID, "bug_report_created", input.Title, now); err != nil {
		return model.BugReportResult{}, fmt.Errorf("record bug report event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.BugReportResult{}, fmt.Errorf("commit bug report: %w", err)
	}
	report, err := s.GetBugReport(ctx, input.ProjectID, reportID)
	if err != nil {
		return model.BugReportResult{}, err
	}
	return model.BugReportResult{Report: report}, nil
}

func (s *Store) ListBugReports(ctx context.Context, projectID string) ([]model.BugReport, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return nil, fmt.Errorf("read bug report project: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT report_id, project_id, category_id, task_id, sprint_id, title, severity,
		expected, actual, reproduction_steps_json, environment_json, source_json, evidence_json, dedupe_key,
		idempotency_key, redaction_status, status, created_at, updated_at
		FROM bug_reports WHERE project_id = ? ORDER BY created_at DESC, report_number DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list bug reports: %w", err)
	}
	defer rows.Close()
	items := make([]model.BugReport, 0)
	for rows.Next() {
		report, err := scanBugReport(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bug reports: %w", err)
	}
	return items, nil
}

func (s *Store) GetBugReport(ctx context.Context, projectID, reportID string) (model.BugReport, error) {
	row := s.db.QueryRowContext(ctx, `SELECT report_id, project_id, category_id, task_id, sprint_id, title, severity,
		expected, actual, reproduction_steps_json, environment_json, source_json, evidence_json, dedupe_key,
		idempotency_key, redaction_status, status, created_at, updated_at
		FROM bug_reports WHERE project_id = ? AND report_id = ?`, projectID, reportID)
	report, err := scanBugReport(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.BugReport{}, fmt.Errorf("%w: report %s", ErrNotFound, reportID)
	}
	return report, err
}

func (s *Store) validateBugReportReferences(ctx context.Context, input model.CreateBugReportInput) error {
	var projectID string
	if err := s.db.QueryRowContext(ctx, "SELECT project_id FROM projects WHERE project_id = ?", input.ProjectID).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("project_id %s was not found", input.ProjectID)
		}
		return fmt.Errorf("validate project_id: %w", err)
	}
	if input.CategoryID != "" {
		var owner string
		if err := s.db.QueryRowContext(ctx, "SELECT project_id FROM categories WHERE category_id = ?", input.CategoryID).Scan(&owner); err != nil || owner != input.ProjectID {
			return fmt.Errorf("category_id %s is not owned by project_id %s", input.CategoryID, input.ProjectID)
		}
	}
	if input.TaskID != "" {
		var owner, categoryID string
		if err := s.db.QueryRowContext(ctx, "SELECT project_id, category_id FROM tasks WHERE task_id = ?", input.TaskID).Scan(&owner, &categoryID); err != nil || owner != input.ProjectID || (input.CategoryID != "" && categoryID != input.CategoryID) {
			return fmt.Errorf("task_id %s is not valid for the selected project/category", input.TaskID)
		}
	}
	if input.SprintID != "" {
		var owner, taskID string
		if err := s.db.QueryRowContext(ctx, "SELECT project_id, task_id FROM sprints WHERE sprint_id = ?", input.SprintID).Scan(&owner, &taskID); err != nil || owner != input.ProjectID || (input.TaskID != "" && taskID != input.TaskID) {
			return fmt.Errorf("sprint_id %s is not valid for the selected project/task", input.SprintID)
		}
	}
	return nil
}

func validateBugReportInput(input model.CreateBugReportInput) error {
	if strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Expected) == "" || strings.TrimSpace(input.Actual) == "" {
		return errors.New("project_id, title, expected, and actual are required")
	}
	if input.Severity != "P0" && input.Severity != "P1" && input.Severity != "P2" && input.Severity != "P3" {
		return errors.New("severity must be P0, P1, P2, or P3")
	}
	if len(input.ReproductionSteps) == 0 || len(input.ReproductionSteps) > model.MaxBugReportSteps {
		return fmt.Errorf("reproduction_steps must contain 1-%d items", model.MaxBugReportSteps)
	}
	if input.RedactionStatus != "verified" {
		return errors.New("redaction_status must be verified")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" || utf8.RuneCountInString(input.IdempotencyKey) > 160 {
		return errors.New("idempotency_key is required and must be at most 160 characters")
	}
	if utf8.RuneCountInString(input.Title) > model.MaxBugReportTitleLength {
		return errors.New("title is too long")
	}
	for _, value := range []string{input.Title, input.Expected, input.Actual, input.DedupeKey, input.IdempotencyKey, input.Source.AgentID, input.Source.SessionID, input.Source.Command} {
		if sensitiveBugReportPattern.MatchString(value) {
			return errors.New("report contains sensitive material")
		}
	}
	for _, step := range input.ReproductionSteps {
		if utf8.RuneCountInString(step) > model.MaxBugReportTextLength || sensitiveBugReportPattern.MatchString(step) {
			return errors.New("reproduction_steps contains invalid or sensitive content")
		}
	}
	if len(input.Evidence) > model.MaxBugReportEvidence {
		return fmt.Errorf("evidence must contain at most %d items", model.MaxBugReportEvidence)
	}
	for _, item := range input.Evidence {
		if item.Type == "" || item.Ref == "" || utf8.RuneCountInString(item.Ref) > 500 || sensitiveBugReportPattern.MatchString(item.Ref) {
			return errors.New("evidence items must contain bounded non-sensitive type/ref values")
		}
	}
	return nil
}

func (s *Store) findBugReportByIdempotency(ctx context.Context, key string) (bool, model.BugReport, error) {
	row := s.db.QueryRowContext(ctx, `SELECT report_id, project_id, category_id, task_id, sprint_id, title, severity, expected, actual, reproduction_steps_json, environment_json, source_json, evidence_json, dedupe_key, idempotency_key, redaction_status, status, created_at, updated_at FROM bug_reports WHERE idempotency_key = ?`, key)
	report, err := scanBugReport(row)
	if errors.Is(err, sql.ErrNoRows) {
		return false, model.BugReport{}, nil
	}
	return err == nil, report, err
}

func (s *Store) findBugReportByDedupe(ctx context.Context, projectID, key string) (bool, model.BugReport, error) {
	row := s.db.QueryRowContext(ctx, `SELECT report_id, project_id, category_id, task_id, sprint_id, title, severity, expected, actual, reproduction_steps_json, environment_json, source_json, evidence_json, dedupe_key, idempotency_key, redaction_status, status, created_at, updated_at FROM bug_reports WHERE project_id = ? AND dedupe_key = ?`, projectID, key)
	report, err := scanBugReport(row)
	if errors.Is(err, sql.ErrNoRows) {
		return false, model.BugReport{}, nil
	}
	return err == nil, report, err
}

func scanBugReport(scanner interface{ Scan(...any) error }) (model.BugReport, error) {
	var report model.BugReport
	var steps, environment, source, evidence, createdAt, updatedAt string
	if err := scanner.Scan(&report.ReportID, &report.ProjectID, &report.CategoryID, &report.TaskID, &report.SprintID, &report.Title, &report.Severity, &report.Expected, &report.Actual, &steps, &environment, &source, &evidence, &report.DedupeKey, &report.IdempotencyKey, &report.RedactionStatus, &report.Status, &createdAt, &updatedAt); err != nil {
		return model.BugReport{}, err
	}
	if err := json.Unmarshal([]byte(steps), &report.ReproductionSteps); err != nil {
		return model.BugReport{}, fmt.Errorf("decode bug report steps: %w", err)
	}
	if environment != "" {
		if err := json.Unmarshal([]byte(environment), &report.Environment); err != nil {
			return model.BugReport{}, fmt.Errorf("decode bug report environment: %w", err)
		}
	}
	if err := json.Unmarshal([]byte(source), &report.Source); err != nil {
		return model.BugReport{}, fmt.Errorf("decode bug report source: %w", err)
	}
	if evidence != "" {
		if err := json.Unmarshal([]byte(evidence), &report.Evidence); err != nil {
			return model.BugReport{}, fmt.Errorf("decode bug report evidence: %w", err)
		}
	}
	var err error
	report.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.BugReport{}, err
	}
	report.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return model.BugReport{}, err
	}
	return report, nil
}

func (s *Store) UpdateBugReportStatus(ctx context.Context, projectID, reportID, status string) (model.BugReport, error) {
	allowed := map[string]bool{"New": true, "Confirmed": true, "In Progress": true, "Fixed": true, "Verified": true, "Duplicate": true, "Rejected": true}
	if !allowed[status] {
		return model.BugReport{}, errors.New("status must be New, Confirmed, In Progress, Fixed, Verified, Duplicate, or Rejected")
	}
	now := time.Now().UTC().Round(0)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.BugReport{}, fmt.Errorf("begin bug report status: %w", err)
	}
	defer tx.Rollback()
	var title string
	if err := tx.QueryRowContext(ctx, "SELECT title FROM bug_reports WHERE project_id = ? AND report_id = ?", projectID, reportID).Scan(&title); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.BugReport{}, fmt.Errorf("%w: report %s", ErrNotFound, reportID)
		}
		return model.BugReport{}, fmt.Errorf("read bug report status: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE bug_reports SET status = ?, updated_at = ? WHERE project_id = ? AND report_id = ?", status, stamp(now), projectID, reportID); err != nil {
		return model.BugReport{}, fmt.Errorf("update bug report status: %w", err)
	}
	if err := recordProjectEvent(ctx, tx, projectID, "bug_report", reportID, "bug_report_status_changed", status+": "+title, now); err != nil {
		return model.BugReport{}, fmt.Errorf("record bug report status event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.BugReport{}, fmt.Errorf("commit bug report status: %w", err)
	}
	return s.GetBugReport(ctx, projectID, reportID)
}
