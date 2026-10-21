package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const (
	fileHistoryResponseLimit  = 48 << 20
	fileHistoryRequestTimeout = 2 * time.Minute
)

type FileVersionContent struct {
	Version model.FileVersion `json:"version"`
	Content string            `json:"content"`
}

type fileListResponse struct {
	Items []model.TrackedFile `json:"items"`
}

type fileVersionListResponse struct {
	Items []model.FileVersion `json:"items"`
}

// ProjectFileRoot reads the explicit source folder configured for one Project.
func (c *CoreClient) ProjectFileRoot(ctx context.Context, projectID string) (model.ProjectFileRoot, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return model.ProjectFileRoot{}, fmt.Errorf("Project ID is required")
	}
	var result model.ProjectFileRoot
	route := "/api/v1/projects/" + url.PathEscape(projectID) + "/file-root"
	if err := c.get(ctx, route, &result); err != nil {
		return model.ProjectFileRoot{}, err
	}
	if result.ProjectID != "" && result.ProjectID != projectID {
		return model.ProjectFileRoot{}, fmt.Errorf("Core returned a source root for a different Project")
	}
	if result.ProjectID == "" {
		result.ProjectID = projectID
	}
	return result, nil
}

// ProjectFiles lists only records already tracked by Core; it never scans a local folder.
func (c *CoreClient) ProjectFiles(ctx context.Context, projectID string) ([]model.TrackedFile, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("Project ID is required")
	}
	var response fileListResponse
	route := "/api/v1/projects/" + url.PathEscape(projectID) + "/files"
	if err := c.get(ctx, route, &response); err != nil {
		return nil, err
	}
	if response.Items == nil {
		response.Items = []model.TrackedFile{}
	}
	for _, file := range response.Items {
		if file.ProjectID != "" && file.ProjectID != projectID {
			return nil, fmt.Errorf("Core returned a tracked file for a different Project")
		}
	}
	return response.Items, nil
}

// FileVersions lists the immutable version sequence for one tracked file.
func (c *CoreClient) FileVersions(ctx context.Context, fileID string) ([]model.FileVersion, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, fmt.Errorf("File ID is required")
	}
	var response fileVersionListResponse
	route := "/api/v1/files/" + url.PathEscape(fileID) + "/versions"
	if err := c.get(ctx, route, &response); err != nil {
		return nil, err
	}
	if response.Items == nil {
		response.Items = []model.FileVersion{}
	}
	for _, version := range response.Items {
		if version.FileID != "" && version.FileID != fileID {
			return nil, fmt.Errorf("Core returned a version for a different file")
		}
	}
	return response.Items, nil
}

// ReadFileVersion loads an immutable historical snapshot. The history endpoint
// accepts files up to 10 MiB, so its response cap and timeout are larger than
// the ordinary metadata requests.
func (c *CoreClient) ReadFileVersion(ctx context.Context, fileID, versionID string) (FileVersionContent, error) {
	fileID = strings.TrimSpace(fileID)
	versionID = strings.TrimSpace(versionID)
	if fileID == "" || versionID == "" {
		return FileVersionContent{}, fmt.Errorf("File ID and version ID are required")
	}
	var result FileVersionContent
	route := "/api/v1/files/" + url.PathEscape(fileID) + "/versions/" + url.PathEscape(versionID)
	if err := c.getFileHistory(ctx, route, &result); err != nil {
		return FileVersionContent{}, err
	}
	if result.Version.FileID != fileID || result.Version.VersionID != versionID {
		return FileVersionContent{}, fmt.Errorf("Core returned a different historical version")
	}
	return result, nil
}

func (c *CoreClient) getFileHistory(ctx context.Context, route string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+route, nil)
	if err != nil {
		return fmt.Errorf("create Core request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	client := *c.http
	client.Timeout = fileHistoryRequestTimeout
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCoreUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Core GET %s returned HTTP %d", route, resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, fileHistoryResponseLimit))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("Core GET %s returned malformed or oversized JSON", route)
	}
	return nil
}

func (c *CoreClient) postFileHistory(ctx context.Context, route string, input, target any) error {
	return c.sendFileHistory(ctx, http.MethodPost, route, input, target)
}

func (c *CoreClient) putFileHistory(ctx context.Context, route string, input, target any) error {
	return c.sendFileHistory(ctx, http.MethodPut, route, input, target)
}

func (c *CoreClient) sendFileHistory(ctx context.Context, method, route string, input, target any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode file-history request: %w", err)
	}
	if len(body) > 11*1024*1024 {
		return fmt.Errorf("file-history request exceeds the 11 MiB limit")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+route, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create file-history request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := *c.http
	client.Timeout = fileHistoryRequestTimeout
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCoreUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Core POST %s returned HTTP %d", route, resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, fileHistoryResponseLimit))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("Core POST %s returned malformed or oversized JSON", route)
	}
	return nil
}

func (c *CoreClient) SetProjectFileRoot(ctx context.Context, projectID, rootPath string) (model.ProjectFileRoot, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || strings.TrimSpace(rootPath) == "" {
		return model.ProjectFileRoot{}, fmt.Errorf("Project ID and source root are required")
	}
	var result model.ProjectFileRoot
	route := "/api/v1/projects/" + url.PathEscape(projectID) + "/file-root"
	if err := c.putFileHistory(ctx, route, map[string]string{"root_path": rootPath}, &result); err != nil {
		return model.ProjectFileRoot{}, err
	}
	if result.ProjectID != projectID || result.RootPath == "" {
		return model.ProjectFileRoot{}, fmt.Errorf("Core returned an incomplete Project source root")
	}
	return result, nil
}

