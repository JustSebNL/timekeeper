// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package shadowcopy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	SlotA        = "shadow-a.db"
	SlotB        = "shadow-b.db"
	manifestFile = "shadow-manifest.json"
)

// Source is the live Core-owned SQLite Store. Shadow copy never opens a second
// Store or writes to the authoritative database.
type Source interface {
	BackupTo(context.Context, string) error
}

// FileIntegrityCheck must perform a full read-only SQLite integrity check.
type FileIntegrityCheck func(context.Context, string) error

// Result describes a fully verified and promoted shadow snapshot.
type Result struct {
	Slot      string    `json:"slot"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type manifest struct {
	Version   int       `json:"version"`
	Slot      string    `json:"slot"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type Manager struct {
	source         Source
	directory      string
	candidateCheck FileIntegrityCheck
	now            func() time.Time
	mu             sync.Mutex
}

// New creates the shadow directory if needed. The supplied checker must be
// read-only; it is used before a slot can become the active recovery copy.
func New(source Source, directory string, checkFile FileIntegrityCheck) (*Manager, error) {
	if source == nil {
		return nil, errors.New("shadow copy requires the authoritative Store")
	}
	if checkFile == nil {
		return nil, errors.New("shadow copy requires a read-only candidate integrity checker")
	}
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, errors.New("shadow copy directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve shadow copy directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create shadow copy directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("inspect shadow copy directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("shadow copy path %q is not a directory", absolute)
	}
	return &Manager{source: source, directory: absolute, candidateCheck: checkFile, now: time.Now}, nil
}

// RunOnce checks the primary before snapshotting, verifies the staged candidate,
// and only then replaces the inactive A/B slot. The previous verified slot is
// left untouched throughout the operation.
func (m *Manager) RunOnce(ctx context.Context) (Result, error) {
	if m == nil {
		return Result{}, errors.New("shadow copy manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	activeSlot, err := m.currentVerifiedSlot(ctx)
	if err != nil {
		return Result{}, err
	}
	targetSlot := SlotA
	if activeSlot == SlotA {
		targetSlot = SlotB
	} else if activeSlot == SlotB {
		targetSlot = SlotA
	}
	temporary, err := os.CreateTemp(m.directory, ".shadow-*.tmp")
	if err != nil {
		return Result{}, fmt.Errorf("reserve shadow snapshot path: %w", err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return Result{}, fmt.Errorf("close temporary shadow path: %w", err)
	}
	if err := os.Remove(temporaryPath); err != nil {
		return Result{}, fmt.Errorf("prepare temporary shadow path: %w", err)
	}
	defer os.Remove(temporaryPath)
	if err := m.source.BackupTo(ctx, temporaryPath); err != nil {
		return Result{}, fmt.Errorf("create consistent shadow snapshot: %w", err)
	}
	if err := m.runCandidateCheck(ctx, temporaryPath, "staged shadow snapshot"); err != nil {
		return Result{}, fmt.Errorf("shadow snapshot integrity verification failed: %w", err)
	}
	hash, err := hashFile(temporaryPath)
	if err != nil {
		return Result{}, fmt.Errorf("hash staged shadow snapshot: %w", err)
	}
	if err := syncFile(temporaryPath); err != nil {
		return Result{}, fmt.Errorf("flush staged shadow snapshot: %w", err)
	}
	targetPath := filepath.Join(m.directory, targetSlot)
	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Result{}, fmt.Errorf("remove inactive shadow slot %q: %w", targetSlot, err)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return Result{}, fmt.Errorf("promote verified shadow snapshot to %q: %w", targetSlot, err)
	}
	promotedHash, err := hashFile(targetPath)
	if err != nil || promotedHash != hash {
		return Result{}, fmt.Errorf("promoted shadow snapshot read-back mismatch: got %q, want %q (error: %v)", promotedHash, hash, err)
	}
	result := Result{Slot: targetSlot, Path: targetPath, SHA256: hash, CreatedAt: m.now().UTC()}
	if err := m.writeManifest(result); err != nil {
		return result, fmt.Errorf("write shadow copy manifest: %w", err)
	}
	return result, nil
}

// RunPeriodic creates a snapshot immediately and then repeats at the requested
// cadence. A zero interval disables the worker; nonzero intervals below one
// minute are rejected to avoid accidental hot-loop snapshots.
func RunPeriodic(ctx context.Context, manager *Manager, interval time.Duration, report func(Result, error)) error {
	if interval == 0 {
		return nil
	}
	if interval < time.Minute {
		return fmt.Errorf("shadow copy interval must be at least 1m or 0, got %s", interval)
	}
	if manager == nil {
		return errors.New("shadow copy manager is unavailable")
	}
	for {
		result, err := manager.RunOnce(ctx)
		if report != nil {
			report(result, err)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

func (m *Manager) currentVerifiedSlot(ctx context.Context) (string, error) {
	manifestPath := filepath.Join(m.directory, manifestFile)
	data, err := os.ReadFile(manifestPath)
	if err == nil {
		var current manifest
		if json.Unmarshal(data, &current) == nil && validSlot(current.Slot) {
			ok, verifyErr := m.verifySlot(ctx, current.Slot, current.SHA256)
			if verifyErr != nil {
				return "", verifyErr
			}
			if ok {
				return current.Slot, nil
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		// A malformed/unreadable manifest is not authority. Scan both slots.
	}
	candidates := make([]Result, 0, 2)
	for _, slot := range []string{SlotA, SlotB} {
		ok, verifyErr := m.verifySlot(ctx, slot, "")
		if verifyErr != nil {
			return "", verifyErr
		}
		if !ok {
			continue
		}
		path := filepath.Join(m.directory, slot)
		info, statErr := os.Stat(path)
		if statErr != nil {
			return "", fmt.Errorf("inspect verified shadow slot %q: %w", slot, statErr)
		}
		hash, hashErr := hashFile(path)
		if hashErr != nil {
			return "", hashErr
		}
		candidates = append(candidates, Result{Slot: slot, Path: path, SHA256: hash, CreatedAt: info.ModTime().UTC()})
	}
	if len(candidates) == 0 {
		return "", nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].Slot < candidates[j].Slot
		}
		return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
	})
	return candidates[0].Slot, nil
}

func (m *Manager) verifySlot(ctx context.Context, slot, expectedHash string) (bool, error) {
	path := filepath.Join(m.directory, slot)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect shadow slot %q: %w", slot, err)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	before, err := hashFile(path)
	if err != nil {
		return false, err
	}
	if expectedHash != "" && before != expectedHash {
		return false, nil
	}
	if err := m.runCandidateCheck(ctx, path, "existing shadow slot"); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	after, err := hashFile(path)
	if err != nil {
		return false, err
	}
	if before != after {
		return false, nil
	}
	return true, nil
}

func (m *Manager) runCandidateCheck(ctx context.Context, path, label string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.candidateCheck(ctx, path); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func (m *Manager) writeManifest(result Result) error {
	contents, err := json.Marshal(manifest{Version: 1, Slot: result.Slot, SHA256: result.SHA256, CreatedAt: result.CreatedAt})
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(m.directory, ".shadow-manifest-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(append(contents, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return err
	}
	path := filepath.Join(m.directory, manifestFile)
	previous := path + ".previous"
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(previous)
		if err := os.Rename(path, previous); err != nil {
			return err
		}
		if err := os.Rename(temporaryPath, path); err != nil {
			_ = os.Rename(previous, path)
			return err
		}
		_ = os.Remove(previous)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func validSlot(slot string) bool { return slot == SlotA || slot == SlotB }
