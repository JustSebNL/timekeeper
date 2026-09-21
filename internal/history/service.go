// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package history

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

const MaxTrackedTextBytes = 10 * 1024 * 1024

type Service struct {
	root  string
	store *store.Store
}

type CaptureInput struct {
	ProjectID      string
	RelativePath   string
	FileType       string
	TrackingPolicy string
	AgentID        string
	SessionID      string
	Model          string
	ShadowboxRunID string
	TaskID         string
	SubtaskID      string
	SprintID       string
	CheckpointID   string
	ChangeSummary  string
	SnapshotType   string
}

type RestoreInput struct {
	FileID                   string
	VersionID                string
	ExpectedCurrentVersionID string
	AgentID                  string
	SessionID                string
	Model                    string
	TaskID                   string
	SubtaskID                string
	SprintID                 string
	CheckpointID             string
}

func New(root string, database *store.Store) (*Service, error) {
	root, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil || root == "" || database == nil {
		return nil, errors.New("history root and store are required")
	}
	return &Service{root: root, store: database}, nil
}

func (s *Service) Capture(ctx context.Context, input CaptureInput) (model.TrackedFile, model.FileVersion, error) {
	absolute, relative, err := s.resolveTrackedPath(input.RelativePath)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("read tracked file: %w", err)
	}
	if len(data) > MaxTrackedTextBytes {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("tracked file exceeds %d bytes", MaxTrackedTextBytes)
	}
	hash := sha256.Sum256(data)
	contentHash := "sha256:" + hex.EncodeToString(hash[:])
	file, err := s.store.TrackFile(ctx, model.TrackFileInput{ProjectID: input.ProjectID, CurrentPath: relative, FileType: input.FileType, TrackingPolicy: input.TrackingPolicy})
	if err != nil {
		file, err = s.store.GetTrackedFileByPath(ctx, input.ProjectID, relative)
		if err != nil {
			return model.TrackedFile{}, model.FileVersion{}, err
		}
	}
	versions, err := s.store.ListFileVersions(ctx, file.FileID)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	if len(versions) > 0 && versions[len(versions)-1].ContentHash == contentHash {
		return file, versions[len(versions)-1], nil
	}
	blobRef, err := s.writeBlob(contentHash, data)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	parent := ""
	if len(versions) > 0 {
		parent = versions[len(versions)-1].VersionID
	}
	snapshotType := strings.TrimSpace(input.SnapshotType)
	if snapshotType == "" {
		snapshotType = "full"
	}
	version, err := s.store.CreateFileVersion(ctx, model.CreateFileVersionInput{FileID: file.FileID, ContentHash: contentHash, ParentVersionID: parent, SnapshotType: snapshotType, BlobRef: blobRef, SizeBytes: int64(len(data)), AgentID: input.AgentID, SessionID: input.SessionID, Model: input.Model, ShadowboxRunID: input.ShadowboxRunID, TaskID: input.TaskID, SubtaskID: input.SubtaskID, SprintID: input.SprintID, CheckpointID: input.CheckpointID, ChangeSummary: input.ChangeSummary})
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	file.CurrentVersionID = version.VersionID
	return file, version, nil
}

func (s *Service) SaveContent(ctx context.Context, fileID, content, expectedCurrentVersionID, agentID, sessionID, modelName string) (model.TrackedFile, model.FileVersion, error) {
	file, err := s.store.GetTrackedFile(ctx, fileID)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	if expectedCurrentVersionID != "" && file.CurrentVersionID != expectedCurrentVersionID {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("%w: expected %s, current %s", store.ErrFileVersionConflict, expectedCurrentVersionID, file.CurrentVersionID)
	}
	if len([]byte(content)) > MaxTrackedTextBytes {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("content exceeds %d bytes", MaxTrackedTextBytes)
	}
	absolute, _, err := s.resolveTrackedPath(file.CurrentPath)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	original, err := os.ReadFile(absolute)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("read current file before save: %w", err)
	}
	if err := writeAtomic(absolute, []byte(content), 0o600); err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	_, version, err := s.Capture(ctx, CaptureInput{ProjectID: file.ProjectID, RelativePath: file.CurrentPath, FileType: file.FileType, TrackingPolicy: file.TrackingPolicy, AgentID: agentID, SessionID: sessionID, Model: modelName, ChangeSummary: "Editor save"})
	if err != nil {
		_ = writeAtomic(absolute, original, 0o600)
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("record editor save: %w", err)
	}
	file.CurrentVersionID = version.VersionID
	return file, version, nil
}

