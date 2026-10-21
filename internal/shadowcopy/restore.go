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
	"runtime"
	"strings"
	"time"
)

// RestoreResult records the verified candidate and the preserved prior database
// file set. The caller must hold the Core instance lock and reserve its listener.
type RestoreResult struct {
	Slot               string
	PrimaryPath        string
	PreservedDirectory string
	SHA256             string
	CleanupWarnings    []string
}

type preservedFile struct {
	Source string `json:"source"`
	SHA256 string `json:"sha256"`
}

type restoreManifest struct {
	Version         int             `json:"version"`
	Slot            string          `json:"slot"`
	PrimaryPath     string          `json:"primary_path"`
	CandidateSHA256 string          `json:"candidate_sha256"`
	CreatedAt       time.Time       `json:"created_at"`
	PreservedFiles  []preservedFile `json:"preserved_files"`
}

type movedFile struct {
	original string
	rollback string
}

// Restore checks and installs one explicit A/B slot while preserving the current
// database file set. Callers must first prove Core is stopped/locked.
func Restore(ctx context.Context, primaryPath, directory, slot string, checkFile FileIntegrityCheck) (RestoreResult, error) {
	if checkFile == nil {
		return RestoreResult{}, errors.New("shadow restore requires a read-only candidate integrity checker")
	}
	absolute, err := filepath.Abs(strings.TrimSpace(directory))
	if err != nil || strings.TrimSpace(directory) == "" {
		return RestoreResult{}, errors.New("shadow restore directory is required")
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("inspect shadow restore directory: %w", err)
	}
	if !info.IsDir() {
		return RestoreResult{}, fmt.Errorf("shadow restore path %q is not a directory", absolute)
	}
	manager := &Manager{directory: absolute, candidateCheck: checkFile}
	return manager.Restore(ctx, primaryPath, slot)
}

