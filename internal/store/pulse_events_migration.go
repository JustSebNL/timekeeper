package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PulseEventType identifies a discrete event in the guardian lifecycle.
type PulseEventType string

const (
	PulseEventCreated  PulseEventType = "pulse_created"
	PulseEventSent     PulseEventType = "pulse_sent"
	PulseEventAffirmed PulseEventType = "agent_affirmed"
	PulseEventResumed  PulseEventType = "agent_resumed"
)

// PulseEvent is a discrete record of a guardian lifecycle event.
type PulseEvent struct {
	EventID   int64
	NudgeID   int64
	EventType PulseEventType
	EventAt   time.Time
	Details   string
}

// migratePulseEvents creates the pulse_events table if it does not exist.
func (s *Store) migratePulseEvents(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS pulse_events (
		event_id INTEGER PRIMARY KEY AUTOINCREMENT,
		nudge_id INTEGER NOT NULL REFERENCES pulse_nudges(nudge_id),
		event_type TEXT NOT NULL CHECK (event_type IN ('pulse_created', 'pulse_sent', 'agent_affirmed', 'agent_resumed')),
		event_at TEXT NOT NULL,
		details TEXT
	)`)
	if err != nil {
		return fmt.Errorf("create pulse_events table: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_pulse_events_nudge ON pulse_events(nudge_id)`)
	if err != nil {
		return fmt.Errorf("create pulse_events nudge index: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_pulse_events_type ON pulse_events(event_type)`)
	if err != nil {
		return fmt.Errorf("create pulse_events type index: %w", err)
	}
	return nil
}

// RecordPulseEvent writes a discrete event to the pulse_events table.
// This is the durable fallback: if the HTTP receiver is down, events remain
// in the table and can be polled later via ListAgentPulseEvents.
func (s *Store) RecordPulseEvent(ctx context.Context, nudgeID int64, eventType PulseEventType, at time.Time, details string) (PulseEvent, error) {
	if nudgeID < 1 {
		return PulseEvent{}, errors.New("nudge_id must be positive")
	}
	if eventType == "" {
		return PulseEvent{}, errors.New("event_type is required")
	}
	if at.IsZero() {
		return PulseEvent{}, errors.New("event_at is required")
	}
	at = at.UTC().Round(0)
	result, err := s.db.ExecContext(ctx, `INSERT INTO pulse_events (nudge_id, event_type, event_at, details) VALUES (?, ?, ?, ?)`,
		nudgeID, string(eventType), stamp(at), details)
	if err != nil {
		return PulseEvent{}, fmt.Errorf("record pulse event: %w", err)
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return PulseEvent{}, fmt.Errorf("read pulse event id: %w", err)
	}
	return PulseEvent{EventID: eventID, NudgeID: nudgeID, EventType: eventType, EventAt: at, Details: details}, nil
}

// ListAgentPulseEvents returns all events for an agent's nudges, ordered by time.
// This is the poll-based fallback: the agent calls this to pick up nudges that
// were stored while the HTTP receiver was unavailable.
func (s *Store) ListAgentPulseEvents(ctx context.Context, agentID string) ([]PulseEvent, error) {
	agentID = agentID
	if agentID == "" {
		return nil, errors.New("agent_id is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT pe.event_id, pe.nudge_id, pe.event_type, pe.event_at, pe.details
		FROM pulse_events pe
		JOIN pulse_nudges pn ON pn.nudge_id = pe.nudge_id
		WHERE pn.agent_id = ? ORDER BY pe.event_at, pe.event_id`, agentID)
	if err != nil {
		return nil, fmt.Errorf("list agent pulse events: %w", err)
	}
	defer rows.Close()
	events := make([]PulseEvent, 0)
	for rows.Next() {
		var event PulseEvent
		var eventAt string
		var details sql.NullString
		if err := rows.Scan(&event.EventID, &event.NudgeID, &event.EventType, &eventAt, &details); err != nil {
			return nil, err
		}
		event.EventAt, err = time.Parse(time.RFC3339Nano, eventAt)
		if err != nil {
			return nil, fmt.Errorf("parse pulse event time: %w", err)
		}
		event.Details = details.String
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent pulse events: %w", err)
	}
	return events, nil
}

// ListPulseEvents returns all events for a nudge, ordered by time.
func (s *Store) ListPulseEvents(ctx context.Context, nudgeID int64) ([]PulseEvent, error) {
	if nudgeID < 1 {
		return nil, errors.New("nudge_id must be positive")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id, nudge_id, event_type, event_at, details FROM pulse_events WHERE nudge_id = ? ORDER BY event_at, event_id`, nudgeID)
	if err != nil {
		return nil, fmt.Errorf("list pulse events: %w", err)
	}
	defer rows.Close()
	events := make([]PulseEvent, 0)
	for rows.Next() {
		var event PulseEvent
		var eventAt string
		var details sql.NullString
		if err := rows.Scan(&event.EventID, &event.NudgeID, &event.EventType, &eventAt, &details); err != nil {
			return nil, err
		}
		event.EventAt, err = time.Parse(time.RFC3339Nano, eventAt)
		if err != nil {
			return nil, fmt.Errorf("parse pulse event time: %w", err)
		}
		event.Details = details.String
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pulse events: %w", err)
	}
	return events, nil
}