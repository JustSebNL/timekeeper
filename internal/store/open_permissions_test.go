// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestOpenRequestsOwnerOnlyDatabasePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file permissions are governed by directory ACLs, not os.FileMode")
	}
	path := filepath.Join(t.TempDir(), "timekeeper.db")
	database, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions = %o, want 600", info.Mode().Perm())
	}
}
