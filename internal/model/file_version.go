// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

type FileChangeKind string

const (
	FileChangeMilestone     FileChangeKind = "milestone"
	FileChangeConcept       FileChangeKind = "concept"
	FileChangeAlphaTesting  FileChangeKind = "alpha_testing"
	FileChangeBetaTesting   FileChangeKind = "beta_testing"
	FileChangeTrail         FileChangeKind = "trail"
	FileChangeBugfix        FileChangeKind = "bugfix"
	FileChangeCodeChange    FileChangeKind = "code_change"
	FileChangeReversion     FileChangeKind = "reversion"
	FileChangeFeature       FileChangeKind = "feature"
	FileChangeRelease       FileChangeKind = "release"
	FileChangeFeatureEOL    FileChangeKind = "feature_eol"
	FileChangeFileAdded     FileChangeKind = "file_added"
	FileChangeRefactor      FileChangeKind = "refactor"
	FileChangeTest          FileChangeKind = "test"
	FileChangeDocumentation FileChangeKind = "documentation"
	FileChangeSecurity      FileChangeKind = "security"
	FileChangePerformance   FileChangeKind = "performance"
	FileChangeDependency    FileChangeKind = "dependency"
	FileChangeConfiguration FileChangeKind = "configuration"
	FileChangeOther         FileChangeKind = "other"
)

var ValidFileChangeKinds = []FileChangeKind{
	FileChangeMilestone, FileChangeBugfix, FileChangeCodeChange, FileChangeReversion,
	FileChangeConcept, FileChangeAlphaTesting, FileChangeBetaTesting, FileChangeTrail,
	FileChangeFeature, FileChangeRelease, FileChangeFeatureEOL, FileChangeFileAdded, FileChangeRefactor,
	FileChangeTest, FileChangeDocumentation, FileChangeSecurity, FileChangePerformance,
	FileChangeDependency, FileChangeConfiguration, FileChangeOther,
}

func IsValidFileChangeKind(kind FileChangeKind) bool {
	for _, valid := range ValidFileChangeKinds {
		if kind == valid {
			return true
		}
	}
	return false
}

type ProjectFileRoot struct {
	ProjectID string    `json:"project_id"`
	RootPath  string    `json:"root_path"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TrackedFile struct {
	FileID           string    `json:"file_id"`
	ProjectID        string    `json:"project_id"`
	CurrentPath      string    `json:"current_path"`
	FileType         string    `json:"file_type"`
	CreatedAt        time.Time `json:"created_at"`
	CurrentVersionID string    `json:"current_version_id,omitempty"`
	TrackingPolicy   string    `json:"tracking_policy"`
	SourceRoot       string    `json:"-"`
}

type FileVersion struct {
	VersionID         string         `json:"version_id"`
	FileID            string         `json:"file_id"`
	VersionNumber     int64          `json:"version_number"`
	ContentHash       string         `json:"content_hash"`
	ParentVersionID   string         `json:"parent_version_id,omitempty"`
	RestoresVersionID string         `json:"restores_version_id,omitempty"`
	ChangeKind        FileChangeKind `json:"change_kind"`
	SnapshotType      string         `json:"snapshot_type"`
	BlobRef           string         `json:"blob_ref"`
	SizeBytes         int64          `json:"size_bytes"`
	CreatedAt         time.Time      `json:"created_at"`
	AgentID           string         `json:"agent_id,omitempty"`
	SessionID         string         `json:"session_id,omitempty"`
	Model             string         `json:"model,omitempty"`
	ShadowboxRunID    string         `json:"shadowbox_run_id,omitempty"`
	TaskID            string         `json:"task_id,omitempty"`
	SubtaskID         string         `json:"subtask_id,omitempty"`
	SprintID          string         `json:"sprint_id,omitempty"`
	CheckpointID      string         `json:"checkpoint_id,omitempty"`
	ChangeSummary     string         `json:"change_summary,omitempty"`
	Status            string         `json:"status"`
}

type TrackFileInput struct {
	ProjectID      string `json:"project_id"`
	CurrentPath    string `json:"current_path"`
	FileType       string `json:"file_type"`
	TrackingPolicy string `json:"tracking_policy"`
	SourceRoot     string `json:"-"`
}

type CreateFileVersionInput struct {
	FileID            string
	ContentHash       string
	ParentVersionID   string
	RestoresVersionID string
	ChangeKind        FileChangeKind
	SnapshotType      string
	BlobRef           string
	SizeBytes         int64
	AgentID           string
	SessionID         string
	Model             string
	ShadowboxRunID    string
	TaskID            string
	SubtaskID         string
	SprintID          string
	CheckpointID      string
	ChangeSummary     string
	Status            string
}
