// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const maxAgentPulseLeaseSeconds int64 = 24 * 60 * 60

func validatePulseGuardianURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > 2048 {
		return errors.New("guardian_url must be at most 2048 characters")
	}
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("guardian_url must be a plain http numeric-loopback URL")
	}
	host := net.ParseIP(endpoint.Hostname())
	if host == nil || !host.IsLoopback() {
		return errors.New("guardian_url must use a numeric loopback host")
	}
	port, err := strconv.ParseUint(endpoint.Port(), 10, 16)
	if err != nil || port == 0 {
		return errors.New("guardian_url must include a valid numeric port")
	}
	return nil
}

// ReportAgentProgress records an agent's explicit material-progress lease. It is
// deliberately not a passive health probe: an agent must renew it while it has
// active work, and the Guardian can escalate after it expires.
func (s *Store) ReportAgentProgress(ctx context.Context, agentID string, input model.AgentPulseProgressInput) (model.AgentPulseProgress, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || len(agentID) > 256 {
		return model.AgentPulseProgress{}, errors.New("agent_id is required and must be at most 256 characters")
	}
	if input.LeaseDurationSeconds < 1 || input.LeaseDurationSeconds > maxAgentPulseLeaseSeconds {
		return model.AgentPulseProgress{}, fmt.Errorf("lease_duration_seconds must be between 1 and %d", maxAgentPulseLeaseSeconds)
	}

	activeSprintID, guardianURL := "", ""
	hasActiveSprint, hasGuardianURL := input.ActiveSprintID != nil, input.GuardianURL != nil
	if hasActiveSprint {
		activeSprintID = strings.TrimSpace(*input.ActiveSprintID)
		if len(activeSprintID) > 100 {
			return model.AgentPulseProgress{}, errors.New("active_sprint_id must be at most 100 characters")
		}
	}
	if hasGuardianURL {
		guardianURL = strings.TrimSpace(*input.GuardianURL)
		if err := validatePulseGuardianURL(guardianURL); err != nil {
			return model.AgentPulseProgress{}, err
		}
	}

	now := time.Now().UTC().Round(0)
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_pulse_progress (
		agent_id, active_sprint_id, lease_duration_seconds, guardian_url, last_progress_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(agent_id) DO UPDATE SET
		active_sprint_id = CASE WHEN ? THEN excluded.active_sprint_id ELSE agent_pulse_progress.active_sprint_id END,
		lease_duration_seconds = excluded.lease_duration_seconds,
		guardian_url = CASE WHEN ? THEN excluded.guardian_url ELSE agent_pulse_progress.guardian_url END,
		last_progress_at = excluded.last_progress_at,
		updated_at = excluded.updated_at`,
		agentID, activeSprintID, input.LeaseDurationSeconds, guardianURL, stamp(now), stamp(now), hasActiveSprint, hasGuardianURL)
	if err != nil {
		return model.AgentPulseProgress{}, fmt.Errorf("record agent progress lease: %w", err)
	}

	var progress model.AgentPulseProgress
	var lastProgressAt, updatedAt string
	if err := s.db.QueryRowContext(ctx, `SELECT agent_id, active_sprint_id, lease_duration_seconds, guardian_url, last_progress_at, updated_at
		FROM agent_pulse_progress WHERE agent_id = ?`, agentID).Scan(&progress.AgentID, &progress.ActiveSprintID,
		&progress.LeaseDurationSeconds, &progress.GuardianURL, &lastProgressAt, &updatedAt); err != nil {
		return model.AgentPulseProgress{}, fmt.Errorf("read recorded agent progress lease: %w", err)
	}
	if progress.LastProgressAt, err = time.Parse(time.RFC3339Nano, lastProgressAt); err != nil {
		return model.AgentPulseProgress{}, fmt.Errorf("parse recorded agent progress timestamp: %w", err)
	}
	if progress.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return model.AgentPulseProgress{}, fmt.Errorf("parse recorded agent progress update timestamp: %w", err)
	}
	return progress, nil
}

// EvaluatePulseGuardianAt creates one durable Pending nudge per expired agent
// lease. It is safe to call repeatedly: a Pending nudge is never duplicated.
// A caller outside the working agent's main loop should invoke it periodically.
func (s *Store) EvaluatePulseGuardianAt(ctx context.Context, at time.Time) ([]model.PulseNudge, error) {
	if at.IsZero() {
		return nil, errors.New("guardian evaluation time is required")
	}
	at = at.UTC().Round(0)
	rows, err := s.db.QueryContext(ctx, `SELECT agent_id, active_sprint_id, lease_duration_seconds, last_progress_at
		FROM agent_pulse_progress ORDER BY agent_id`)
	if err != nil {
		return nil, fmt.Errorf("read agent progress leases: %w", err)
	}
	defer rows.Close()

	created := make([]model.PulseNudge, 0)
	for rows.Next() {
		var agentID, activeSprintID, lastProgress string
		var leaseSeconds int64
		if err := rows.Scan(&agentID, &activeSprintID, &leaseSeconds, &lastProgress); err != nil {
			return nil, fmt.Errorf("scan agent progress lease: %w", err)
		}
		progressAt, err := time.Parse(time.RFC3339Nano, lastProgress)
		if err != nil {
			return nil, fmt.Errorf("parse agent progress lease: %w", err)
		}
		detectedAfter := elapsedSeconds(progressAt, at)
		if detectedAfter <= leaseSeconds {
			continue
		}

		result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO pulse_nudges (
			agent_id, active_sprint_id, kind, status, detected_after_seconds, created_at
		) VALUES (?, ?, 'agent_unresponsive', 'Pending', ?, ?)`, agentID, activeSprintID, detectedAfter, stamp(at))
		if err != nil {
			return nil, fmt.Errorf("create pulse nudge: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("read pulse nudge result: %w", err)
		}
		if affected == 0 {
			continue
		}
		nudgeID, err := result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("read pulse nudge id: %w", err)
		}
		created = append(created, model.PulseNudge{
			NudgeID:              nudgeID,
			AgentID:              agentID,
			ActiveSprintID:       activeSprintID,
			Kind:                 "agent_unresponsive",
			Status:               "Pending",
			DetectedAfterSeconds: detectedAfter,
			CreatedAt:            at,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent progress leases: %w", err)
	}
	return created, nil
}

// ListPendingPulseNudges returns unacknowledged escalations for exactly one agent.
func (s *Store) ListPendingPulseNudges(ctx context.Context, agentID string) ([]model.PulseNudge, error) {
	return s.listPulseNudges(ctx, agentID, true)
}

// ListPulseNudgeHistory returns durable pending and acknowledged nudge evidence
// for exactly one agent. It is distinct from the pending work queue so callers
// can audit recovery without accidentally re-delivering acknowledged work.
func (s *Store) ListPulseNudgeHistory(ctx context.Context, agentID string) ([]model.PulseNudge, error) {
	return s.listPulseNudges(ctx, agentID, false)
}

func (s *Store) listPulseNudges(ctx context.Context, agentID string, pendingOnly bool) ([]model.PulseNudge, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || len(agentID) > 256 {
		return nil, errors.New("agent_id is required and must be at most 256 characters")
	}
	query := `SELECT nudge_id, agent_id, active_sprint_id, kind, status,
		detected_after_seconds, delivery_attempts, last_delivery_at, delivered_at, created_at, acknowledged_at, acknowledged_by
		FROM pulse_nudges WHERE agent_id = ?`
	if pendingOnly {
		query += " AND status = 'Pending'"
	}
	query += " ORDER BY created_at, nudge_id"
	rows, err := s.db.QueryContext(ctx, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("list pulse nudges: %w", err)
	}
	defer rows.Close()
	nudges := make([]model.PulseNudge, 0)
	for rows.Next() {
		nudge, err := scanPulseNudge(rows)
		if err != nil {
			return nil, err
		}
		nudges = append(nudges, nudge)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pulse nudges: %w", err)
	}
	return nudges, nil
}

// PulseNudgeDelivery pairs an unacknowledged nudge with its owner's explicitly
// configured local Guardian callback. It is an internal delivery contract, not a
// remote webhook facility.
type PulseNudgeDelivery struct {
	Nudge       model.PulseNudge
	GuardianURL string
}

// ListPendingPulseNudgeDeliveries returns only callbacks that were explicitly
// registered on numeric loopback and not yet accepted by that Guardian. The URL
// is validated before it can be stored; failed callbacks remain eligible for a
// later retry, while confirmed acceptance is not spammed before acknowledgement.
func (s *Store) ListPendingPulseNudgeDeliveries(ctx context.Context) ([]PulseNudgeDelivery, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT n.nudge_id, n.agent_id, n.active_sprint_id, n.kind, n.status,
		n.detected_after_seconds, n.delivery_attempts, n.last_delivery_at, n.delivered_at, n.created_at, n.acknowledged_at, n.acknowledged_by, p.guardian_url
		FROM pulse_nudges n JOIN agent_pulse_progress p ON p.agent_id = n.agent_id
		WHERE n.status = 'Pending' AND n.delivered_at IS NULL AND p.guardian_url <> '' ORDER BY n.created_at, n.nudge_id`)
	if err != nil {
		return nil, fmt.Errorf("list pending Pulse Guardian deliveries: %w", err)
	}
	defer rows.Close()
	deliveries := make([]PulseNudgeDelivery, 0)
	for rows.Next() {
		var delivery PulseNudgeDelivery
		var created string
		var lastDeliveryAt, deliveredAt, acknowledgedAt, acknowledgedBy sql.NullString
		if err := rows.Scan(&delivery.Nudge.NudgeID, &delivery.Nudge.AgentID, &delivery.Nudge.ActiveSprintID,
			&delivery.Nudge.Kind, &delivery.Nudge.Status, &delivery.Nudge.DetectedAfterSeconds,
			&delivery.Nudge.DeliveryAttempts, &lastDeliveryAt, &deliveredAt, &created,
			&acknowledgedAt, &acknowledgedBy, &delivery.GuardianURL); err != nil {
			return nil, fmt.Errorf("scan pending Pulse Guardian delivery: %w", err)
		}
		if delivery.Nudge.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("parse pending Pulse Guardian delivery created_at: %w", err)
		}
		if delivery.Nudge.LastDeliveryAt, err = parseOptionalTime(lastDeliveryAt); err != nil {
			return nil, fmt.Errorf("parse pending Pulse Guardian delivery last_delivery_at: %w", err)
		}
		if delivery.Nudge.DeliveredAt, err = parseOptionalTime(deliveredAt); err != nil {
			return nil, fmt.Errorf("parse pending Pulse Guardian delivery delivered_at: %w", err)
		}
		if acknowledgedAt.Valid {
			at, err := time.Parse(time.RFC3339Nano, acknowledgedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse pending Pulse Guardian delivery acknowledged_at: %w", err)
			}
			delivery.Nudge.AcknowledgedAt = &at
		}
		if acknowledgedBy.Valid {
			delivery.Nudge.AcknowledgedBy = acknowledgedBy.String
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending Pulse Guardian deliveries: %w", err)
	}
	return deliveries, nil
}

// RecordPulseNudgeDeliveryAttempt keeps delivery evidence separate from the
// Project history. A failed callback remains Pending and is retried by the next
// Guardian tick; a callback may acknowledge immediately, so this method also
// preserves the final delivery evidence for an already Acknowledged nudge.
func (s *Store) RecordPulseNudgeDeliveryAttempt(ctx context.Context, nudgeID int64, delivered bool, at time.Time) error {
	if nudgeID < 1 {
		return errors.New("nudge_id must be positive")
	}
	if at.IsZero() {
		return errors.New("delivery attempt time is required")
	}
	at = at.UTC().Round(0)
	result, err := s.db.ExecContext(ctx, `UPDATE pulse_nudges SET delivery_attempts = delivery_attempts + 1,
		last_delivery_at = ?, delivered_at = CASE WHEN ? THEN COALESCE(delivered_at, ?) ELSE delivered_at END
		WHERE nudge_id = ? AND status IN ('Pending', 'Acknowledged')`, stamp(at), delivered, stamp(at), nudgeID)
	if err != nil {
		return fmt.Errorf("record Pulse Guardian delivery attempt: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read Pulse Guardian delivery attempt result: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: pending pulse nudge %d", ErrNotFound, nudgeID)
	}
	return nil
}

// AcknowledgePulseNudge proves that the named agent received the escalation and
// renews its progress lease. Acknowledgement is idempotent for its owner.
func (s *Store) AcknowledgePulseNudge(ctx context.Context, nudgeID int64, agentID string) (model.PulseNudge, error) {
	agentID = strings.TrimSpace(agentID)
	if nudgeID < 1 {
		return model.PulseNudge{}, errors.New("nudge_id must be positive")
	}
	if agentID == "" || len(agentID) > 256 {
		return model.PulseNudge{}, errors.New("agent_id is required and must be at most 256 characters")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.PulseNudge{}, fmt.Errorf("begin pulse nudge acknowledgement: %w", err)
	}
	defer tx.Rollback()

	nudge, err := scanPulseNudge(tx.QueryRowContext(ctx, `SELECT nudge_id, agent_id, active_sprint_id, kind, status,
		detected_after_seconds, delivery_attempts, last_delivery_at, delivered_at, created_at, acknowledged_at, acknowledged_by
		FROM pulse_nudges WHERE nudge_id = ?`, nudgeID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.PulseNudge{}, fmt.Errorf("%w: pulse nudge %d", ErrNotFound, nudgeID)
	}
	if err != nil {
		return model.PulseNudge{}, err
	}
	if nudge.AgentID != agentID {
		return model.PulseNudge{}, errors.New("pulse nudge belongs to a different agent")
	}
	if nudge.Status == "Acknowledged" {
		return nudge, tx.Commit()
	}
	if nudge.Status != "Pending" {
		return model.PulseNudge{}, fmt.Errorf("unsupported pulse nudge status %q", nudge.Status)
	}

	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, `UPDATE pulse_nudges SET status = 'Acknowledged', acknowledged_at = ?, acknowledged_by = ? WHERE nudge_id = ?`, stamp(now), agentID, nudgeID); err != nil {
		return model.PulseNudge{}, fmt.Errorf("acknowledge pulse nudge: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_pulse_progress SET last_progress_at = ?, updated_at = ? WHERE agent_id = ?`, stamp(now), stamp(now), agentID); err != nil {
		return model.PulseNudge{}, fmt.Errorf("renew agent progress lease: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.PulseNudge{}, fmt.Errorf("commit pulse nudge acknowledgement: %w", err)
	}
	nudge.Status = "Acknowledged"
	nudge.AcknowledgedAt = &now
	nudge.AcknowledgedBy = agentID
	return nudge, nil
}

// RegisteredGuardianURLs returns the loopback recovery callbacks that agents
// have explicitly registered. A Guardian tick delivers to these; an empty
// result means detection runs but no recovery action can be taken locally.
func (s *Store) RegisteredGuardianURLs(ctx context.Context) ([]model.RegisteredGuardian, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT agent_id, guardian_url FROM agent_pulse_progress WHERE guardian_url <> '' ORDER BY agent_id`)
	if err != nil {
		return nil, fmt.Errorf("list registered guardian callbacks: %w", err)
	}
	defer rows.Close()
	registered := make([]model.RegisteredGuardian, 0)
	for rows.Next() {
		var item model.RegisteredGuardian
		if err := rows.Scan(&item.AgentID, &item.GuardianURL); err != nil {
			return nil, fmt.Errorf("scan registered guardian callback: %w", err)
		}
		registered = append(registered, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate registered guardian callbacks: %w", err)
	}
	return registered, nil
}

func scanPulseNudge(scanner interface{ Scan(...any) error }) (model.PulseNudge, error) {
	var nudge model.PulseNudge
	var created string
	var lastDeliveryAt, deliveredAt, acknowledgedAt, acknowledgedBy sql.NullString
	if err := scanner.Scan(&nudge.NudgeID, &nudge.AgentID, &nudge.ActiveSprintID, &nudge.Kind, &nudge.Status,
		&nudge.DetectedAfterSeconds, &nudge.DeliveryAttempts, &lastDeliveryAt, &deliveredAt, &created,
		&acknowledgedAt, &acknowledgedBy); err != nil {
		return model.PulseNudge{}, err
	}
	var err error
	if nudge.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return model.PulseNudge{}, fmt.Errorf("parse pulse nudge created_at: %w", err)
	}
	if nudge.LastDeliveryAt, err = parseOptionalTime(lastDeliveryAt); err != nil {
		return model.PulseNudge{}, fmt.Errorf("parse pulse nudge last_delivery_at: %w", err)
	}
	if nudge.DeliveredAt, err = parseOptionalTime(deliveredAt); err != nil {
		return model.PulseNudge{}, fmt.Errorf("parse pulse nudge delivered_at: %w", err)
	}
	if acknowledgedAt.Valid {
		at, err := time.Parse(time.RFC3339Nano, acknowledgedAt.String)
		if err != nil {
			return model.PulseNudge{}, fmt.Errorf("parse pulse nudge acknowledged_at: %w", err)
		}
		nudge.AcknowledgedAt = &at
	}
	if acknowledgedBy.Valid {
		nudge.AcknowledgedBy = acknowledgedBy.String
	}
	return nudge, nil
}