type ProjectFileScanResult struct {
	ProjectID       string `json:"project_id"`
	RootPath        string `json:"root_path"`
	FilesDiscovered int    `json:"files_discovered"`
	FilesAdded      int    `json:"files_added"`
	VersionsAdded   int    `json:"versions_added"`
	FilesSkipped    int    `json:"files_skipped"`
}

func (c *CoreClient) ScanProjectFiles(ctx context.Context, projectID string) (ProjectFileScanResult, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectFileScanResult{}, fmt.Errorf("Project ID is required")
	}
	var result ProjectFileScanResult
	route := "/api/v1/projects/" + url.PathEscape(projectID) + "/files/scan"
	if err := c.postFileHistory(ctx, route, struct{}{}, &result); err != nil {
		return ProjectFileScanResult{}, err
	}
	if result.ProjectID != projectID {
		return ProjectFileScanResult{}, fmt.Errorf("Core returned a scan result for a different Project")
	}
	return result, nil
}

type FileHistorySaveInput struct {
	Content                  string               `json:"content"`
	ExpectedCurrentVersionID string               `json:"expected_current_version_id"`
	ChangeKind               model.FileChangeKind `json:"change_kind,omitempty"`
}

type FileHistoryRestoreInput struct {
	VersionID                string `json:"version_id"`
	ExpectedCurrentVersionID string `json:"expected_current_version_id"`
}

type FileHistoryMutationResult struct {
	File    model.TrackedFile `json:"file"`
	Version model.FileVersion `json:"version"`
}

func (c *CoreClient) SaveFileVersion(ctx context.Context, fileID string, input FileHistorySaveInput) (FileHistoryMutationResult, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" || strings.TrimSpace(input.ExpectedCurrentVersionID) == "" {
		return FileHistoryMutationResult{}, fmt.Errorf("File ID and expected current version ID are required")
	}
	if len(input.Content) > 10*1024*1024 {
		return FileHistoryMutationResult{}, fmt.Errorf("file content exceeds the 10 MiB history limit")
	}
	var result FileHistoryMutationResult
	route := "/api/v1/files/" + url.PathEscape(fileID) + "/versions"
	if err := c.postFileHistory(ctx, route, input, &result); err != nil {
		return FileHistoryMutationResult{}, err
	}
	if result.File.FileID != fileID || result.Version.FileID != fileID || result.File.CurrentVersionID != result.Version.VersionID {
		return FileHistoryMutationResult{}, fmt.Errorf("Core returned an inconsistent file version")
	}
	return result, nil
}

func (c *CoreClient) RestoreFileVersion(ctx context.Context, fileID string, input FileHistoryRestoreInput) (FileHistoryMutationResult, error) {
	fileID = strings.TrimSpace(fileID)
	input.VersionID = strings.TrimSpace(input.VersionID)
	input.ExpectedCurrentVersionID = strings.TrimSpace(input.ExpectedCurrentVersionID)
	if fileID == "" || input.VersionID == "" || input.ExpectedCurrentVersionID == "" {
		return FileHistoryMutationResult{}, fmt.Errorf("File ID, target version ID, and expected current version ID are required")
	}
	var result FileHistoryMutationResult
	route := "/api/v1/files/" + url.PathEscape(fileID) + "/restore"
	if err := c.postFileHistory(ctx, route, input, &result); err != nil {
		return FileHistoryMutationResult{}, err
	}
	if result.File.FileID != fileID || result.Version.FileID != fileID || result.File.CurrentVersionID != result.Version.VersionID || result.Version.RestoresVersionID != input.VersionID {
		return FileHistoryMutationResult{}, fmt.Errorf("Core returned an inconsistent restore result")
	}
	return result, nil
}

type FileCompareHunk struct {
	FromStart int      `json:"from_start"`
	FromCount int      `json:"from_count"`
	ToStart   int      `json:"to_start"`
	ToCount   int      `json:"to_count"`
	Removed   []string `json:"removed"`
	Added     []string `json:"added"`
}

type FileCompareResult struct {
	From        model.FileVersion `json:"from"`
	To          model.FileVersion `json:"to"`
	FromContent string            `json:"from_content"`
	ToContent   string            `json:"to_content"`
	Hunks       []FileCompareHunk `json:"hunks"`
}

func (c *CoreClient) CompareFileVersions(ctx context.Context, fileID, fromVersionID, toVersionID string) (FileCompareResult, error) {
	fileID = strings.TrimSpace(fileID)
	fromVersionID = strings.TrimSpace(fromVersionID)
	toVersionID = strings.TrimSpace(toVersionID)
	if fileID == "" || fromVersionID == "" || toVersionID == "" {
		return FileCompareResult{}, fmt.Errorf("File ID and both version IDs are required")
	}
	route := "/api/v1/files/" + url.PathEscape(fileID) + "/compare?from=" + url.QueryEscape(fromVersionID) + "&to=" + url.QueryEscape(toVersionID)
	var result FileCompareResult
	if err := c.getFileHistory(ctx, route, &result); err != nil {
		return FileCompareResult{}, err
	}
	if result.From.FileID != fileID || result.To.FileID != fileID || result.From.VersionID != fromVersionID || result.To.VersionID != toVersionID {
		return FileCompareResult{}, fmt.Errorf("Core returned a comparison for different file versions")
	}
	return result, nil
}
