// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package shadowcopy_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/shadowcopy"
)

func TestRestoreVerifiesCandidateAndPreservesPrimaryFileSet(t *testing.T) {
	source := &sourceStub{payload: "healthy:unused"}
	shadowDir := filepath.Join(t.TempDir(), "recovery")
	manager, err := shadowcopy.New(source, shadowDir, func(_ context.Context, path string) error {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(content) != "healthy:restore-target" {
			return errors.New("candidate not healthy")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join(shadowDir, shadowcopy.SlotA)
	if err := os.WriteFile(candidatePath, []byte("healthy:restore-target"), 0o600); err != nil {
		t.Fatal(err)
	}
	primary := filepath.Join(t.TempDir(), "timekeeper.db")
	oldFiles := map[string]string{
		primary:          "old primary bytes",
		primary + "-wal": "old wal bytes",
		primary + "-shm": "old shm bytes",
	}
	for path, content := range oldFiles {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	result, err := manager.Restore(context.Background(), primary, shadowcopy.SlotA)
	if err != nil {
		t.Fatalf("restore candidate: %v", err)
	}
	if result.Slot != shadowcopy.SlotA || result.PrimaryPath != primary || result.PreservedDirectory == "" {
		t.Fatalf("restore result = %+v", result)
	}
	installed, err := os.ReadFile(primary)
	if err != nil || string(installed) != "healthy:restore-target" {
		t.Fatalf("restored primary = %q err=%v", installed, err)
	}
	for original, expected := range oldFiles {
		preserved, err := os.ReadFile(filepath.Join(result.PreservedDirectory, filepath.Base(original)))
		if err != nil || string(preserved) != expected {
			t.Fatalf("preserved %s = %q err=%v", filepath.Base(original), preserved, err)
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(primary + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale sidecar %s remains beside restored primary: %v", suffix, err)
		}
	}
}

func TestRestoreRejectsUnhealthyCandidateWithoutTouchingPrimary(t *testing.T) {
	source := &sourceStub{payload: "healthy:unused"}
	shadowDir := filepath.Join(t.TempDir(), "recovery")
	manager, err := shadowcopy.New(source, shadowDir, func(_ context.Context, _ string) error {
		return errors.New("database disk image is malformed")
	})
	if err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join(shadowDir, shadowcopy.SlotB)
	if err := os.WriteFile(candidatePath, []byte("bad candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	primary := filepath.Join(t.TempDir(), "timekeeper.db")
	original := []byte("keep current primary")
	if err := os.WriteFile(primary, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Restore(context.Background(), primary, shadowcopy.SlotB); err == nil || !strings.Contains(err.Error(), "candidate health check failed") {
		t.Fatalf("restore error = %v", err)
	}
	current, err := os.ReadFile(primary)
	if err != nil || string(current) != string(original) {
		t.Fatalf("primary changed despite candidate failure: %q err=%v", current, err)
	}
}

func TestRestoreRejectsPathLikeSlotBeforeTouchingPrimary(t *testing.T) {
	manager, _ := newManager(t, &sourceStub{payload: "healthy:unused"})
	primary := filepath.Join(t.TempDir(), "timekeeper.db")
	original := []byte("keep current primary")
	if err := os.WriteFile(primary, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Restore(context.Background(), primary, "../outside.db"); err == nil {
		t.Fatal("expected non-slot restore target to be rejected")
	}
	current, err := os.ReadFile(primary)
	if err != nil || string(current) != string(original) {
		t.Fatalf("primary changed despite invalid slot: %q err=%v", current, err)
	}
}
