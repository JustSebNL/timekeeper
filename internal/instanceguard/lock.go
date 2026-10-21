// Package instanceguard prevents multiple TimeKeeper Core processes from using
// the same SQLite database or starting independently from one installation.
// Kernel-managed file locks are released automatically when a process exits or crashes.
package instanceguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrAlreadyRunning     = errors.New("another TimeKeeper instance is already using this database")
	ErrCoreAlreadyRunning = errors.New("tk is already running")
)

// Lock is an OS-backed advisory lock held for the lifetime of a TimeKeeper
// server instance. Do not remove Path on Close: unlinking a locked file can let
// another process create a second inode and bypass the lock on Unix.
type Lock struct {
	file *os.File
	Path string
}

// Acquire locks the database identity. Equivalent cleaned relative/absolute
// paths converge on one lock file; kernel ownership makes stale lock recovery
// automatic after crashes and forced process termination.
func Acquire(databasePath string) (*Lock, error) {
	databasePath = strings.TrimSpace(databasePath)
	if databasePath == "" || databasePath == ":memory:" {
		return nil, errors.New("instance guard requires a file-backed database path")
	}
	absolute, err := filepath.Abs(databasePath)
	if err != nil {
		return nil, fmt.Errorf("resolve database path for instance guard: %w", err)
	}
	absolute = filepath.Clean(absolute)
	return acquireFileLock(absolute+".instance.lock", ErrAlreadyRunning, "database", absolute)
}

// AcquireCoreInstance prevents a second Core process from this TimeKeeper
// installation, even when it is configured with a different database path or
// listener. The interactive `tk` CLI is a client and does not acquire this lock.
func AcquireCoreInstance(databasePath, uiPath string) (*Lock, error) {
	lockDir, err := coreInstanceLockDirectory(databasePath, uiPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return nil, fmt.Errorf("create TimeKeeper Core lock directory %q: %w", lockDir, err)
	}
	return acquireFileLock(filepath.Join(lockDir, "core.instance.lock"), ErrCoreAlreadyRunning, "installation", lockDir)
}

func coreInstanceLockDirectory(databasePath, uiPath string) (string, error) {
	for _, candidate := range []string{databasePath, uiPath} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			return "", fmt.Errorf("resolve TimeKeeper Core path %q: %w", candidate, err)
		}
		current := filepath.Dir(absolute)
		for {
			if strings.EqualFold(filepath.Base(current), ".timekeeper") {
				return filepath.Clean(current), nil
			}
			if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
				return filepath.Join(current, ".timekeeper"), nil
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	if strings.TrimSpace(databasePath) == "" || strings.TrimSpace(databasePath) == ":memory:" {
		return "", errors.New("Core instance guard requires a file-backed database path or UI path")
	}
	absolute, err := filepath.Abs(databasePath)
	if err != nil {
		return "", fmt.Errorf("resolve database path for Core instance guard: %w", err)
	}
	return filepath.Dir(filepath.Clean(absolute)), nil
}

func acquireFileLock(lockPath string, alreadyRunning error, scope, identity string) (*Lock, error) {
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open TimeKeeper instance lock %q: %w", lockPath, err)
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		if errors.Is(err, errLockBusy) {
			return nil, fmt.Errorf("%w (%s: %s)", alreadyRunning, scope, identity)
		}
		return nil, fmt.Errorf("lock TimeKeeper %s %q: %w", scope, identity, err)
	}
	return &Lock{file: file, Path: lockPath}, nil
}

// Close releases the kernel lock. The lock file itself is intentionally kept.
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

var errLockBusy = errors.New("lock is already held")
