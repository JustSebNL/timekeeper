// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// RecordUsageSnapshot accepts cumulative counters for one agent turn. A
// repeated (project, agent, session, turn) is a no-op; a new turn must advance
// monotonically so the returned delta is trustworthy.
func (s *Store) RecordUsageSnapshot(ctx context.Context, projectID string, input model.UsageSnapshotInput) (model.UsageSnapshotResult, error) {
	projectID = strings.TrimSpace(projectID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.Model = strings.TrimSpace(input.Model)
	input.SprintID = strings.TrimSpace(input.SprintID)
	if projectID == "" {
		return model.UsageSnapshotResult{}, errors.New("project_id is required")
	}
	if input.SessionID == "" {
		return model.UsageSnapshotResult{}, errors.New("session_id is required")
	}
	if input.AgentID == "" {
		return model.UsageSnapshotResult{}, errors.New("agent_id is required")
	}
	if input.TurnSeq < 0 {
		return model.UsageSnapshotResult{}, errors.New("turn_seq must be non-negative")
	}
	if err := validateUsageCounters(input); err != nil {
		return model.UsageSnapshotResult{}, err
	}
	capturedAt := input.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC().Round(0)
	} else {
		capturedAt = capturedAt.UTC().Round(0)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.UsageSnapshotResult{}, fmt.Errorf("begin usage snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.UsageSnapshotResult{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.UsageSnapshotResult{}, fmt.Errorf("validate usage project: %w", err)
	}
	if input.SprintID != "" {
		var sprintProject string
		err := tx.QueryRowContext(ctx, "SELECT project_id FROM sprints WHERE sprint_id = ?", input.SprintID).Scan(&sprintProject)
		if errors.Is(err, sql.ErrNoRows) || sprintProject != projectID {
			return model.UsageSnapshotResult{}, errors.New("sprint_id must belong to the usage project")
		}
		if err != nil {
			return model.UsageSnapshotResult{}, fmt.Errorf("validate usage Sprint: %w", err)
		}
	}

	row, err := readUsageSession(ctx, tx, projectID, input.AgentID, input.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		result, err := createUsageSession(ctx, tx, projectID, input, capturedAt)
		if err != nil {
			return model.UsageSnapshotResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return model.UsageSnapshotResult{}, fmt.Errorf("commit first usage snapshot: %w", err)
		}
		return result, nil
	}
	if err != nil {
		return model.UsageSnapshotResult{}, fmt.Errorf("read usage session: %w", err)
	}
	if input.SprintID != "" && row.sprintID.Valid && row.sprintID.String != input.SprintID {
		return model.UsageSnapshotResult{}, errors.New("sprint_id cannot change within a usage session")
	}

	var snapshotID int64
	if err := tx.QueryRowContext(ctx, "SELECT snapshot_id FROM agent_usage_snapshots WHERE usage_id = ? AND turn_seq = ?", row.usageID, input.TurnSeq).Scan(&snapshotID); err == nil {
		session, err := usageSessionFromRow(ctx, tx, row.usageID)
		if err != nil {
			return model.UsageSnapshotResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return model.UsageSnapshotResult{}, fmt.Errorf("commit duplicate usage snapshot: %w", err)
		}
		return model.UsageSnapshotResult{Session: session, Duplicate: true}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return model.UsageSnapshotResult{}, fmt.Errorf("check usage snapshot: %w", err)
	}
	if input.TurnSeq <= row.turnSeq {
		return model.UsageSnapshotResult{}, errors.New("usage snapshot turn_seq must increase; start a new session if provider counters reset")
	}
	if err := validateCumulativeIncrease(row, input); err != nil {
		return model.UsageSnapshotResult{}, err
	}

	delta := model.UsageDelta{
		InputTokens:         input.InputTokens - row.inputTokens,
		OutputTokens:        input.OutputTokens - row.outputTokens,
		CacheCreationTokens: input.CacheCreationTokens - row.cacheCreationTokens,
		CacheReadTokens:     input.CacheReadTokens - row.cacheReadTokens,
		Messages:            input.Messages - row.messages,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_usage_snapshots (
		usage_id, turn_seq, input_tokens, output_tokens, cache_creation_tokens,
		cache_read_tokens, context_used, context_size, messages, captured_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, row.usageID, input.TurnSeq, input.InputTokens, input.OutputTokens,
		input.CacheCreationTokens, input.CacheReadTokens, optionalInt(input.ContextUsed), optionalInt(input.ContextSize), input.Messages, stamp(capturedAt)); err != nil {
		return model.UsageSnapshotResult{}, fmt.Errorf("insert usage snapshot: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_usage_sessions SET
		model = CASE WHEN ? <> '' THEN ? ELSE model END,
		sprint_id = CASE WHEN ? <> '' THEN ? ELSE sprint_id END,
		turn_seq = ?, input_tokens = ?, output_tokens = ?, cache_creation_tokens = ?,
		cache_read_tokens = ?, context_used = CASE WHEN ? IS NULL THEN context_used ELSE ? END,
		context_size = CASE WHEN ? IS NULL THEN context_size ELSE ? END,
		messages = ?, last_activity_at = ?, updated_at = ? WHERE usage_id = ?`,
		input.Model, input.Model, input.SprintID, input.SprintID, input.TurnSeq, input.InputTokens, input.OutputTokens,
		input.CacheCreationTokens, input.CacheReadTokens, optionalInt(input.ContextUsed), optionalInt(input.ContextUsed),
		optionalInt(input.ContextSize), optionalInt(input.ContextSize), input.Messages, stamp(capturedAt), stamp(capturedAt), row.usageID); err != nil {
		return model.UsageSnapshotResult{}, fmt.Errorf("update usage session: %w", err)
	}
	session, err := usageSessionFromRow(ctx, tx, row.usageID)
	if err != nil {
		return model.UsageSnapshotResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.UsageSnapshotResult{}, fmt.Errorf("commit usage snapshot: %w", err)
	}
	return model.UsageSnapshotResult{Session: session, Delta: delta}, nil
}

// ProjectUsageSummary returns the latest cumulative usage for each agent session
// in a Project. It is read-only and intentionally does not calculate cost until
// TimeKeeper has a trusted local price source.
func (s *Store) ProjectUsageSummary(ctx context.Context, projectID string) (model.ProjectUsageSummary, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ProjectUsageSummary{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.ProjectUsageSummary{}, fmt.Errorf("validate usage summary project: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT usage_id, session_id, project_id, sprint_id, agent_id, model,
		turn_seq, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
		context_used, context_size, messages, last_activity_at, created_at, updated_at
		FROM agent_usage_sessions WHERE project_id = ? ORDER BY updated_at DESC, usage_id DESC`, projectID)
	if err != nil {
		return model.ProjectUsageSummary{}, fmt.Errorf("list project usage: %w", err)
	}
	defer rows.Close()

	summary := model.ProjectUsageSummary{ProjectID: projectID, Sessions: make([]model.UsageSession, 0)}
	for rows.Next() {
		row, err := scanUsageSession(rows)
		if err != nil {
			return model.ProjectUsageSummary{}, err
		}
		session := row.toModel()
		summary.Sessions = append(summary.Sessions, session)
		summary.Totals.InputTokens += session.InputTokens
		summary.Totals.OutputTokens += session.OutputTokens
		summary.Totals.CacheCreationTokens += session.CacheCreationTokens
		summary.Totals.CacheReadTokens += session.CacheReadTokens
		summary.Totals.Messages += session.Messages
		summary.Totals.SessionCount++
	}
	if err := rows.Err(); err != nil {
		return model.ProjectUsageSummary{}, fmt.Errorf("read project usage rows: %w", err)
	}
	days, err := s.projectUsageDays(ctx, projectID)
	if err != nil {
		return model.ProjectUsageSummary{}, err
	}
	summary.Days = days
	return summary, nil
}

func (s *Store) projectUsageDays(ctx context.Context, projectID string) ([]model.UsageDay, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.usage_id, snap.turn_seq, snap.input_tokens, snap.output_tokens,
		snap.cache_creation_tokens, snap.cache_read_tokens, snap.messages, snap.captured_at
		FROM agent_usage_snapshots snap JOIN agent_usage_sessions u ON u.usage_id = snap.usage_id
		WHERE u.project_id = ? ORDER BY u.usage_id, snap.turn_seq`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project usage timeline: %w", err)
	}
	defer rows.Close()
	type counters struct{ input, output, cacheCreation, cacheRead, messages int64 }
	previous := make(map[int64]counters)
	days := make(map[string]*model.UsageDay)
	seenSessions := make(map[string]map[int64]bool)
	for rows.Next() {
		var usageID, turnSeq, input, output, cacheCreation, cacheRead, messages int64
		var captured string
		if err := rows.Scan(&usageID, &turnSeq, &input, &output, &cacheCreation, &cacheRead, &messages, &captured); err != nil {
			return nil, fmt.Errorf("scan project usage timeline: %w", err)
		}
		stampValue, err := time.Parse(time.RFC3339Nano, captured)
		if err != nil {
			return nil, fmt.Errorf("parse usage timeline timestamp: %w", err)
		}
		prior := previous[usageID]
		day := stampValue.UTC().Format("2006-01-02")
		bucket := days[day]
		if bucket == nil {
			bucket = &model.UsageDay{Date: day}
			days[day] = bucket
		}
		bucket.InputTokens += input - prior.input
		bucket.OutputTokens += output - prior.output
		bucket.CacheCreationTokens += cacheCreation - prior.cacheCreation
		bucket.CacheReadTokens += cacheRead - prior.cacheRead
		bucket.Messages += messages - prior.messages
		if seenSessions[day] == nil {
			seenSessions[day] = make(map[int64]bool)
		}
		if !seenSessions[day][usageID] {
			seenSessions[day][usageID] = true
			bucket.SessionCount++
		}
		previous[usageID] = counters{input: input, output: output, cacheCreation: cacheCreation, cacheRead: cacheRead, messages: messages}
		_ = turnSeq
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read project usage timeline rows: %w", err)
	}
	result := make([]model.UsageDay, 0, len(days))
	for _, day := range days {
		result = append(result, *day)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date < result[j].Date })
	return result, nil
}

type usageSessionRow struct {
	usageID, turnSeq, inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens, messages int64
	projectID, sessionID, agentID, model                                                        string
	sprintID, contextUsed, contextSize                                                          sql.NullString
	lastActivityAt, createdAt, updatedAt                                                        string
}

func readUsageSession(ctx context.Context, tx *sql.Tx, projectID, agentID, sessionID string) (usageSessionRow, error) {
	return scanUsageSession(tx.QueryRowContext(ctx, `SELECT usage_id, session_id, project_id, sprint_id, agent_id, model,
		turn_seq, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
		context_used, context_size, messages, last_activity_at, created_at, updated_at
		FROM agent_usage_sessions WHERE project_id = ? AND agent_id = ? AND session_id = ?`, projectID, agentID, sessionID))
}

func usageSessionFromRow(ctx context.Context, tx *sql.Tx, usageID int64) (model.UsageSession, error) {
	row, err := scanUsageSession(tx.QueryRowContext(ctx, `SELECT usage_id, session_id, project_id, sprint_id, agent_id, model,
		turn_seq, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
		context_used, context_size, messages, last_activity_at, created_at, updated_at
		FROM agent_usage_sessions WHERE usage_id = ?`, usageID))
	if err != nil {
		return model.UsageSession{}, fmt.Errorf("read updated usage session: %w", err)
	}
	return row.toModel(), nil
}

func scanUsageSession(scanner interface{ Scan(...any) error }) (usageSessionRow, error) {
	var row usageSessionRow
	if err := scanner.Scan(&row.usageID, &row.sessionID, &row.projectID, &row.sprintID, &row.agentID, &row.model,
		&row.turnSeq, &row.inputTokens, &row.outputTokens, &row.cacheCreationTokens, &row.cacheReadTokens,
		&row.contextUsed, &row.contextSize, &row.messages, &row.lastActivityAt, &row.createdAt, &row.updatedAt); err != nil {
		return usageSessionRow{}, err
	}
	return row, nil
}

func (row usageSessionRow) toModel() model.UsageSession {
	return model.UsageSession{
		SessionID: row.sessionID, ProjectID: row.projectID, SprintID: row.sprintID.String, AgentID: row.agentID, Model: row.model,
		TurnSeq: row.turnSeq, InputTokens: row.inputTokens, OutputTokens: row.outputTokens,
		CacheCreationTokens: row.cacheCreationTokens, CacheReadTokens: row.cacheReadTokens,
		ContextUsed: parseOptionalInt(row.contextUsed), ContextSize: parseOptionalInt(row.contextSize), Messages: row.messages,
		LastActivityAt: parseTime(row.lastActivityAt), CreatedAt: parseTime(row.createdAt), UpdatedAt: parseTime(row.updatedAt),
	}
}

func createUsageSession(ctx context.Context, tx *sql.Tx, projectID string, input model.UsageSnapshotInput, capturedAt time.Time) (model.UsageSnapshotResult, error) {
	result, err := tx.ExecContext(ctx, `INSERT INTO agent_usage_sessions (
		project_id, session_id, agent_id, model, sprint_id, turn_seq, input_tokens, output_tokens,
		cache_creation_tokens, cache_read_tokens, context_used, context_size, messages,
		last_activity_at, created_at, updated_at) VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		projectID, input.SessionID, input.AgentID, input.Model, input.SprintID, input.TurnSeq, input.InputTokens, input.OutputTokens,
		input.CacheCreationTokens, input.CacheReadTokens, optionalInt(input.ContextUsed), optionalInt(input.ContextSize), input.Messages,
		stamp(capturedAt), stamp(capturedAt), stamp(capturedAt))
	if err != nil {
		return model.UsageSnapshotResult{}, fmt.Errorf("insert usage session: %w", err)
	}
	usageID, err := result.LastInsertId()
	if err != nil {
		return model.UsageSnapshotResult{}, fmt.Errorf("read usage session ID: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_usage_snapshots (
		usage_id, turn_seq, input_tokens, output_tokens, cache_creation_tokens,
		cache_read_tokens, context_used, context_size, messages, captured_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, usageID, input.TurnSeq, input.InputTokens, input.OutputTokens,
		input.CacheCreationTokens, input.CacheReadTokens, optionalInt(input.ContextUsed), optionalInt(input.ContextSize), input.Messages, stamp(capturedAt)); err != nil {
		return model.UsageSnapshotResult{}, fmt.Errorf("insert first usage snapshot: %w", err)
	}
	session, err := usageSessionFromRow(ctx, tx, usageID)
	if err != nil {
		return model.UsageSnapshotResult{}, err
	}
	return model.UsageSnapshotResult{Session: session, Delta: model.UsageDelta{
		InputTokens: input.InputTokens, OutputTokens: input.OutputTokens,
		CacheCreationTokens: input.CacheCreationTokens, CacheReadTokens: input.CacheReadTokens,
		Messages: input.Messages,
	}}, nil
}

func validateUsageCounters(input model.UsageSnapshotInput) error {
	values := map[string]int64{
		"input_tokens": input.InputTokens, "output_tokens": input.OutputTokens,
		"cache_creation_tokens": input.CacheCreationTokens, "cache_read_tokens": input.CacheReadTokens, "messages": input.Messages,
	}
	for name, value := range values {
		if value < 0 {
			return fmt.Errorf("%s must be non-negative", name)
		}
	}
	if input.ContextUsed != nil && *input.ContextUsed < 0 {
		return errors.New("context_used must be non-negative")
	}
	if input.ContextSize != nil && *input.ContextSize < 0 {
		return errors.New("context_size must be non-negative")
	}
	if input.ContextUsed != nil && input.ContextSize != nil && *input.ContextUsed > *input.ContextSize {
		return errors.New("context_used cannot exceed context_size")
	}
	return nil
}

func validateCumulativeIncrease(row usageSessionRow, input model.UsageSnapshotInput) error {
	checks := []struct {
		name              string
		previous, current int64
	}{
		{"input_tokens", row.inputTokens, input.InputTokens}, {"output_tokens", row.outputTokens, input.OutputTokens},
		{"cache_creation_tokens", row.cacheCreationTokens, input.CacheCreationTokens}, {"cache_read_tokens", row.cacheReadTokens, input.CacheReadTokens},
		{"messages", row.messages, input.Messages},
	}
	for _, check := range checks {
		if check.current < check.previous {
			return fmt.Errorf("%s cannot decrease within a usage session; start a new session if provider counters reset", check.name)
		}
	}
	return nil
}

func optionalInt(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func parseOptionalInt(value sql.NullString) *int64 {
	if !value.Valid || value.String == "" {
		return nil
	}
	var parsed int64
	if _, err := fmt.Sscan(value.String, &parsed); err != nil {
		return nil
	}
	return &parsed
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
