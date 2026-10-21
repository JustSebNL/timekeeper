// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package history_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/history"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestReadVersionRejectsBlobPathEscape(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := filepath.Join(root, "outside-secret.txt")
	if err := os.WriteFile(outside, []byte("must not be read"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(filepath.Join(root, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Blob boundary"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := database.TrackFile(ctx, model.TrackFileInput{ProjectID: project.ProjectID, CurrentPath: "main.go"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := database.CreateFileVersion(ctx, model.CreateFileVersionInput{
		FileID:      file.FileID,
		ContentHash: "sha256:outside",
		BlobRef:     "../outside-secret.txt",
		SizeBytes:   int64(len("must not be read")),
	})
	if err != nil {
		t.Fatal(err)
	}
	serviceRoot := filepath.Join(root, "history")
	if err := os.MkdirAll(serviceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	service, err := history.New(serviceRoot, database)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.ReadVersion(ctx, file.FileID, version.VersionID)
	if err == nil {
		t.Fatal("expected a blob path escape to be rejected")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escape was treated as a missing blob instead of rejected: %v", err)
	}
}
