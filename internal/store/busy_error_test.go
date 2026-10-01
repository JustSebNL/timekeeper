// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/store"
)

// fakeSQLiteError mirrors the shape SQLite drivers expose: a primary result
// code plus a message.
type fakeSQLiteError struct {
	code int
	msg  string
}

func (e fakeSQLiteError) Error() string { return e.msg }
func (e fakeSQLiteError) Code() int     { return e.code }

func TestIsBusyErrorClassifiesTransientWriteContention(t *testing.T) {
	busy := []error{
		fakeSQLiteError{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"},
		fakeSQLiteError{code: 6, msg: "database table is locked (6) (SQLITE_LOCKED)"},
		fmt.Errorf("update sprint transition: %w", fakeSQLiteError{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"}),
		errors.New("update sprint transition: database is locked (5) (SQLITE_BUSY)"),
	}
	for i, err := range busy {
		if !store.IsBusyError(err) {
			t.Fatalf("case %d: %v should be classified as transient contention", i, err)
		}
	}
}

func TestIsBusyErrorRejectsPermanentFailuresAndNil(t *testing.T) {
	if store.IsBusyError(nil) {
		t.Fatal("nil must not be classified as contention")
	}
	permanent := []error{
		errors.New("task cannot complete while it owns non-terminal sprints or subtasks"),
		fakeSQLiteError{code: 19, msg: "constraint failed (19) (SQLITE_CONSTRAINT)"},
		fmt.Errorf("%w: project P-1", store.ErrNotFound),
		fmt.Errorf("%w: name is required", store.ErrValidation),
	}
	for i, err := range permanent {
		if store.IsBusyError(err) {
			t.Fatalf("case %d: %v must not be classified as contention", i, err)
		}
	}
}
