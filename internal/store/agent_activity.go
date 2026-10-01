package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/JustSebNL/timekeeper/internal/model"
	"strings"
	"time"
)

var ErrAgentSessionConflict = errors.New("agent identity belongs to a different session; use a unique agent id")

func (s *Store) migrateSupersededNudges(ctx context.Context) error {
	if err := s.ensureColumn(ctx, "pulse_nudges", "superseded_at", "TEXT"); err != nil {
		return err
	}
	var schema string
	if err := s.db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type='index' AND name='pulse_nudges_one_pending_agent_idx'").Scan(&schema); err != nil {
		return err
	}
	if strings.Contains(schema, "superseded_at") {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DROP INDEX pulse_nudges_one_pending_agent_idx"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "CREATE UNIQUE INDEX pulse_nudges_one_pending_agent_idx ON pulse_nudges(agent_id) WHERE status='Pending' AND superseded_at IS NULL"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) migrateAgentWorkState(ctx context.Context) error {
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS agent_work_states (
 agent_id TEXT PRIMARY KEY REFERENCES agent_pulse_progress(agent_id),
 session_id TEXT NOT NULL, runtime TEXT NOT NULL DEFAULT '', project_id TEXT NOT NULL DEFAULT '',
 reported_state TEXT NOT NULL DEFAULT 'idle', reason TEXT NOT NULL DEFAULT '', work_expected INTEGER NOT NULL DEFAULT 0,
 last_contact_at TEXT NOT NULL, state_changed_at TEXT NOT NULL, wait_until TEXT)`,
		`CREATE TABLE IF NOT EXISTS agent_activity_events (
 activity_id INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL REFERENCES agent_work_states(agent_id),
 event_id TEXT NOT NULL, event TEXT NOT NULL, project_id TEXT NOT NULL DEFAULT '', sprint_id TEXT NOT NULL DEFAULT '',
 reason TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, UNIQUE(agent_id,event_id))`,
	} {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

// RecordAgentActivityAt records contact independently from useful progress.
// Explicit session conflicts and event replay must never refresh another worker.
func (s *Store) RecordAgentActivityAt(ctx context.Context, agentID string, in model.AgentActivityInput, at time.Time) (model.AgentWorkState, error) {
	agentID = strings.TrimSpace(agentID)
	in.SessionID = strings.TrimSpace(in.SessionID)
	if agentID == "" || len(agentID) > 256 || in.SessionID == "" || len(in.SessionID) > 256 || len(in.Runtime) > 64 || len(in.Reason) > 1000 || len(in.EventID) > 256 || at.IsZero() {
		return model.AgentWorkState{}, errors.New("valid agent/session identity, bounded metadata, and timestamp are required")
	}
	switch in.Event {
	case "contact", "work", "progress", "wait", "idle", "stop":
	default:
		return model.AgentWorkState{}, errors.New("event must be contact, work, progress, wait, idle, or stop")
	}
	if in.LeaseDurationSeconds < 0 || in.LeaseDurationSeconds > 86400 {
		return model.AgentWorkState{}, errors.New("lease must be at most 86400 seconds")
	}
	if in.Event == "wait" && (in.WaitDurationSeconds < 1 || in.WaitDurationSeconds > 86400 || strings.TrimSpace(in.Reason) == "") {
		return model.AgentWorkState{}, errors.New("waiting requires a reason and a duration from 1 to 86400 seconds")
	}
	if in.GuardianURL != nil {
		if err := validatePulseGuardianURL(*in.GuardianURL); err != nil {
			return model.AgentWorkState{}, err
		}
	}
	at = at.UTC()
	if in.EventID == "" {
		in.EventID = fmt.Sprintf("%d-%s", at.UnixNano(), in.Event)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.AgentWorkState{}, err
	}
	defer tx.Rollback()
	var session string
	err = tx.QueryRowContext(ctx, "SELECT session_id FROM agent_work_states WHERE agent_id=?", agentID).Scan(&session)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return model.AgentWorkState{}, err
	}
	if err == nil && session != in.SessionID {
		return model.AgentWorkState{}, ErrAgentSessionConflict
	}
	lease := in.LeaseDurationSeconds
	if lease == 0 {
		lease = 900
	}
	leaseInsert, err := tx.ExecContext(ctx, `INSERT INTO agent_pulse_progress(agent_id,active_sprint_id,lease_duration_seconds,guardian_url,last_progress_at,updated_at) VALUES (?,'',?,'',?,?) ON CONFLICT(agent_id) DO NOTHING`, agentID, lease, stamp(at), stamp(at))
	if err != nil {
		return model.AgentWorkState{}, err
	}
	newLease, err := leaseInsert.RowsAffected()
	if err != nil {
		return model.AgentWorkState{}, err
	}
	initialState := "idle"
	if newLease == 0 {
		initialState = "working"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_work_states(agent_id,session_id,runtime,project_id,reported_state,work_expected,last_contact_at,state_changed_at) SELECT p.agent_id,?,?,COALESCE(sp.project_id,''),?,?,?,? FROM agent_pulse_progress p LEFT JOIN sprints sp ON sp.sprint_id=p.active_sprint_id WHERE p.agent_id=? ON CONFLICT(agent_id) DO NOTHING`, in.SessionID, in.Runtime, initialState, newLease == 0, stamp(at), stamp(at), agentID); err != nil {
		return model.AgentWorkState{}, err
	}
	var seen, changed, project, sprint, reported, reason, progress string
	var expected bool
	var wait sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT a.last_contact_at,a.state_changed_at,a.project_id,p.active_sprint_id,a.reported_state,a.reason,a.work_expected,a.wait_until,p.last_progress_at,p.lease_duration_seconds FROM agent_work_states a JOIN agent_pulse_progress p USING(agent_id) WHERE a.agent_id=?`, agentID).Scan(&seen, &changed, &project, &sprint, &reported, &reason, &expected, &wait, &progress, &lease)
	if err != nil {
		return model.AgentWorkState{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_activity_events(agent_id,event_id,event,project_id,sprint_id,reason,created_at) VALUES (?,?,?,?,?,?,?)`, agentID, in.EventID, in.Event, in.ProjectID, in.SprintID, in.Reason, stamp(at))
	if err != nil {
		return model.AgentWorkState{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return model.AgentWorkState{}, err
	}
	if count == 0 {
		if err = tx.Commit(); err != nil {
			return model.AgentWorkState{}, err
		}
		return s.AgentWorkStateAt(ctx, agentID, at)
	}
	lastSeen, err := parseTimestamp(seen)
	if err != nil {
		return model.AgentWorkState{}, err
	}
	if at.Before(lastSeen) {
		return model.AgentWorkState{}, errors.New("activity predates the latest agent observation")
	}
	if in.SprintID != "" {
		var sprintProject, status string
		if err = tx.QueryRowContext(ctx, "SELECT project_id,status FROM sprints WHERE sprint_id=?", in.SprintID).Scan(&sprintProject, &status); err != nil {
			return model.AgentWorkState{}, fmt.Errorf("resolve activity Sprint: %w", err)
		}
		if in.ProjectID != "" && in.ProjectID != sprintProject {
			return model.AgentWorkState{}, errors.New("Sprint does not belong to the supplied project")
		}
		if (in.Event == "work" || in.Event == "progress") && status != "Active" {
			return model.AgentWorkState{}, errors.New("work requires an Active Sprint")
		}
		project, sprint = sprintProject, in.SprintID
	} else if in.ProjectID != "" {
		var exists int
		if err = tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id=?", in.ProjectID).Scan(&exists); err != nil {
			return model.AgentWorkState{}, fmt.Errorf("resolve activity project: %w", err)
		}
		if project != "" && project != in.ProjectID && in.Event != "work" {
			return model.AgentWorkState{}, errors.New("activity belongs to another project")
		}
		if project != in.ProjectID {
			sprint = ""
		}
		project = in.ProjectID
	}
	if in.LeaseDurationSeconds > 0 {
		lease = in.LeaseDurationSeconds
	}
	switch in.Event {
	case "work", "progress":
		reported = "working"
		expected = true
		reason = ""
		wait = sql.NullString{}
		progress = stamp(at)
		changed = stamp(at)
	case "wait":
		reported = "waiting"
		reason = in.Reason
		wait = sql.NullString{String: stamp(at.Add(time.Duration(in.WaitDurationSeconds) * time.Second)), Valid: true}
		changed = stamp(at)
	case "idle":
		reported = "idle"
		reason = in.Reason
		wait = sql.NullString{}
		changed = stamp(at)
	case "stop":
		reported = "stopped"
		expected = false
		reason = in.Reason
		wait = sql.NullString{}
		changed = stamp(at)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE agent_work_states SET runtime=CASE WHEN ?<>'' THEN ? ELSE runtime END,project_id=?,reported_state=?,reason=?,work_expected=?,last_contact_at=?,state_changed_at=?,wait_until=? WHERE agent_id=?`, in.Runtime, in.Runtime, project, reported, reason, expected, stamp(at), changed, wait, agentID); err != nil {
		return model.AgentWorkState{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE agent_pulse_progress SET active_sprint_id=?,lease_duration_seconds=?,last_progress_at=?,updated_at=? WHERE agent_id=?`, sprint, lease, progress, stamp(at), agentID); err != nil {
		return model.AgentWorkState{}, err
	}
	if in.Event == "work" || in.Event == "progress" || in.Event == "wait" || in.Event == "stop" {
		if in.GuardianURL != nil {
			if _, err = tx.ExecContext(ctx, "UPDATE agent_pulse_progress SET guardian_url=? WHERE agent_id=?", *in.GuardianURL, agentID); err != nil {
				return model.AgentWorkState{}, err
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE pulse_nudges SET superseded_at=? WHERE agent_id=? AND status='Pending' AND superseded_at IS NULL AND kind IN ('agent_unresponsive','sprint_overdue')`, stamp(at), agentID); err != nil {
			return model.AgentWorkState{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return model.AgentWorkState{}, err
	}
	return s.AgentWorkStateAt(ctx, agentID, at)
}

func (s *Store) AgentWorkStateAt(ctx context.Context, agentID string, at time.Time) (model.AgentWorkState, error) {
	var state model.AgentWorkState
	var contact, progress, changed string
	var wait, sprintStatus sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT a.agent_id,a.session_id,a.runtime,a.project_id,p.active_sprint_id,a.reported_state,a.reason,a.work_expected,a.last_contact_at,p.last_progress_at,a.state_changed_at,a.wait_until,p.lease_duration_seconds,sp.status FROM agent_work_states a JOIN agent_pulse_progress p USING(agent_id) LEFT JOIN sprints sp ON sp.sprint_id=p.active_sprint_id WHERE a.agent_id=?`, agentID).Scan(&state.AgentID, &state.SessionID, &state.Runtime, &state.ProjectID, &state.ActiveSprintID, &state.ReportedState, &state.Reason, &state.WorkExpected, &contact, &progress, &changed, &wait, &state.LeaseDurationSeconds, &sprintStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return state, ErrNotFound
	}
	if err != nil {
		return state, err
	}
	if state.LastContactAt, err = parseTimestamp(contact); err != nil {
		return state, err
	}
	if state.LastProgressAt, err = parseTimestamp(progress); err != nil {
		return state, err
	}
	if state.StateChangedAt, err = parseTimestamp(changed); err != nil {
		return state, err
	}
	if wait.Valid {
		t, e := parseTimestamp(wait.String)
		if e != nil {
			return state, e
		}
		state.WaitUntil = &t
	}
	state.State = state.ReportedState
	switch {
	case state.ReportedState == "stopped":
	case sprintStatus.Valid && sprintStatus.String == "On Hold":
		state.State = "blocked"
	case sprintStatus.Valid && sprintStatus.String != "Active":
		state.State = "finished"
	case !state.WorkExpected:
		state.State = "available"
	case state.ReportedState == "waiting" && state.WaitUntil != nil:
		state.Deadline = state.WaitUntil
		if !at.Before(*state.WaitUntil) {
			state.State = "idle"
		}
	case state.ReportedState == "idle":
		t := state.StateChangedAt
		state.Deadline = &t
	default:
		deadline := state.LastProgressAt.Add(time.Duration(state.LeaseDurationSeconds) * time.Second).Add(graceFor(state.LeaseDurationSeconds))
		state.Deadline = &deadline
		if at.After(deadline) {
			state.State = "idle"
		} else {
			state.State = "working"
		}
	}
	return state, nil
}

func (s *Store) ListAgentWorkStatesAt(ctx context.Context, at time.Time) ([]model.AgentWorkState, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT agent_id FROM agent_work_states ORDER BY agent_id")
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := make([]model.AgentWorkState, 0, len(ids))
	for _, id := range ids {
		state, err := s.AgentWorkStateAt(ctx, id, at)
		if err != nil {
			return nil, err
		}
		result = append(result, state)
	}
	return result, nil
}
