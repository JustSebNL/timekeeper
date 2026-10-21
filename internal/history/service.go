// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

const MaxTrackedTextBytes = 10 * 1024 * 1024
const MaxProjectScanFiles = 10000
const MaxProjectScanBytes = 512 << 20

type Service struct {
	root  string
	store *store.Store
}

type ProjectScanResult struct {
	ProjectID       string `json:"project_id"`
	RootPath        string `json:"root_path"`
	FilesDiscovered int    `json:"files_discovered"`
	FilesAdded      int    `json:"files_added"`
	VersionsAdded   int    `json:"versions_added"`
	FilesSkipped    int    `json:"files_skipped"`
}

type CaptureInput struct {
	ProjectID         string
	RelativePath      string
	FileType          string
	TrackingPolicy    string
	AgentID           string
	SessionID         string
	Model             string
	ShadowboxRunID    string
	TaskID            string
	SubtaskID         string
	SprintID          string
	CheckpointID      string
	ChangeSummary     string
	ChangeKind        model.FileChangeKind
	RestoresVersionID string
	SnapshotType      string
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

func (s *Service) SetProjectRoot(ctx context.Context, projectID, rootPath string) (model.ProjectFileRoot, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" || !filepath.IsAbs(rootPath) {
		return model.ProjectFileRoot{}, fmt.Errorf("%w: project source root must be an absolute existing directory", store.ErrValidation)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(rootPath))
	if err != nil {
		return model.ProjectFileRoot{}, fmt.Errorf("%w: project source root is unavailable or inaccessible", store.ErrValidation)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return model.ProjectFileRoot{}, fmt.Errorf("%w: project source root must be a directory", store.ErrValidation)
	}
	if err := s.store.SetProjectFileRoot(ctx, projectID, resolved); err != nil {
		return model.ProjectFileRoot{}, err
	}
	return s.store.GetProjectFileRoot(ctx, projectID)
}

func (s *Service) GetProjectRoot(ctx context.Context, projectID string) (model.ProjectFileRoot, error) {
	return s.store.GetProjectFileRoot(ctx, projectID)
}

func (s *Service) ScanProjectFiles(ctx context.Context, projectID string) (ProjectScanResult, error) {
	projectID = strings.TrimSpace(projectID)
	root, err := s.store.GetProjectFileRoot(ctx, projectID)
	if errors.Is(err, store.ErrNotFound) {
		return ProjectScanResult{ProjectID: projectID}, fmt.Errorf("%w: set a project source root before scanning", store.ErrValidation)
	}
	if err != nil {
		return ProjectScanResult{}, err
	}
	result := ProjectScanResult{ProjectID: projectID, RootPath: root.RootPath}
	ignoredDirectories := map[string]bool{
		".git": true, ".hg": true, ".svn": true, ".timekeeper": true,
		"node_modules": true, "vendor": true, "dist": true, "build": true,
		"target": true, "coverage": true, "out": true, "bin": true, "obj": true,
		".venv": true, "venv": true, "__pycache__": true,
	}
	historyRelative, historyInsideRoot := "", false
	if historyRoot, err := filepath.EvalSymlinks(s.root); err == nil {
		if relative, relErr := filepath.Rel(root.RootPath, historyRoot); relErr == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative) {
			historyRelative = filepath.Clean(relative)
			historyInsideRoot = true
		}
	}
	var totalBytes int64
	err = filepath.WalkDir(root.RootPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if walkErr != nil {
			if path == root.RootPath {
				return walkErr
			}
			result.FilesSkipped++
			return nil
		}
		relative, err := filepath.Rel(root.RootPath, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root.RootPath && ignoredDirectories[strings.ToLower(entry.Name())] {
				return filepath.SkipDir
			}
			if historyInsideRoot && filepath.Clean(relative) == historyRelative {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			result.FilesSkipped++
			return nil
		}
		if result.FilesDiscovered >= MaxProjectScanFiles {
			return fmt.Errorf("project scan exceeded limits (%d files)", MaxProjectScanFiles)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			result.FilesSkipped++
			return nil
		}
		if int64(len(data)) > MaxTrackedTextBytes {
			result.FilesSkipped++
			return nil
		}
		if totalBytes+int64(len(data)) > MaxProjectScanBytes {
			return fmt.Errorf("project scan exceeded total byte limit (%d bytes)", MaxProjectScanBytes)
		}
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			result.FilesSkipped++
			return nil
		}
		relative = filepath.ToSlash(relative)
		result.FilesDiscovered++
		previous, lookupErr := s.store.GetTrackedFileByPath(ctx, projectID, relative, root.RootPath)
		if lookupErr != nil && !errors.Is(lookupErr, store.ErrNotFound) {
			return lookupErr
		}
		if errors.Is(lookupErr, store.ErrNotFound) {
			result.FilesAdded++
		}
		fileType := strings.TrimPrefix(strings.ToLower(filepath.Ext(relative)), ".")
		if fileType == "" {
			fileType = "text"
		}
		_, version, err := s.Capture(ctx, CaptureInput{ProjectID: projectID, RelativePath: relative, FileType: fileType, TrackingPolicy: "project_scan", ChangeSummary: "Project directory scan"})
		if err != nil {
			return fmt.Errorf("capture %s: %w", relative, err)
		}
		if lookupErr != nil || previous.CurrentVersionID != version.VersionID {
			result.VersionsAdded++
		}
		totalBytes += int64(len(data))
		return nil
	})
	return result, err
}

