package store

import (
	"context"
	"fmt"
)

func (s *Store) migrateRegistrationQueue(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS registration_queue (
			queue_seq INTEGER PRIMARY KEY AUTOINCREMENT,
			queue_id TEXT NOT NULL UNIQUE,
			project_id TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('task', 'subtask', 'sprint', 'task_batch', 'execution_batch')),
			idempotency_key TEXT NOT NULL DEFAULT '',
			payload_hash TEXT NOT NULL CHECK (length(payload_hash) = 64),
			payload_json TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('Queued', 'Registered', 'Rejected')),
			counts_json TEXT NOT NULL DEFAULT '{}',
			validation_json TEXT NOT NULL DEFAULT '',
			result_json TEXT NOT NULL DEFAULT '',
			attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			processed_at TEXT
		)`,
		"CREATE UNIQUE INDEX IF NOT EXISTS registration_queue_idempotency_idx ON registration_queue(project_id, idempotency_key) WHERE idempotency_key != ''",
		"CREATE INDEX IF NOT EXISTS registration_queue_project_order_idx ON registration_queue(project_id, queue_seq)",
		"CREATE INDEX IF NOT EXISTS registration_queue_status_order_idx ON registration_queue(status, queue_seq)",
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create registration queue schema: %w", err)
		}
	}
	return nil
}
