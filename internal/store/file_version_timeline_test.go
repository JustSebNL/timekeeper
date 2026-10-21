// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestCreateFileVersionPersistsChangeKindAndRestoreTarget(t *testing.T) {
	ctx := context.Background()
	database, err := Open(t.TempDir() + "/file-branch.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "File branch history"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := database.TrackFile(ctx, model.TrackFileInput{ProjectID: project.ProjectID, CurrentPath: "src/example.go"})
	if err != nil {
		t.Fatal(err)
	}
	base, err := database.CreateFileVersion(ctx, model.CreateFileVersionInput{FileID: file.FileID, ContentHash: "sha256:base", BlobRef: "blob/base", SizeBytes: 4, ChangeKind: model.FileChangeFeature})
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := database.CreateFileVersion(ctx, model.CreateFileVersionInput{FileID: file.FileID, ContentHash: "sha256:restore", BlobRef: "blob/restore", SizeBytes: 4, ParentVersionID: base.VersionID, RestoresVersionID: base.VersionID, ChangeKind: model.FileChangeReversion})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := database.ListFileVersions(ctx, file.FileID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].ChangeKind != model.FileChangeFeature || versions[1].ChangeKind != model.FileChangeReversion || versions[1].RestoresVersionID != base.VersionID || reverted.RestoresVersionID != base.VersionID {
		t.Fatalf("versions did not preserve event/branch metadata: %#v", versions)
	}
	other, err := database.TrackFile(ctx, model.TrackFileInput{ProjectID: project.ProjectID, CurrentPath: "src/other.go"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateFileVersion(ctx, model.CreateFileVersionInput{FileID: other.FileID, ContentHash: "sha256:cross", BlobRef: "blob/cross", SizeBytes: 4, ChangeKind: model.FileChangeReversion, RestoresVersionID: base.VersionID})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("cross-file revert target error=%v, want validation error", err)
	}
}

func TestMigrationBackfillsHistoricalRestoreBranchFromSummary(t *testing.T) {
	ctx := context.Background()
	database, err := Open(t.TempDir() + "/legacy-file-branch.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Legacy branch"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := database.TrackFile(ctx, model.TrackFileInput{ProjectID: project.ProjectID, CurrentPath: "legacy.txt"})
	if err != nil {
		t.Fatal(err)
	}
	base, err := database.CreateFileVersion(ctx, model.CreateFileVersionInput{FileID: file.FileID, ContentHash: "sha256:base", BlobRef: "blob/base", SizeBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	legacyRevert, err := database.CreateFileVersion(ctx, model.CreateFileVersionInput{FileID: file.FileID, ContentHash: "sha256:revert", BlobRef: "blob/revert", SizeBytes: 1, ParentVersionID: base.VersionID, ChangeSummary: "Restored " + base.VersionID})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	versions, err := database.ListFileVersions(ctx, file.FileID)
	if err != nil {
		t.Fatal(err)
	}
	got := versions[len(versions)-1]
	if got.VersionID != legacyRevert.VersionID || got.ChangeKind != model.FileChangeReversion || got.RestoresVersionID != base.VersionID {
		t.Fatalf("legacy reversion was not backfilled: %+v", got)
	}
}

func TestFileVersionsSchemaStoresChangeKindAndRestoreBranch(t *testing.T) {
	database, err := Open(t.TempDir() + "/file-timeline.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	rows, err := database.db.QueryContext(context.Background(), "PRAGMA table_info(file_versions)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"change_kind", "restores_version_id"} {
		if !columns[column] {
			t.Errorf("file_versions schema missing %q; timeline cannot preserve typed events and revert branches", column)
		}
	}
}
