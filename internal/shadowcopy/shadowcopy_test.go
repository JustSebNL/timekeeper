// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package shadowcopy_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/shadowcopy"
	"github.com/JustSebNL/timekeeper/internal/store"
)

type sourceStub struct {
	healthErr   error
	backupErr   error
	payload     string
	calls       []string
	backupCalls int
}

func (s *sourceStub) IntegrityCheck(context.Context) error {
	s.calls = append(s.calls, "primary-health")
	return s.healthErr
}

func (s *sourceStub) BackupTo(ctx context.Context, destination string) error {
	if err := s.IntegrityCheck(ctx); err != nil {
		return fmt.Errorf("primary health check failed; backup skipped: %w", err)
	}
	s.calls = append(s.calls, "snapshot")
	s.backupCalls++
	if s.backupErr != nil {
		return s.backupErr
	}
	return os.WriteFile(destination, []byte(s.payload), 0o600)
}

func newManager(t *testing.T, source *sourceStub) (*shadowcopy.Manager, *[]string) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "shadow")
	events := &[]string{}
	manager, err := shadowcopy.New(source, directory, func(_ context.Context, path string) error {
		*events = append(*events, "candidate-health")
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(string(content), "healthy:") {
			return errors.New("snapshot is not healthy")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create shadow manager: %v", err)
	}
	return manager, events
}

func TestRunOnceHealthChecksPrimaryBeforeCreatingSnapshot(t *testing.T) {
	source := &sourceStub{payload: "healthy:first"}
	manager, candidateEvents := newManager(t, source)
	result, err := manager.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("first shadow copy: %v", err)
	}
	if result.Slot != shadowcopy.SlotA {
		t.Fatalf("first slot = %q, want %q", result.Slot, shadowcopy.SlotA)
	}
	if len(source.calls) < 2 || source.calls[0] != "primary-health" || source.calls[1] != "snapshot" {
		t.Fatalf("source call order = %#v", source.calls)
	}
	if len(*candidateEvents) != 1 || (*candidateEvents)[0] != "candidate-health" {
		t.Fatalf("candidate checks = %#v", *candidateEvents)
	}
}

func TestUnhealthyPrimaryCannotRotateKnownGoodShadow(t *testing.T) {
	source := &sourceStub{payload: "healthy:first"}
	manager, _ := newManager(t, source)
	first, err := manager.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	knownGood, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	backupCalls := source.backupCalls
	source.healthErr = errors.New("database disk image is malformed")
	if _, err := manager.RunOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "primary health check") {
		t.Fatalf("unhealthy primary result = %v", err)
	}
	if source.backupCalls != backupCalls {
		t.Fatalf("snapshot called %d times, want %d", source.backupCalls, backupCalls)
	}
	after, err := os.ReadFile(first.Path)
	if err != nil || string(after) != string(knownGood) {
		t.Fatalf("known-good shadow changed: bytes=%q err=%v", after, err)
	}
}

func TestFailedCandidateVerificationDoesNotReplaceKnownGoodShadow(t *testing.T) {
	source := &sourceStub{payload: "healthy:first"}
	manager, _ := newManager(t, source)
	first, err := manager.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	source.payload = "corrupt:new-copy"
	if _, err := manager.RunOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "snapshot integrity") {
		t.Fatalf("failed candidate result = %v", err)
	}
	after, err := os.ReadFile(first.Path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("last known-good slot changed: bytes=%q err=%v", after, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(first.Path), shadowcopy.SlotB)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unverified B slot exists or stat failed: %v", err)
	}
}

