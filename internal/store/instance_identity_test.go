package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceIDIsStablePerDatabaseAndDistinctBetweenDatabases(t *testing.T) {
	ctx := context.Background()
	firstPath := filepath.Join(t.TempDir(), "first.db")
	first, err := Open(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := first.InstanceID(ctx)
	if err != nil {
		t.Fatalf("read first instance ID: %v", err)
	}
	if !strings.HasPrefix(firstID, "tk_instance_") || len(firstID) != len("tk_instance_")+32 {
		t.Fatalf("unexpected instance ID format %q", firstID)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	first, err = Open(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	reopenedID, err := first.InstanceID(ctx)
	if err != nil {
		t.Fatalf("read reopened instance ID: %v", err)
	}
	if reopenedID != firstID {
		t.Fatalf("instance ID changed across reopen: first=%q reopened=%q", firstID, reopenedID)
	}

	second, err := Open(filepath.Join(t.TempDir(), "second.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	secondID, err := second.InstanceID(ctx)
	if err != nil {
		t.Fatalf("read second instance ID: %v", err)
	}
	if secondID == firstID {
		t.Fatalf("independent database instances share ID %q", firstID)
	}
}

func TestInstanceIdentityMigrationRestoresMissingIdentityTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(ctx, `DROP TABLE instance_identity`); err != nil {
		database.Close()
		t.Fatalf("simulate pre-identity schema: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = Open(path)
	if err != nil {
		t.Fatalf("reopen legacy database: %v", err)
	}
	defer database.Close()
	id, err := database.InstanceID(ctx)
	if err != nil {
		t.Fatalf("read migrated instance ID: %v", err)
	}
	if !strings.HasPrefix(id, "tk_instance_") || len(id) != len("tk_instance_")+32 {
		t.Fatalf("migration produced invalid instance ID %q", id)
	}
}
