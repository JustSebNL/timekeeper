// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

var ErrFileVersionConflict = errors.New("file version conflict")

func (s *Store) TrackFile(ctx context.Context, input model.TrackFileInput) (model.TrackedFile, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.CurrentPath = strings.TrimSpace(input.CurrentPath)
	input.FileType = strings.TrimSpace(input.FileType)
	input.TrackingPolicy = strings.TrimSpace(input.TrackingPolicy)
	if input.ProjectID == "" || input.CurrentPath == "" {
		return model.TrackedFile{}, fmt.Errorf("%w: project and path are required", ErrValidation)
	}
	if input.FileType == "" {
		input.FileType = "text"
	}
	if input.TrackingPolicy == "" {
		input.TrackingPolicy = "manual"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.TrackedFile{}, fmt.Errorf("begin tracked file: %w", err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", input.ProjectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.TrackedFile{}, fmt.Errorf("%w: project %s", ErrNotFound, input.ProjectID)
		}
		return model.TrackedFile{}, fmt.Errorf("check tracked file project: %w", err)
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.TrackedFile{}, err
	}
	fileID := fmt.Sprintf("F-%d", number)
	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, `INSERT INTO tracked_files (file_id, project_id, current_path, file_type, created_at, current_version_id, tracking_policy) VALUES (?, ?, ?, ?, ?, '', ?)`, fileID, input.ProjectID, input.CurrentPath, input.FileType, stamp(now), input.TrackingPolicy); err != nil {
		return model.TrackedFile{}, fmt.Errorf("insert tracked file: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.TrackedFile{}, fmt.Errorf("commit tracked file: %w", err)
	}
	return model.TrackedFile{FileID: fileID, ProjectID: input.ProjectID, CurrentPath: input.CurrentPath, FileType: input.FileType, CreatedAt: now, TrackingPolicy: input.TrackingPolicy}, nil
}

func (s *Store) ListTrackedFiles(ctx context.Context, projectID string) ([]model.TrackedFile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT file_id, project_id, current_path, file_type, created_at, current_version_id, tracking_policy FROM tracked_files WHERE project_id = ? ORDER BY current_path ASC, file_id ASC`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, fmt.Errorf("list tracked files: %w", err)
	}
	defer rows.Close()
	var files []model.TrackedFile
	for rows.Next() {
		var file model.TrackedFile
		var created string
		if err := rows.Scan(&file.FileID, &file.ProjectID, &file.CurrentPath, &file.FileType, &created, &file.CurrentVersionID, &file.TrackingPolicy); err != nil {
			return nil, fmt.Errorf("scan tracked file: %w", err)
		}
		file.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse tracked file created_at: %w", err)
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (s *Store) GetTrackedFileByPath(ctx context.Context, projectID, currentPath string) (model.TrackedFile, error) {
	var file model.TrackedFile
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT file_id, project_id, current_path, file_type, created_at, current_version_id, tracking_policy FROM tracked_files WHERE project_id = ? AND current_path = ?`, strings.TrimSpace(projectID), strings.TrimSpace(currentPath)).Scan(&file.FileID, &file.ProjectID, &file.CurrentPath, &file.FileType, &created, &file.CurrentVersionID, &file.TrackingPolicy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.TrackedFile{}, fmt.Errorf("%w: file path %s", ErrNotFound, currentPath)
		}
		return model.TrackedFile{}, fmt.Errorf("get tracked file by path: %w", err)
	}
	file.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return model.TrackedFile{}, fmt.Errorf("parse tracked file created_at: %w", err)
	}
	return file, nil
}

