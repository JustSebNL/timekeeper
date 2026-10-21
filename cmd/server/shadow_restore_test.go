// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func makeShadowRestoreCandidate(t *testing.T, directory string) string {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	source, err := store.Open(filepath.Join(t.TempDir(), "candidate-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.CreateProject(context.Background(), model.CreateProjectInput{Name: "Verified shadow"}); err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	candidate := filepath.Join(directory, "shadow-a.db")
	if err := source.BackupTo(context.Background(), candidate); err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckDatabaseFileIntegrity(context.Background(), candidate); err != nil {
		t.Fatalf("candidate fixture integrity: %v", err)
	}
	return candidate
}

func TestRestoreShadowSlotRequiresExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "timekeeper.db")
	original := []byte("current primary")
	if err := os.WriteFile(primary, original, 0o600); err != nil {
		t.Fatal(err)
	}
	shadowDir := filepath.Join(root, "recovery")
	makeShadowRestoreCandidate(t, shadowDir)
	if _, err := runShadowRestore(context.Background(), primary, shadowDir, "shadow-a.db", "127.0.0.1:0", false); err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("unconfirmed restore error = %v", err)
	}
	current, err := os.ReadFile(primary)
	if err != nil || string(current) != string(original) {
		t.Fatalf("unconfirmed restore changed primary: %q err=%v", current, err)
	}
}

func TestRestoreShadowSlotRefusesRunningCoreListener(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "timekeeper.db")
	original := []byte("current primary")
	if err := os.WriteFile(primary, original, 0o600); err != nil {
		t.Fatal(err)
	}
	shadowDir := filepath.Join(root, "recovery")
	makeShadowRestoreCandidate(t, shadowDir)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if _, err := runShadowRestore(context.Background(), primary, shadowDir, "shadow-a.db", listener.Addr().String(), true); err == nil || !strings.Contains(err.Error(), "listener") {
		t.Fatalf("restore with occupied listener error = %v", err)
	}
	current, err := os.ReadFile(primary)
	if err != nil || string(current) != string(original) {
		t.Fatalf("refused restore changed primary: %q err=%v", current, err)
	}
}

func TestRestoreShadowSlotRequiresHealthyCopyAndPreservesWALSet(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	primary := filepath.Join(root, "timekeeper.db")
	original := []byte("old primary file")
	if err := os.WriteFile(primary, original, 0o600); err != nil {
		t.Fatal(err)
	}
	oldWAL := []byte("old wal bytes")
	oldSHM := []byte("old shm bytes")
	if err := os.WriteFile(primary+"-wal", oldWAL, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primary+"-shm", oldSHM, 0o600); err != nil {
		t.Fatal(err)
	}
	shadowDir := filepath.Join(root, "recovery")
	makeShadowRestoreCandidate(t, shadowDir)

	result, err := runShadowRestore(ctx, primary, shadowDir, "shadow-a.db", "127.0.0.1:0", true)
	if err != nil {
		t.Fatalf("restore verified slot: %v", err)
	}
	if result.Slot != "shadow-a.db" || result.PreservedDirectory == "" {
		t.Fatalf("restore result = %+v", result)
	}
	if err := store.CheckDatabaseFileIntegrity(ctx, primary); err != nil {
		t.Fatalf("restored primary integrity: %v", err)
	}
	for name, expected := range map[string][]byte{"timekeeper.db": original, "timekeeper.db-wal": oldWAL, "timekeeper.db-shm": oldSHM} {
		preserved, err := os.ReadFile(filepath.Join(result.PreservedDirectory, name))
		if err != nil || string(preserved) != string(expected) {
			t.Fatalf("preserved %s = %q err=%v", name, preserved, err)
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(primary + suffix); !os.IsNotExist(err) {
			t.Fatalf("stale sidecar %s remains: %v", suffix, err)
		}
	}
}

func TestResolveShadowCopyDirectoryUsesDatabaseSiblingAndHonorsOverride(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "state", "timekeeper.db")
	defaultPath, err := resolveShadowCopyDirectory(databasePath, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "state", "recovery", "sqlite-shadow"); defaultPath != want {
		t.Fatalf("default shadow directory = %q, want %q", defaultPath, want)
	}
	explicit := filepath.Join(root, "E recovery")
	got, err := resolveShadowCopyDirectory(databasePath, explicit)
	if err != nil || got != explicit {
		t.Fatalf("explicit shadow directory = %q err=%v", got, err)
	}
	if _, err := resolveShadowCopyDirectory(":memory:", ""); err == nil {
		t.Fatal("expected default shadow directory to reject an in-memory primary")
	}
}