func TestRunOnceRotatesAThenBAndKeepsBothVerifiedCopies(t *testing.T) {
	source := &sourceStub{payload: "healthy:first"}
	manager, _ := newManager(t, source)
	first, err := manager.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	source.payload = "healthy:second"
	second, err := manager.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Slot != shadowcopy.SlotA || second.Slot != shadowcopy.SlotB {
		t.Fatalf("rotation slots = %q, %q", first.Slot, second.Slot)
	}
	if second.SHA256 == first.SHA256 {
		t.Fatal("new shadow hash did not change")
	}
	preserved, err := os.ReadFile(first.Path)
	if err != nil || string(preserved) != string(firstBytes) {
		t.Fatalf("first verified slot changed during B rotation: bytes=%q err=%v", preserved, err)
	}
	latest, err := os.ReadFile(second.Path)
	if err != nil || string(latest) != "healthy:second" {
		t.Fatalf("latest verified slot = %q err=%v", latest, err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(filepath.Dir(second.Path), "shadow-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var active struct {
		Slot   string `json:"slot"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(manifestBytes, &active); err != nil {
		t.Fatal(err)
	}
	if active.Slot != second.Slot || active.SHA256 != second.SHA256 {
		t.Fatalf("active manifest = %+v, want slot=%s sha256=%s", active, second.Slot, second.SHA256)
	}
}

func TestRunPeriodicStartsImmediatelyAndStopsOnCancellation(t *testing.T) {
	source := &sourceStub{payload: "healthy:periodic"}
	manager, _ := newManager(t, source)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reported := 0
	err := shadowcopy.RunPeriodic(ctx, manager, time.Minute, func(_ shadowcopy.Result, runErr error) {
		reported++
		if runErr != nil {
			t.Errorf("initial shadow run: %v", runErr)
		}
		cancel()
	})
	if err != nil {
		t.Fatalf("RunPeriodic() = %v", err)
	}
	if reported != 1 || source.backupCalls != 1 {
		t.Fatalf("initial run reports=%d backup calls=%d", reported, source.backupCalls)
	}
}

func TestRunPeriodicRejectsSubminuteIntervals(t *testing.T) {
	source := &sourceStub{payload: "healthy:unused"}
	manager, _ := newManager(t, source)
	if err := shadowcopy.RunPeriodic(context.Background(), manager, time.Second, nil); err == nil {
		t.Fatal("expected sub-minute interval to be rejected")
	}
	if source.backupCalls != 0 {
		t.Fatalf("sub-minute interval performed %d snapshots", source.backupCalls)
	}
}

func TestRealStoreShadowSnapshotContainsReadableCoreData(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	database, err := store.Open(filepath.Join(root, "primary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	first, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Shadow A"})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := shadowcopy.New(database, filepath.Join(root, "recovery"), store.CheckDatabaseFileIntegrity)
	if err != nil {
		t.Fatal(err)
	}
	firstCopy, err := manager.RunOnce(ctx)
	if err != nil {
		t.Fatalf("create first real shadow: %v", err)
	}
	if err := database.IntegrityCheck(ctx); err != nil {
		t.Fatalf("primary health after first shadow: %v", err)
	}
	second, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Shadow B"})
	if err != nil {
		t.Fatal(err)
	}
	secondCopy, err := manager.RunOnce(ctx)
	if err != nil {
		t.Fatalf("create second real shadow: %v", err)
	}
	if firstCopy.Slot != shadowcopy.SlotA || secondCopy.Slot != shadowcopy.SlotB {
		t.Fatalf("real shadow slots = %q, %q", firstCopy.Slot, secondCopy.Slot)
	}
	if err := store.CheckDatabaseFileIntegrity(ctx, secondCopy.Path); err != nil {
		t.Fatalf("read-only second shadow integrity: %v", err)
	}
	copyDB, err := store.Open(secondCopy.Path)
	if err != nil {
		t.Fatalf("open second shadow: %v", err)
	}
	projects, err := copyDB.ListProjects(ctx)
	if closeErr := copyDB.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatalf("read second shadow projects: %v", err)
	}
	projectIDs := map[string]bool{}
	for _, project := range projects {
		projectIDs[project.ProjectID] = true
	}
	if len(projects) != 2 || !projectIDs[first.ProjectID] || !projectIDs[second.ProjectID] {
		t.Fatalf("shadow projects = %+v", projects)
	}
}