func (s *Service) Restore(ctx context.Context, input RestoreInput) (model.TrackedFile, model.FileVersion, error) {
	file, err := s.store.GetTrackedFile(ctx, input.FileID)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	if input.ExpectedCurrentVersionID != "" && file.CurrentVersionID != input.ExpectedCurrentVersionID {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("%w: expected %s, current %s", store.ErrFileVersionConflict, input.ExpectedCurrentVersionID, file.CurrentVersionID)
	}
	content, target, err := s.ReadVersion(ctx, input.FileID, input.VersionID)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	absolute, _, err := s.resolveTrackedPath(file.CurrentPath)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	original, err := os.ReadFile(absolute)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("read current file before restore: %w", err)
	}
	if err := writeAtomic(absolute, content, 0o600); err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	_, restored, err := s.Capture(ctx, CaptureInput{ProjectID: file.ProjectID, RelativePath: file.CurrentPath, FileType: file.FileType, TrackingPolicy: file.TrackingPolicy, AgentID: input.AgentID, SessionID: input.SessionID, Model: input.Model, TaskID: input.TaskID, SubtaskID: input.SubtaskID, SprintID: input.SprintID, CheckpointID: input.CheckpointID, ChangeSummary: "Restored " + target.VersionID})
	if err != nil {
		_ = writeAtomic(absolute, original, 0o600)
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("record restored file version: %w", err)
	}
	return file, restored, nil
}

func (s *Service) GetTrackedFile(ctx context.Context, fileID string) (model.TrackedFile, error) {
	return s.store.GetTrackedFile(ctx, fileID)
}

func (s *Service) ListTrackedFiles(ctx context.Context, projectID string) ([]model.TrackedFile, error) {
	return s.store.ListTrackedFiles(ctx, projectID)
}

func (s *Service) ListFileVersions(ctx context.Context, fileID string) ([]model.FileVersion, error) {
	return s.store.ListFileVersions(ctx, fileID)
}

func (s *Service) ReadVersion(ctx context.Context, fileID, versionID string) ([]byte, model.FileVersion, error) {
	versions, err := s.store.ListFileVersions(ctx, fileID)
	if err != nil {
		return nil, model.FileVersion{}, err
	}
	for _, version := range versions {
		if version.VersionID != versionID {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(s.root, version.BlobRef))
		if readErr != nil {
			return nil, model.FileVersion{}, fmt.Errorf("read version blob: %w", readErr)
		}
		return data, version, nil
	}
	return nil, model.FileVersion{}, fmt.Errorf("%w: version %s", store.ErrNotFound, versionID)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	temporary := path + fmt.Sprintf(".restore-%d", os.Getpid())
	if err := os.WriteFile(temporary, data, mode); err != nil {
		return fmt.Errorf("write atomic file: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("commit atomic file: %w", err)
	}
	return nil
}

func (s *Service) resolveTrackedPath(relative string) (string, string, error) {
	relative = filepath.Clean(strings.TrimSpace(relative))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", "", errors.New("tracked path must be relative to the history root")
	}
	absolute := filepath.Join(s.root, relative)
	return absolute, filepath.ToSlash(relative), nil
}

func (s *Service) writeBlob(hash string, data []byte) (string, error) {
	digest := strings.TrimPrefix(hash, "sha256:")
	if len(digest) < 4 {
		return "", errors.New("invalid content hash")
	}
	relative := filepath.Join("history", "blobs", "sha256", digest[:2], digest[2:4], digest)
	absolute := filepath.Join(s.root, relative)
	if _, err := os.Stat(absolute); err == nil {
		return filepath.ToSlash(relative), nil
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", fmt.Errorf("create history blob directory: %w", err)
	}
	temporary := absolute + fmt.Sprintf(".tmp-%d", os.Getpid())
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return "", fmt.Errorf("write history blob: %w", err)
	}
	if err := os.Rename(temporary, absolute); err != nil {
		_ = os.Remove(temporary)
		return "", fmt.Errorf("commit history blob: %w", err)
	}
	return filepath.ToSlash(relative), nil
}
