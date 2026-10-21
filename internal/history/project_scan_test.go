// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package history_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/history"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectScanIndexesTextFilesAndCapturesChangedContents(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	projectRoot := filepath.Join(workspace, "source")
	if err := os.MkdirAll(filepath.Join(projectRoot, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "node_modules", "ignored"), 0o700); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectRoot, "src", "main.go")
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# Project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "node_modules", "ignored", "vendor.js"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "logo.bin"), []byte{0, 1, 2}, 0o600); err != nil {
		t.Fatal(err)
	}

	database, err := store.Open(filepath.Join(workspace, "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Project scan"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := history.New(filepath.Join(workspace, "history"), database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetProjectRoot(ctx, project.ProjectID, projectRoot); err != nil {
		t.Fatal(err)
	}

	first, err := service.ScanProjectFiles(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if first.FilesDiscovered != 2 || first.FilesAdded != 2 || first.VersionsAdded != 2 || first.FilesSkipped != 1 {
		t.Fatalf("first scan = %+v", first)
	}
	tracked, err := database.GetTrackedFileByPath(ctx, project.ProjectID, "src/main.go")
	if err != nil || tracked.SourceRoot != projectRoot {
		t.Fatalf("tracked file=%+v err=%v", tracked, err)
	}

	if err := os.WriteFile(mainPath, []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := service.ScanProjectFiles(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if second.FilesDiscovered != 2 || second.FilesAdded != 0 || second.VersionsAdded != 1 {
		t.Fatalf("second scan = %+v", second)
	}
	versions, err := database.ListFileVersions(ctx, tracked.FileID)
	if err != nil || len(versions) != 2 || versions[1].ParentVersionID != versions[0].VersionID {
		t.Fatalf("versions=%+v err=%v", versions, err)
	}
	content, _, err := service.ReadVersion(ctx, tracked.FileID, versions[1].VersionID)
	if err != nil || string(content) != "package changed\n" {
		t.Fatalf("second snapshot=%q err=%v", content, err)
	}
}
