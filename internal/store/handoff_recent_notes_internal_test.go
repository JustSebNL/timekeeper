package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestGetHandoffSnapshotReturnsNewestFiveNotes(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "handoff-notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Recent notes"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 7; i++ {
		createdAt := time.Date(2026, 10, 18, 10, 0, i, 0, time.UTC).Format(time.RFC3339Nano)
		if _, err := database.db.ExecContext(ctx, `INSERT INTO project_notes(project_id, content, actor_id, created_at) VALUES (?, ?, ?, ?)`, project.ProjectID, fmt.Sprintf("note-%d", i), "tester", createdAt); err != nil {
			t.Fatalf("insert note %d: %v", i, err)
		}
	}

	snapshot, err := database.GetHandoffSnapshot(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.RecentNotes) != 5 {
		t.Fatalf("recent notes length = %d, want 5: %+v", len(snapshot.RecentNotes), snapshot.RecentNotes)
	}
	if snapshot.RecentNotes[0].Content != "note-7" || snapshot.RecentNotes[4].Content != "note-3" {
		t.Fatalf("handoff notes were not the newest five in newest-first order: %+v", snapshot.RecentNotes)
	}
}
