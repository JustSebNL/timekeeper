// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

type TrackedFile struct {
	FileID           string    `json:"file_id"`
	ProjectID        string    `json:"project_id"`
	CurrentPath      string    `json:"current_path"`
	FileType         string    `json:"file_type"`
	CreatedAt        time.Time `json:"created_at"`
	CurrentVersionID string    `json:"current_version_id,omitempty"`
	TrackingPolicy   string    `json:"tracking_policy"`
}

type FileVersion struct {
	VersionID       string    `json:"version_id"`
	FileID          string    `json:"file_id"`
	VersionNumber   int64     `json:"version_number"`
	ContentHash     string    `json:"content_hash"`
	ParentVersionID string    `json:"parent_version_id,omitempty"`
	SnapshotType    string    `json:"snapshot_type"`
	BlobRef         string    `json:"blob_ref"`
	SizeBytes       int64     `json:"size_bytes"`
	CreatedAt       time.Time `json:"created_at"`
	AgentID         string    `json:"agent_id,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	Model           string    `json:"model,omitempty"`
	ShadowboxRunID  string    `json:"shadowbox_run_id,omitempty"`
	TaskID          string    `json:"task_id,omitempty"`
	SubtaskID       string    `json:"subtask_id,omitempty"`
	SprintID        string    `json:"sprint_id,omitempty"`
	CheckpointID    string    `json:"checkpoint_id,omitempty"`
	ChangeSummary   string    `json:"change_summary,omitempty"`
	Status          string    `json:"status"`
}

type TrackFileInput struct {
	ProjectID      string `json:"project_id"`
	CurrentPath    string `json:"current_path"`
	FileType       string `json:"file_type"`
	TrackingPolicy string `json:"tracking_policy"`
}

type CreateFileVersionInput struct {
	FileID          string
	ContentHash     string
	ParentVersionID string
	SnapshotType    string
	BlobRef         string
	SizeBytes       int64
	AgentID         string
	SessionID       string
	Model           string
	ShadowboxRunID  string
	TaskID          string
	SubtaskID       string
	SprintID        string
	CheckpointID    string
	ChangeSummary   string
	Status          string
}
