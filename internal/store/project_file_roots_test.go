// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestProjectFileRootAndTrackedFilesStayProjectBound(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "project-roots.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Rooted files"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := database.SetProjectFileRoot(ctx, project.ProjectID, root); err != nil {
		t.Fatal(err)
	}
	gotRoot, err := database.GetProjectFileRoot(ctx, project.ProjectID)
	if err != nil || gotRoot.RootPath != root {
		t.Fatalf("root=%+v err=%v", gotRoot, err)
	}
	file, err := database.TrackFile(ctx, model.TrackFileInput{ProjectID: project.ProjectID, CurrentPath: "src/main.go", SourceRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := database.GetTrackedFile(ctx, file.FileID)
	if err != nil || loaded.SourceRoot != root {
		t.Fatalf("tracked source root=%q err=%v", loaded.SourceRoot, err)
	}
	files, err := database.ListTrackedFiles(ctx, project.ProjectID)
	if err != nil || len(files) != 1 || files[0].SourceRoot != root {
		t.Fatalf("listed files=%+v err=%v", files, err)
	}
	if err := database.SetProjectFileRoot(ctx, project.ProjectID, t.TempDir()); !errors.Is(err, ErrValidation) {
		t.Fatalf("changing a root with tracked files error=%v", err)
	}
}