func hashContent(data []byte) string {
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func (s *Service) Capture(ctx context.Context, input CaptureInput) (model.TrackedFile, model.FileVersion, error) {
	sourceRoot := ""
	if projectRoot, rootErr := s.store.GetProjectFileRoot(ctx, input.ProjectID); rootErr == nil {
		sourceRoot = projectRoot.RootPath
	} else if !errors.Is(rootErr, store.ErrNotFound) {
		return model.TrackedFile{}, model.FileVersion{}, rootErr
	}
	file, lookupErr := s.store.GetTrackedFileByPath(ctx, input.ProjectID, filepath.ToSlash(input.RelativePath), sourceRoot)
	captureKind := input.ChangeKind
	if lookupErr == nil {
		sourceRoot = file.SourceRoot
	} else if errors.Is(lookupErr, store.ErrNotFound) {
		if captureKind == "" {
			captureKind = model.FileChangeFileAdded
		}
	} else {
		return model.TrackedFile{}, model.FileVersion{}, lookupErr
	}
	absolute, relative, err := s.resolveTrackedPath(input.RelativePath, sourceRoot)
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
	contentHash := hashContent(data)
	file, err = s.store.TrackFile(ctx, model.TrackFileInput{ProjectID: input.ProjectID, CurrentPath: relative, FileType: input.FileType, TrackingPolicy: input.TrackingPolicy, SourceRoot: sourceRoot})
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
	if len(versions) > 0 && versions[len(versions)-1].ContentHash == contentHash && input.ChangeKind == "" && input.RestoresVersionID == "" {
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
	version, err := s.store.CreateFileVersion(ctx, model.CreateFileVersionInput{FileID: file.FileID, ContentHash: contentHash, ParentVersionID: parent, RestoresVersionID: input.RestoresVersionID, ChangeKind: captureKind, SnapshotType: snapshotType, BlobRef: blobRef, SizeBytes: int64(len(data)), AgentID: input.AgentID, SessionID: input.SessionID, Model: input.Model, ShadowboxRunID: input.ShadowboxRunID, TaskID: input.TaskID, SubtaskID: input.SubtaskID, SprintID: input.SprintID, CheckpointID: input.CheckpointID, ChangeSummary: input.ChangeSummary})
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	file.CurrentVersionID = version.VersionID
	return file, version, nil
}

func (s *Service) SaveContent(ctx context.Context, fileID, content, expectedCurrentVersionID, agentID, sessionID, modelName string, changeKind model.FileChangeKind) (model.TrackedFile, model.FileVersion, error) {
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
	absolute, _, err := s.resolveTrackedPath(file.CurrentPath, file.SourceRoot)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	original, err := os.ReadFile(absolute)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("read current file before save: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("stat current file before save: %w", err)
	}
	mode := info.Mode().Perm()
	if file.CurrentVersionID != "" {
		_, currentVersion, readErr := s.ReadVersion(ctx, fileID, file.CurrentVersionID)
		if readErr != nil {
			return model.TrackedFile{}, model.FileVersion{}, readErr
		}
		if hashContent(original) != currentVersion.ContentHash {
			if _, _, captureErr := s.Capture(ctx, CaptureInput{ProjectID: file.ProjectID, RelativePath: file.CurrentPath, FileType: file.FileType, TrackingPolicy: file.TrackingPolicy, ChangeSummary: "External change detected before editor save"}); captureErr != nil {
				return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("record external file change: %w", captureErr)
			}
			return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("%w: file changed outside the editor; an external snapshot was added to history, reload it before saving", store.ErrFileVersionConflict)
		}
	}
	if err := writeAtomic(absolute, []byte(content), mode); err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	_, version, err := s.Capture(ctx, CaptureInput{ProjectID: file.ProjectID, RelativePath: file.CurrentPath, FileType: file.FileType, TrackingPolicy: file.TrackingPolicy, AgentID: agentID, SessionID: sessionID, Model: modelName, ChangeSummary: "Editor save", ChangeKind: changeKind})
	if err != nil {
		_ = writeAtomic(absolute, original, mode)
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
	absolute, _, err := s.resolveTrackedPath(file.CurrentPath, file.SourceRoot)
	if err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	original, readErr := os.ReadFile(absolute)
	missing := errors.Is(readErr, os.ErrNotExist)
	if readErr != nil && !missing {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("read current file before restore: %w", readErr)
	}
	mode := os.FileMode(0o600)
	if !missing {
		info, statErr := os.Stat(absolute)
		if statErr != nil {
			return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("stat current file before restore: %w", statErr)
		}
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return model.TrackedFile{}, model.FileVersion{}, fmt.Errorf("create restore parent directory: %w", err)
	}
	if err := writeAtomic(absolute, content, mode); err != nil {
		return model.TrackedFile{}, model.FileVersion{}, err
	}
	_, restored, err := s.Capture(ctx, CaptureInput{ProjectID: file.ProjectID, RelativePath: file.CurrentPath, FileType: file.FileType, TrackingPolicy: file.TrackingPolicy, AgentID: input.AgentID, SessionID: input.SessionID, Model: input.Model, TaskID: input.TaskID, SubtaskID: input.SubtaskID, SprintID: input.SprintID, CheckpointID: input.CheckpointID, ChangeSummary: "Restored " + target.VersionID, ChangeKind: model.FileChangeReversion, RestoresVersionID: target.VersionID})
	if err != nil {
		if missing {
			_ = os.Remove(absolute)
		} else {
			_ = writeAtomic(absolute, original, mode)
		}
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
		blobPath, resolveErr := s.resolveBlobPath(version.BlobRef)
		if resolveErr != nil {
			return nil, model.FileVersion{}, fmt.Errorf("resolve version blob: %w", resolveErr)
		}
		data, readErr := os.ReadFile(blobPath)
		if readErr != nil {
			return nil, model.FileVersion{}, fmt.Errorf("read version blob: %w", readErr)
		}
		return data, version, nil
	}
	return nil, model.FileVersion{}, fmt.Errorf("%w: version %s", store.ErrNotFound, versionID)
}

func (s *Service) resolveBlobPath(blobRef string) (string, error) {
	relative := filepath.Clean(filepath.FromSlash(strings.TrimSpace(blobRef)))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("version blob path must remain under the history root")
	}
	root, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return "", fmt.Errorf("resolve history root: %w", err)
	}
	absolute := filepath.Join(s.root, relative)
	checkPath := absolute
	for {
		resolved, evalErr := filepath.EvalSymlinks(checkPath)
		if evalErr == nil {
			rel, relErr := filepath.Rel(filepath.Clean(root), filepath.Clean(resolved))
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
				return "", errors.New("version blob path resolves outside the history root")
			}
			return absolute, nil
		}
		if !errors.Is(evalErr, os.ErrNotExist) {
			return "", fmt.Errorf("resolve version blob path: %w", evalErr)
		}
		parent := filepath.Dir(checkPath)
		if parent == checkPath {
			return "", errors.New("version blob path has no existing history root ancestor")
		}
		checkPath = parent
	}
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

func (s *Service) resolveTrackedPath(relative, sourceRoot string) (string, string, error) {
	relative = filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", "", errors.New("tracked path must be relative to its authorized root")
	}
	base := s.root
	if strings.TrimSpace(sourceRoot) != "" {
		base = filepath.Clean(sourceRoot)
		if !filepath.IsAbs(base) {
			return "", "", errors.New("tracked source root must be absolute")
		}
	}
	absolute := filepath.Join(base, relative)

	evalRoot, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", "", fmt.Errorf("resolve tracked source root: %w", err)
	}
	checkPath := absolute
	for {
		resolved, err := filepath.EvalSymlinks(checkPath)
		if err == nil {
			cleanRoot := filepath.Clean(evalRoot)
			cleanResolved := filepath.Clean(resolved)
			rel, err := filepath.Rel(cleanRoot, cleanResolved)
			if err != nil || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
				return "", "", errors.New("symlink security violation: tracked path resolves outside its authorized root")
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("resolve tracked path: %w", err)
		}
		parent := filepath.Dir(checkPath)
		if parent == checkPath {
			return "", "", errors.New("tracked path has no existing root ancestor")
		}
		checkPath = parent
	}
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
