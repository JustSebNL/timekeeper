// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenMigratesExistingSprintsForSubtaskOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timekeeper.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE sprints (
		sprint_id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		sprint_name TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		_ = raw.Close()
		t.Fatalf("create legacy sprints table: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	rows, err := database.db.Query("PRAGMA table_info(sprints)")
	if err != nil {
		t.Fatalf("inspect sprints columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan sprints column: %v", err)
		}
		if name == "subtask_id" {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sprints columns: %v", err)
	}
	t.Fatal("sprints table is missing subtask_id after migration")
}
