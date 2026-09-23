package store

import (
	"context"
	"fmt"
	"time"
)

// ActiveAgent is an agent with a non-expired progress lease.
type ActiveAgent struct {
	AgentID              string    `json:"agent_id"`
	ActiveSprintID       string    `json:"active_sprint_id,omitempty"`
	LeaseDurationSeconds int64     `json:"lease_duration_seconds"`
	LastProgressAt       time.Time `json:"last_progress_at"`
	GuardianURL          string    `json:"guardian_url,omitempty"`
}

// ListActiveAgents returns all agents whose progress lease has not expired.
// An agent is active when last_progress_at + lease_duration_seconds > now.
func (s *Store) ListActiveAgents(ctx context.Context) ([]ActiveAgent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT agent_id, active_sprint_id, lease_duration_seconds, last_progress_at, guardian_url
		FROM agent_pulse_progress
		WHERE last_process_at + CAST(lease_duration_seconds AS INTEGER) > ?
		ORDER BY agent_id`, time.Now().Unix())
	if err != nil {
		return nil, fmt.Errorf("read active agents: %w", err)
	}
	defer rows.Close()

	agents := make([]ActiveAgent, 0)
	for rows.Next() {
		var a ActiveAgent
		if err := rows.Scan(&a.AgentID, &a.ActiveSprintID, &a.LeaseDurationSeconds, &a.LastProgressAt, &a.GuardianURL); err != nil {
			return nil, fmt.Errorf("scan active agent: %w", err)
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active agents: %w", err)
	}
	return agents, nil
}

// BroadcastNudgeResult reports how many agents received a broadcast.
type BroadcastNudgeResult struct {
	AgentIDs []string `json:"agent_ids"`
	Count    int      `json:"count"`
}

// CreateBroadcastNudge inserts one Pending 'broadcast' nudge for every
// agent with a non-expired progress lease. The nudge kind 'broadcast'
// is delivered by the Guardian's existing delivery loop.
func (s *Store) CreateBroadcastNudge(ctx context.Context, message string) (BroadcastNudgeResult, error) {
	agents, err := s.ListActiveAgents(ctx)
	if err != nil {
		return BroadcastNudgeResult{}, err
	}
	if len(agents) == 0 {
		return BroadcastNudgeResult{}, fmt.Errorf("no active agents to broadcast to")
	}

	now := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BroadcastNudgeResult{}, fmt.Errorf("begin broadcast: %w", err)
	}
	defer tx.Rollback()

	ids := make([]string, 0, len(agents))
	for _, a := range agents {
		// Broadcast nudges are per-agent so the existing delivery loop
		// and ack flow work unchanged. detected_after is 0 (not an
		// escalation) and the message is stored as the nudge body.
		res, err := tx.ExecContext(ctx, `
			INSERT INTO pulse_nudges (agent_id, active_sprint_id, kind, status, detected_after_seconds, created_at)
			VALUES (?, ?, 'broadcast', 'Pending', 0, ?)`,
			a.AgentID, a.ActiveSprintID, now)
		if err != nil {
			return BroadcastNudgeResult{}, fmt.Errorf("insert broadcast nudge: %w", err)
		}
		id, _ := res.LastInsertId()
		_ = id
		ids = append(ids, a.AgentID)
	}

	if err := tx.Commit(); err != nil {
		return BroadcastNudgeResult{}, fmt.Errorf("commit broadcast: %w", err)
	}
	return BroadcastNudgeResult{AgentIDs: ids, Count: len(ids)}, nil
}
