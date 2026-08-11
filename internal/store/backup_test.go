// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestBackupToCreatesIndependentSQLiteSnapshot(t *testing.T) {
	ctx := context.Background()
	source, err := store.Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	project, err := source.CreateProject(ctx, model.CreateProjectInput{Name: "Recover me"})
	if err != nil {
		t.Fatalf("create source project: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "timekeeper-backup.db")
	if err := source.BackupTo(ctx, backupPath); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}
	if info, err := os.Stat(backupPath); err != nil || info.Size() == 0 {
		t.Fatalf("backup file = %#v, err = %v", info, err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions = %o, want 600", info.Mode().Perm())
	}
	copy, err := store.Open(backupPath)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = copy.Close() })
	items, err := copy.ListProjects(ctx)
	if err != nil {
		t.Fatalf("list backup projects: %v", err)
	}
	if len(items) != 1 || items[0].ProjectID != project.ProjectID || items[0].ProjectName != "Recover me" {
		t.Fatalf("backup projects = %#v", items)
	}
}
