// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectLifecycleMigrationBackfillsExistingProjectsSafely(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "existing.db")
	database, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Existing Project"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"deadline_warning_days", "schedule_timezone", "lifecycle_phase", "cancelled_at", "completed_at", "actual_started_at", "target_end_date", "planned_start_date"} {
		if _, err := legacy.ExecContext(ctx, "ALTER TABLE projects DROP COLUMN "+column); err != nil {
			_ = legacy.Close()
			t.Fatalf("simulate pre-lifecycle schema without %s: %v", column, err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen and migrate existing database: %v", err)
	}
	defer migrated.Close()
	tree, err := migrated.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Project.LifecyclePhase != model.ProjectPhaseExecution || tree.Project.ScheduleTimezone != "UTC" || tree.Project.DeadlineWarningDays != 7 || tree.Project.PlannedStartDate != "" || tree.Project.TargetEndDate != "" || tree.Project.DeadlineState != "unscheduled" {
		t.Fatalf("migrated Project defaults=%+v", tree.Project)
	}
}
