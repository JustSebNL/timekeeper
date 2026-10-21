// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupToHealthChecksPrimaryBeforeDestinationOrSnapshotWork(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "damaged.db")
	if err := os.WriteFile(databasePath, []byte("not a SQLite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	destination := filepath.Join(dir, "already-exists.db")
	original := []byte("preserve this destination")
	if err := os.WriteFile(destination, original, 0o600); err != nil {
		t.Fatal(err)
	}

	store := &Store{db: database}
	err = store.BackupTo(context.Background(), destination)
	if err == nil || !strings.Contains(err.Error(), "primary health check failed") {
		t.Fatalf("BackupTo error = %v, want primary-health rejection", err)
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != string(original) {
		t.Fatalf("existing destination changed: %q", contents)
	}
}