func (s *Store) GetTrackedFile(ctx context.Context, fileID string) (model.TrackedFile, error) {
	var file model.TrackedFile
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT file_id, project_id, current_path, file_type, created_at, current_version_id, tracking_policy FROM tracked_files WHERE file_id = ?`, strings.TrimSpace(fileID)).Scan(&file.FileID, &file.ProjectID, &file.CurrentPath, &file.FileType, &created, &file.CurrentVersionID, &file.TrackingPolicy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.TrackedFile{}, fmt.Errorf("%w: file %s", ErrNotFound, fileID)
		}
		return model.TrackedFile{}, fmt.Errorf("get tracked file: %w", err)
	}
	var parseErr error
	file.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, created)
	if parseErr != nil {
		return model.TrackedFile{}, fmt.Errorf("parse tracked file created_at: %w", parseErr)
	}
	return file, nil
}

func (s *Store) CreateFileVersion(ctx context.Context, input model.CreateFileVersionInput) (model.FileVersion, error) {
	input.FileID = strings.TrimSpace(input.FileID)
	input.ContentHash = strings.TrimSpace(input.ContentHash)
	input.ParentVersionID = strings.TrimSpace(input.ParentVersionID)
	input.SnapshotType = strings.TrimSpace(input.SnapshotType)
	input.BlobRef = strings.TrimSpace(input.BlobRef)
	if input.FileID == "" || input.ContentHash == "" || input.BlobRef == "" || input.SizeBytes < 0 {
		return model.FileVersion{}, fmt.Errorf("%w: file version identity/content/blob are required", ErrValidation)
	}
	if input.SnapshotType == "" {
		input.SnapshotType = "full"
	}
	if input.SnapshotType != "full" && input.SnapshotType != "delta" {
		return model.FileVersion{}, fmt.Errorf("%w: unsupported snapshot type", ErrValidation)
	}
	if input.Status == "" {
		input.Status = "accepted"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.FileVersion{}, fmt.Errorf("begin file version: %w", err)
	}
	defer tx.Rollback()
	var current string
	if err := tx.QueryRowContext(ctx, "SELECT current_version_id FROM tracked_files WHERE file_id = ?", input.FileID).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.FileVersion{}, fmt.Errorf("%w: file %s", ErrNotFound, input.FileID)
		}
		return model.FileVersion{}, fmt.Errorf("read tracked file version: %w", err)
	}
	if input.ParentVersionID != "" && current != input.ParentVersionID {
		return model.FileVersion{}, fmt.Errorf("%w: expected %s, current %s", ErrFileVersionConflict, input.ParentVersionID, current)
	}
	if input.ParentVersionID == "" && current != "" {
		return model.FileVersion{}, fmt.Errorf("%w: parent version is required", ErrFileVersionConflict)
	}
	var versionNumber int64
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version_number), 0) + 1 FROM file_versions WHERE file_id = ?", input.FileID).Scan(&versionNumber); err != nil {
		return model.FileVersion{}, fmt.Errorf("allocate file version number: %w", err)
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.FileVersion{}, err
	}
	versionID := fmt.Sprintf("FV-%d", number)
	now := time.Now().UTC().Round(0)
	_, err = tx.ExecContext(ctx, `INSERT INTO file_versions (version_id, file_id, version_number, content_hash, parent_version_id, snapshot_type, blob_ref, size_bytes, created_at, agent_id, session_id, model, shadowbox_run_id, task_id, subtask_id, sprint_id, checkpoint_id, change_summary, status) VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, versionID, input.FileID, versionNumber, input.ContentHash, input.ParentVersionID, input.SnapshotType, input.BlobRef, input.SizeBytes, stamp(now), input.AgentID, input.SessionID, input.Model, input.ShadowboxRunID, input.TaskID, input.SubtaskID, input.SprintID, input.CheckpointID, input.ChangeSummary, input.Status)
	if err != nil {
		return model.FileVersion{}, fmt.Errorf("insert file version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE tracked_files SET current_version_id = ? WHERE file_id = ?", versionID, input.FileID); err != nil {
		return model.FileVersion{}, fmt.Errorf("update current file version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.FileVersion{}, fmt.Errorf("commit file version: %w", err)
	}
	return model.FileVersion{VersionID: versionID, FileID: input.FileID, VersionNumber: versionNumber, ContentHash: input.ContentHash, ParentVersionID: input.ParentVersionID, SnapshotType: input.SnapshotType, BlobRef: input.BlobRef, SizeBytes: input.SizeBytes, CreatedAt: now, AgentID: input.AgentID, SessionID: input.SessionID, Model: input.Model, ShadowboxRunID: input.ShadowboxRunID, TaskID: input.TaskID, SubtaskID: input.SubtaskID, SprintID: input.SprintID, CheckpointID: input.CheckpointID, ChangeSummary: input.ChangeSummary, Status: input.Status}, nil
}

func (s *Store) ListFileVersions(ctx context.Context, fileID string) ([]model.FileVersion, error) {
	if _, err := s.GetTrackedFile(ctx, fileID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT version_id, file_id, version_number, content_hash, COALESCE(parent_version_id, ''), snapshot_type, blob_ref, size_bytes, created_at, agent_id, session_id, model, shadowbox_run_id, task_id, subtask_id, sprint_id, checkpoint_id, change_summary, status FROM file_versions WHERE file_id = ? ORDER BY version_number ASC`, strings.TrimSpace(fileID))
	if err != nil {
		return nil, fmt.Errorf("list file versions: %w", err)
	}
	defer rows.Close()
	var versions []model.FileVersion
	for rows.Next() {
		var version model.FileVersion
		var created string
		if err := rows.Scan(&version.VersionID, &version.FileID, &version.VersionNumber, &version.ContentHash, &version.ParentVersionID, &version.SnapshotType, &version.BlobRef, &version.SizeBytes, &created, &version.AgentID, &version.SessionID, &version.Model, &version.ShadowboxRunID, &version.TaskID, &version.SubtaskID, &version.SprintID, &version.CheckpointID, &version.ChangeSummary, &version.Status); err != nil {
			return nil, fmt.Errorf("scan file version: %w", err)
		}
		version.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse file version created_at: %w", err)
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}
