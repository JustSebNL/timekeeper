package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/JustSebNL/timekeeper/internal/model"
	"strings"
	"time"
)

func (s *Store) migrateSupervisedLaunches(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS supervised_task_limits(task_id TEXT PRIMARY KEY REFERENCES tasks(task_id),budget_seconds INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS supervised_launches(session_id TEXT PRIMARY KEY,agent_id TEXT NOT NULL,task_id TEXT NOT NULL REFERENCES tasks(task_id),sprint_id TEXT NOT NULL REFERENCES sprints(sprint_id),created_at TEXT NOT NULL);`)
	return err
}

// ReserveSupervisedLaunch consumes one of four task-wide attempts, including launch
// failures. Replacing a Sprint or increasing its estimate cannot reset the allowance.
func (s *Store) ReserveSupervisedLaunch(ctx context.Context, id string, in model.SupervisedLaunchInput) (model.SupervisedLaunch, error) {
	var out model.SupervisedLaunch
	if strings.TrimSpace(in.SessionID) == "" || strings.TrimSpace(in.AgentID) == "" || len(in.SessionID) > 200 || len(in.AgentID) > 200 {
		return out, errors.New("bounded session and agent IDs required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	sp, err := loadSprintState(ctx, tx, id)
	if err != nil {
		return out, err
	}
	if sp.Sprint.Status != "Open" && sp.Sprint.Status != "On Hold" {
		return out, errors.New("supervision requires an Open or On Hold Sprint")
	}
	var existingSprint, existingAgent string
	err = tx.QueryRowContext(ctx, "SELECT sprint_id,agent_id FROM supervised_launches WHERE session_id=?", in.SessionID).Scan(&existingSprint, &existingAgent)
	duplicate := err == nil
	if duplicate {
		return out, errors.New("launch session already consumed; replay cannot restart work")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	var estimate, used, active int64
	err = tx.QueryRowContext(ctx, "SELECT estimated_duration_seconds FROM tasks WHERE task_id=?", sp.Sprint.TaskID).Scan(&estimate)
	if err != nil {
		return out, err
	}
	if estimate <= 0 {
		estimate = sp.Sprint.EstimatedDurationSeconds
	}
	if estimate <= 0 || estimate > 31536000 {
		return out, errors.New("supervised task estimate must be positive and no more than one year")
	}
	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO supervised_task_limits(task_id,budget_seconds) VALUES(?,?)", sp.Sprint.TaskID, 2*estimate+sp.Sprint.BufferDurationSeconds)
	if err != nil {
		return out, err
	}
	var budget int64
	err = tx.QueryRowContext(ctx, "SELECT budget_seconds FROM supervised_task_limits WHERE task_id=?", sp.Sprint.TaskID).Scan(&budget)
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(active_duration_seconds),0),COALESCE(SUM(CASE WHEN status='Active' THEN 1 ELSE 0 END),0) FROM sprints WHERE task_id=?", sp.Sprint.TaskID).Scan(&used, &active)
	if err != nil {
		return out, err
	}
	if active > 0 {
		return out, errors.New("task already has active work; refusing concurrent supervised takeover")
	}
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM supervised_launches WHERE task_id=?", sp.Sprint.TaskID).Scan(&out.Attempts)
	if err != nil {
		return out, err
	}
	out.RemainingSeconds = budget - used
	if out.RemainingSeconds <= 0 || (!duplicate && out.Attempts >= 4) {
		return out, errors.New("supervised task budget exhausted; human reassessment required")
	}
	if !duplicate {
		_, err = tx.ExecContext(ctx, "INSERT INTO supervised_launches(session_id,agent_id,task_id,sprint_id,created_at) VALUES(?,?,?,?,?)", in.SessionID, in.AgentID, sp.Sprint.TaskID, id, stamp(time.Now().UTC()))
		if err != nil {
			return out, err
		}
		out.Attempts++
	}
	now := time.Now().UTC()
	holdSeconds := int64(0)
	if sp.Sprint.Status == "On Hold" {
		if sp.HoldStartedAt == nil {
			return out, errors.New("held Sprint has no hold interval")
		}
		holdSeconds = elapsedSeconds(*sp.HoldStartedAt, now)
		if err := addTimeEntry(ctx, tx, id, "hold", *sp.HoldStartedAt, now, holdSeconds, "supervised resume"); err != nil {
			return out, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE sprints SET status='Active',started_at=COALESCE(started_at,?),active_started_at=?,hold_started_at=NULL,hold_duration_seconds=hold_duration_seconds+?,updated_at=? WHERE sprint_id=?`, stamp(now), stamp(now), holdSeconds, stamp(now), id)
	if err != nil {
		return out, err
	}
	if err := recordProjectEvent(ctx, tx, sp.Sprint.ProjectID, "sprint", id, "supervised_launch", "Supervisor claimed bounded task execution.", now); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