// Restore verifies a selected A/B candidate, stages and verifies it beside the
// primary, preserves the current database plus WAL/SHM, then replaces the
// primary. It never starts Core or automatically selects a recovery candidate.
func (m *Manager) Restore(ctx context.Context, primaryPath, slot string) (RestoreResult, error) {
	if m == nil {
		return RestoreResult{}, errors.New("shadow copy manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !validSlot(slot) {
		return RestoreResult{}, fmt.Errorf("shadow restore slot must be %q or %q", SlotA, SlotB)
	}
	if err := ctx.Err(); err != nil {
		return RestoreResult{}, err
	}
	primaryPath = strings.TrimSpace(primaryPath)
	if primaryPath == "" || primaryPath == ":memory:" || strings.HasPrefix(primaryPath, "file:") {
		return RestoreResult{}, errors.New("shadow restore requires a filesystem database path")
	}
	primary, err := filepath.Abs(primaryPath)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("resolve primary database path: %w", err)
	}
	candidate := filepath.Join(m.directory, slot)
	if samePath(primary, candidate) || samePath(primary, filepath.Join(m.directory, SlotA)) || samePath(primary, filepath.Join(m.directory, SlotB)) {
		return RestoreResult{}, errors.New("shadow restore candidate cannot be the primary database")
	}
	candidateInfo, err := os.Lstat(candidate)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("inspect restore candidate: %w", err)
	}
	if !candidateInfo.Mode().IsRegular() {
		return RestoreResult{}, errors.New("shadow restore candidate must be a regular file")
	}
	if err := m.runCandidateCheck(ctx, candidate, "restore candidate"); err != nil {
		return RestoreResult{}, fmt.Errorf("candidate health check failed; primary unchanged: %w", err)
	}
	candidateHash, err := hashFile(candidate)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("hash restore candidate: %w", err)
	}
	primaryDir := filepath.Dir(primary)
	info, err := os.Stat(primaryDir)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("inspect primary database directory: %w", err)
	}
	if !info.IsDir() {
		return RestoreResult{}, fmt.Errorf("primary database parent %q is not a directory", primaryDir)
	}
	stagedPath, err := createStagedCopy(ctx, candidate, primaryDir, candidateHash, m.candidateCheck)
	if err != nil {
		return RestoreResult{}, err
	}
	defer os.Remove(stagedPath)

	preservedDir, err := os.MkdirTemp(m.directory, "recovery-before-restore-")
	if err != nil {
		return RestoreResult{}, fmt.Errorf("create pre-restore preservation directory: %w", err)
	}
	preserved := make([]preservedFile, 0, 3)
	originals := []string{primary, primary + "-wal", primary + "-shm"}
	for _, original := range originals {
		sourceInfo, statErr := os.Lstat(original)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return RestoreResult{}, fmt.Errorf("inspect current database file %q: %w", original, statErr)
		}
		if !sourceInfo.Mode().IsRegular() {
			return RestoreResult{}, fmt.Errorf("refuse to preserve non-regular database file %q", original)
		}
		destination := filepath.Join(preservedDir, filepath.Base(original))
		hash, copyErr := copyFile(ctx, original, destination)
		if copyErr != nil {
			return RestoreResult{}, fmt.Errorf("preserve current database file %q: %w", original, copyErr)
		}
		sourceHash, hashErr := hashFile(original)
		if hashErr != nil || sourceHash != hash {
			return RestoreResult{}, fmt.Errorf("preserved database file %q failed hash read-back", original)
		}
		preserved = append(preserved, preservedFile{Source: original, SHA256: hash})
	}
	metadata, err := json.Marshal(restoreManifest{
		Version:         1,
		Slot:            slot,
		PrimaryPath:     primary,
		CandidateSHA256: candidateHash,
		CreatedAt:       time.Now().UTC(),
		PreservedFiles:  preserved,
	})
	if err != nil {
		return RestoreResult{}, fmt.Errorf("encode pre-restore manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(preservedDir, "restore.json"), append(metadata, '\n'), 0o600); err != nil {
		return RestoreResult{}, fmt.Errorf("write pre-restore manifest: %w", err)
	}

	rollbackToken := strings.TrimPrefix(filepath.Base(stagedPath), ".shadow-restore-")
	rollbackToken = strings.TrimSuffix(rollbackToken, ".tmp")
	moved := make([]movedFile, 0, 3)
	rollback := func() error {
		var rollbackErr error
		for i := len(moved) - 1; i >= 0; i-- {
			file := moved[i]
			if err := os.Rename(file.rollback, file.original); err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %q from %q: %w", file.original, file.rollback, err))
			}
		}
		return rollbackErr
	}
	for _, original := range originals {
		if _, statErr := os.Lstat(original); errors.Is(statErr, os.ErrNotExist) {
			continue
		} else if statErr != nil {
			return RestoreResult{}, errors.Join(fmt.Errorf("inspect current database path %q before replacement: %w", original, statErr), rollback())
		}
		rollbackPath := filepath.Join(primaryDir, ".shadow-restore-rollback-"+rollbackToken+"-"+filepath.Base(original))
		if err := os.Rename(original, rollbackPath); err != nil {
			return RestoreResult{}, errors.Join(fmt.Errorf("stage current database path %q for replacement: %w", original, err), rollback())
		}
		moved = append(moved, movedFile{original: original, rollback: rollbackPath})
	}
	if err := os.Rename(stagedPath, primary); err != nil {
		return RestoreResult{}, errors.Join(fmt.Errorf("install verified shadow candidate: %w", err), rollback())
	}
	if err := m.runCandidateCheck(ctx, primary, "restored primary"); err != nil {
		failedPath := primary + ".failed-shadow-restore-" + rollbackToken
		moveErr := os.Rename(primary, failedPath)
		rollbackErr := rollback()
		return RestoreResult{}, errors.Join(fmt.Errorf("restored primary failed integrity verification: %w", err), moveErr, rollbackErr)
	}
	result := RestoreResult{Slot: slot, PrimaryPath: primary, PreservedDirectory: preservedDir, SHA256: candidateHash}
	for _, file := range moved {
		if err := os.Remove(file.rollback); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.CleanupWarnings = append(result.CleanupWarnings, fmt.Sprintf("could not remove temporary rollback file %q: %v", file.rollback, err))
		}
	}
	return result, nil
}

func createStagedCopy(ctx context.Context, source, directory, expectedHash string, check FileIntegrityCheck) (string, error) {
	placeholder, err := os.CreateTemp(directory, ".shadow-restore-*.tmp")
	if err != nil {
		return "", fmt.Errorf("reserve restore staging path: %w", err)
	}
	path := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close restore staging path: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("prepare restore staging path: %w", err)
	}
	hash, err := copyFile(ctx, source, path)
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("stage verified restore candidate: %w", err)
	}
	if hash != expectedHash {
		_ = os.Remove(path)
		return "", errors.New("staged restore candidate hash does not match the verified shadow")
	}
	if err := check(ctx, path); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("staged restore candidate health check failed: %w", err)
	}
	if err := syncFile(path); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("flush staged restore candidate: %w", err)
	}
	return path, nil
}

func copyFile(ctx context.Context, source, destination string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	removeOnError := true
	defer func() {
		_ = output.Close()
		if removeOnError {
			_ = os.Remove(destination)
		}
	}()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(output, hash), input); err != nil {
		return "", err
	}
	if err := output.Sync(); err != nil {
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return "", err
	}
	removeOnError = false
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
