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

type ClaimConflictError struct{ ItemID, AgentKey string }

func (e *ClaimConflictError) Error() string {
	return fmt.Sprintf("item %s has execution claimant FASE-%s", e.ItemID, e.AgentKey)
}
func lockFASEWrite(ctx context.Context, tx *sql.Tx) error {
	_, e := tx.ExecContext(ctx, "UPDATE fase_registry_guard SET id=id WHERE id=1")
	return e
}

func checkFASEClaim(ctx context.Context, tx *sql.Tx, item string) error {
	var owner string
	e := tx.QueryRowContext(ctx, "SELECT agent_key FROM fase_execution_claims WHERE item_id=?", item).Scan(&owner)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	actor, ok := model.FASEActorFromContext(ctx)
	if !ok || actor.AgentKey != owner {
		return &ClaimConflictError{item, owner}
	}
	return nil
}
func claimFASE(ctx context.Context, tx *sql.Tx, item, project string, now time.Time) error {
	if e := checkFASEClaim(ctx, tx, item); e != nil {
		return e
	}
	actor, ok := model.FASEActorFromContext(ctx)
	if !ok {
		return nil
	} // legacy unclaimed work remains usable
	alias := faseAlias(actor.AgentKey, now)
	result, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO fase_execution_claims(item_id,project_id,agent_key,fase_id,claimed_at) VALUES(?,?,?,?,?)", item, project, actor.AgentKey, alias, stamp(now))
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n == 1 {
		return recordProjectEvent(ctx, tx, project, "execution_claim", item, "execution_claim_acquired", "Execution claimed; collaborative contributions remain permitted.", now)
	}
	return nil
}
func claimFASESprint(ctx context.Context, tx *sql.Tx, sp model.Sprint, now time.Time) error {
	if e := claimFASE(ctx, tx, sp.TaskID, sp.ProjectID, now); e != nil {
		return e
	}
	return claimFASE(ctx, tx, sp.SprintID, sp.ProjectID, now)
}
func (s *Store) ClaimFASEItem(ctx context.Context, item string) (model.FASEClaim, error) {
	if _, ok := model.FASEActorFromContext(ctx); !ok {
		return model.FASEClaim{}, fmt.Errorf("%w: a resolved FASE caller is required", ErrValidation)
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return model.FASEClaim{}, e
	}
	defer tx.Rollback()
	if e = lockFASEWrite(ctx, tx); e != nil {
		return model.FASEClaim{}, e
	}
	now := time.Now().UTC()
	if strings.HasPrefix(item, "SP-") {
		state, err := loadSprintState(ctx, tx, item)
		if err != nil {
			return model.FASEClaim{}, err
		}
		if state.Sprint.Status != "Open" && state.Sprint.Status != "On Hold" && state.Sprint.Status != "Active" {
			return model.FASEClaim{}, fmt.Errorf("%w: terminal work cannot be claimed", ErrValidation)
		}
		e = claimFASESprint(ctx, tx, state.Sprint, now)
	} else {
		var project, status string
		if e = tx.QueryRowContext(ctx, "SELECT project_id,status FROM tasks WHERE task_id=?", item).Scan(&project, &status); errors.Is(e, sql.ErrNoRows) {
			return model.FASEClaim{}, ErrNotFound
		} else if e != nil {
			return model.FASEClaim{}, e
		}
		if status != "Open" && status != "On Hold" {
			return model.FASEClaim{}, fmt.Errorf("%w: terminal work cannot be claimed", ErrValidation)
		}
		e = claimFASE(ctx, tx, item, project, now)
	}
	if e != nil {
		return model.FASEClaim{}, e
	}
	var result model.FASEClaim
	var at string
	e = tx.QueryRowContext(ctx, "SELECT item_id,project_id,agent_key,fase_id,claimed_at FROM fase_execution_claims WHERE item_id=?", item).Scan(&result.ItemID, &result.ProjectID, &result.AgentKey, &result.FASEID, &at)
	if e != nil {
		return result, e
	}
	result.ClaimedAt, e = parseTimestamp(at)
	if e != nil {
		return result, e
	}
	return result, tx.Commit()
}
func (s *Store) ReleaseFASEClaim(ctx context.Context, item string) error {
	if _, ok := model.FASEActorFromContext(ctx); !ok {
		return fmt.Errorf("%w: a resolved FASE caller is required", ErrValidation)
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = lockFASEWrite(ctx, tx); e != nil {
		return e
	}
	if e = checkFASEClaim(ctx, tx, item); e != nil {
		return e
	}
	var project string
	if e = tx.QueryRowContext(ctx, "SELECT project_id FROM fase_execution_claims WHERE item_id=?", item).Scan(&project); errors.Is(e, sql.ErrNoRows) {
		return nil
	} else if e != nil {
		return e
	}
	if strings.HasPrefix(item, "SP-") {
		var status string
		if e = tx.QueryRowContext(ctx, "SELECT status FROM sprints WHERE sprint_id=?", item).Scan(&status); e != nil {
			return e
		}
		if status == "Active" {
			return fmt.Errorf("%w: place active work On Hold before releasing its execution claim", ErrValidation)
		}
	} else {
		var live int
		if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM fase_execution_claims c JOIN sprints s ON s.sprint_id=c.item_id WHERE s.task_id=? AND s.status IN ('Open','Active','On Hold')", item).Scan(&live); e != nil {
			return e
		}
		if live > 0 {
			return fmt.Errorf("%w: release live child execution claims before their parent", ErrValidation)
		}
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM fase_execution_claims WHERE item_id=?", item); e != nil {
		return e
	}
	if e = recordProjectEvent(ctx, tx, project, "execution_claim", item, "execution_claim_released", "Execution claim explicitly released.", time.Now().UTC()); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) ListFASEClaims(ctx context.Context, projectID string) ([]model.FASEClaim, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT item_id,project_id,agent_key,fase_id,claimed_at FROM fase_execution_claims WHERE project_id=? ORDER BY item_id", projectID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := []model.FASEClaim{}
	for rows.Next() {
		var c model.FASEClaim
		var at string
		if e = rows.Scan(&c.ItemID, &c.ProjectID, &c.AgentKey, &c.FASEID, &at); e != nil {
			return nil, e
		}
		c.ClaimedAt, e = parseTimestamp(at)
		if e != nil {
			return nil, e
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
