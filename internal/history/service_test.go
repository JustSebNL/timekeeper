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

func TestCaptureVersionsAndReadBack(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "src", "example.go")
	if err := os.WriteFile(filePath, []byte("package v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(filepath.Join(root, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "File history"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := history.New(root, database)
	if err != nil {
		t.Fatal(err)
	}

	file, first, err := service.Capture(ctx, history.CaptureInput{ProjectID: project.ProjectID, RelativePath: "src/example.go", FileType: "go", ChangeSummary: "Initial file"})
	if err != nil {
		t.Fatal(err)
	}
	if file.FileID == "" || first.VersionNumber != 1 {
		t.Fatalf("first=%#v file=%#v", first, file)
	}
	if err := os.WriteFile(filePath, []byte("package v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, second, err := service.Capture(ctx, history.CaptureInput{ProjectID: project.ProjectID, RelativePath: "src/example.go", FileType: "go", ChangeSummary: "Second version"})
	if err != nil {
		t.Fatal(err)
	}
	if second.VersionNumber != 2 || second.ParentVersionID != first.VersionID {
		t.Fatalf("second=%#v", second)
	}
	versions, err := database.ListFileVersions(ctx, file.FileID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions=%#v err=%v", versions, err)
	}
	content, _, err := service.ReadVersion(ctx, file.FileID, first.VersionID)
	if err != nil || string(content) != "package v1\n" {
		t.Fatalf("read v1=%q err=%v", content, err)
	}
	content, _, err = service.ReadVersion(ctx, file.FileID, second.VersionID)
	if err != nil || string(content) != "package v2\n" {
		t.Fatalf("read v2=%q err=%v", content, err)
	}
	if _, err := database.CreateFileVersion(ctx, model.CreateFileVersionInput{FileID: file.FileID, ContentHash: "sha256:stale", BlobRef: "history/blobs/stale", ParentVersionID: first.VersionID}); !errors.Is(err, store.ErrFileVersionConflict) {
		t.Fatalf("conflict=%v", err)
	}
}

func TestRestoreCreatesNewVersion(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	filePath := filepath.Join(root, "example.txt")
	if err := os.WriteFile(filePath, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(filepath.Join(root, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Restore history"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := history.New(root, database)
	if err != nil {
		t.Fatal(err)
	}
	tracked, first, err := service.Capture(ctx, history.CaptureInput{ProjectID: project.ProjectID, RelativePath: "example.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, second, err := service.Capture(ctx, history.CaptureInput{ProjectID: project.ProjectID, RelativePath: "example.txt"})
	if err != nil {
		t.Fatal(err)
	}
	modeBefore, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	_, saved, err := service.SaveContent(ctx, tracked.FileID, "three\n", second.VersionID, "agent", "session", "model", model.FileChangeBugfix)
	if err != nil || saved.VersionNumber != 3 || saved.ChangeKind != model.FileChangeBugfix {
		t.Fatalf("saved=%#v err=%v", saved, err)
	}
	modeAfterSave, err := os.Stat(filePath)
	if err != nil || modeAfterSave.Mode().Perm() != modeBefore.Mode().Perm() {
		t.Fatalf("save changed file permissions: before=%v after=%v err=%v", modeBefore.Mode().Perm(), modeAfterSave.Mode().Perm(), err)
	}
	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}
	_, restored, err := service.Restore(ctx, history.RestoreInput{FileID: tracked.FileID, VersionID: first.VersionID, ExpectedCurrentVersionID: saved.VersionID})
	if err != nil {
		t.Fatal(err)
	}
	if restored.VersionNumber != 4 || restored.ParentVersionID != saved.VersionID || restored.RestoresVersionID != first.VersionID || restored.ChangeKind != model.FileChangeReversion {
		t.Fatalf("restored=%#v", restored)
	}
	content, err := os.ReadFile(filePath)
	if err != nil || string(content) != "one\n" {
		t.Fatalf("file=%q err=%v", content, err)
	}
	restoredInfo, err := os.Stat(filePath)
	if err != nil || restoredInfo.Mode().Perm() != modeBefore.Mode().Perm() {
		t.Fatalf("restore changed file permissions: before=%v after=%v err=%v", modeBefore.Mode().Perm(), restoredInfo.Mode().Perm(), err)
	}
	versions, err := database.ListFileVersions(ctx, tracked.FileID)
	if err != nil || len(versions) != 4 {
		t.Fatalf("versions=%#v err=%v", versions, err)
	}
}

func TestSaveContentPreservesExternalEditAsHistoryBeforeConflict(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	filePath := filepath.Join(root, "shared.go")
	if err := os.WriteFile(filePath, []byte("package original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(filepath.Join(root, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "External editor conflict"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := history.New(root, database)
	if err != nil {
		t.Fatal(err)
	}
	tracked, initial, err := service.Capture(ctx, history.CaptureInput{ProjectID: project.ProjectID, RelativePath: "shared.go", FileType: "go"})
	if err != nil {
		t.Fatal(err)
	}
	external := []byte("package external\n")
	if err := os.WriteFile(filePath, external, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.SaveContent(ctx, tracked.FileID, "package editor\n", initial.VersionID, "agent", "session", "model", model.FileChangeCodeChange); !errors.Is(err, store.ErrFileVersionConflict) {
		t.Fatalf("external edit save error=%v", err)
	}
	actual, err := os.ReadFile(filePath)
	if err != nil || string(actual) != string(external) {
		t.Fatalf("external file changed=%q err=%v", actual, err)
	}
	updated, err := database.GetTrackedFile(ctx, tracked.FileID)
	if err != nil || updated.CurrentVersionID == initial.VersionID {
		t.Fatalf("external edit was not added to history: file=%+v err=%v", updated, err)
	}
	content, version, err := service.ReadVersion(ctx, tracked.FileID, updated.CurrentVersionID)
	if err != nil || string(content) != string(external) || version.ParentVersionID != initial.VersionID {
		t.Fatalf("external history version=%+v content=%q err=%v", version, content, err)
	}
}

func TestHistoryServiceRejectsSymlinkTraversal(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(root, "symlink.txt")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	database, err := store.Open(filepath.Join(root, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	service, err := history.New(root, database)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = service.Capture(context.Background(), history.CaptureInput{
		ProjectID:    "P-10001",
		RelativePath: "symlink.txt",
		FileType:     "txt",
	})
	if err == nil {
		t.Fatal("expected error capturing symlink target outside history root, got nil")
	}
}
