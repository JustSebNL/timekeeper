package store

import (
	"context"
	"fmt"
	"strings"
)

// migratePulseBroadcastKind adds the 'broadcast' nudge kind so operators
// can send server-restart announcements to all active agents.
func (s *Store) migratePulseBroadcastKind(ctx context.Context) error {
	var schema string
	if err := s.db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type='table' AND name='pulse_nudges'").Scan(&schema); err != nil {
		return err
	}
	if strings.Contains(schema, "'broadcast'") {
		return nil
	}
	old := "CHECK (kind IN ('agent_unresponsive', 'sprint_overdue'))"
	if !strings.Contains(schema, old) {
		// Pattern not found — migration already ran or schema is in a
		// different state. Return nil (no-op) rather than erroring.
		return nil
	}
	replacement := strings.Replace(schema, "pulse_nudges", "pulse_nudges_broadcast", 1)
	replacement = strings.Replace(replacement, old, "CHECK (kind IN ('agent_unresponsive', 'sprint_overdue', 'broadcast'))", 1)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sequence int64
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM sqlite_sequence WHERE name='pulse_nudges'").Scan(&sequence); err != nil {
		return err
	}
	columns := "nudge_id,agent_id,active_sprint_id,kind,status,detected_after_seconds,delivery_attempts,last_delivery_at,delivered_at,created_at,acknowledged_at,acknowledged_by"
	for _, statement := range []string{
		replacement,
		"INSERT INTO pulse_nudges_broadcast (" + columns + ") SELECT " + columns + " FROM pulse_nudges",
		"DROP TABLE pulse_nudges",
		"ALTER TABLE pulse_nudges_broadcast RENAME TO pulse_nudges",
		"CREATE UNIQUE INDEX pulse_nudges_one_pending_agent_idx ON pulse_nudges(agent_id) WHERE status = 'Pending'",
		"CREATE INDEX pulse_nudges_agent_status_created_idx ON pulse_nudges(agent_id,status,created_at,nudge_id)",
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate Pulse broadcast notifications: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sqlite_sequence SET seq = MAX(seq, ?) WHERE name = 'pulse_nudges'", sequence); err != nil {
		return err
	}
	return tx.Commit()
}
