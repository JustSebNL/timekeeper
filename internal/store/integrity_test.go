// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestStoreIntegrityCheckAcceptsHealthyPrimary(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "primary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Health gate"}); err != nil {
		t.Fatal(err)
	}
	if err := database.IntegrityCheck(ctx); err != nil {
		t.Fatalf("healthy primary integrity check: %v", err)
	}
}

func TestCheckDatabaseFileIntegrityIsReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.db")
	database, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Read-only check"}); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CheckDatabaseFileIntegrity(context.Background(), path); err != nil {
		t.Fatalf("healthy candidate integrity check: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(after) != sha256.Sum256(before) {
		t.Fatal("read-only integrity check changed the candidate database")
	}
}

func TestCheckDatabaseFileIntegrityRejectsNonSQLiteCandidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-sqlite.db")
	if err := os.WriteFile(path, []byte("not a SQLite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckDatabaseFileIntegrity(context.Background(), path); err == nil {
		t.Fatal("expected invalid SQLite candidate to fail integrity check")
	}
}
