package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestCreateExecutionBatchRollsBackLateSQLiteFailure(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "execution-batch-rollback.db"))
	if err != nil {
		t.Fatalf("open Store: %v", err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Atomic batch rollback"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Atomic imports"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(ctx, `CREATE TRIGGER fail_second_batch_sprint BEFORE INSERT ON sprints
		WHEN NEW.sprint_name = 'explode' BEGIN SELECT RAISE(ABORT, 'forced late insert failure'); END`); err != nil {
		t.Fatalf("create late-failure trigger: %v", err)
	}
	var beforeCounter int64
	if err := database.db.QueryRowContext(ctx, "SELECT next_value FROM id_counters WHERE name = 'item'").Scan(&beforeCounter); err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := database.ListEventsAfterID(ctx, project.ProjectID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	result, validation, err := database.CreateExecutionBatch(ctx, project.ProjectID, model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks: []model.ExecutionBatchTask{{
			CategoryID: category.CategoryID,
			Name:       "Parent inserted before failure",
			Sprints: []model.CreateSprintInput{
				{Name: "first succeeds", EstimatedDurationSeconds: 60},
				{Name: "explode", EstimatedDurationSeconds: 60},
			},
		}},
	})
	if err == nil || !validation.Valid {
		t.Fatalf("expected late storage error, not validation error: result=%#v validation=%#v err=%v", result, validation, err)
	}
	for _, table := range []string{"tasks", "subtasks", "sprints"} {
		var count int
		if err := database.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE project_id = ?", project.ProjectID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("late failure left %d rows in %s", count, table)
		}
	}
	var afterCounter int64
	if err := database.db.QueryRowContext(ctx, "SELECT next_value FROM id_counters WHERE name = 'item'").Scan(&afterCounter); err != nil {
		t.Fatal(err)
	}
	if afterCounter != beforeCounter {
		t.Errorf("late failure advanced item counter: %d -> %d", beforeCounter, afterCounter)
	}
	afterEvents, err := database.ListEventsAfterID(ctx, project.ProjectID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEvents) != len(beforeEvents) {
		t.Errorf("late failure emitted Project events: %d -> %d", len(beforeEvents), len(afterEvents))
	}
}
