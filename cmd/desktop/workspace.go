package main

import (
	"context"
	"time"

	"github.com/JustSebNL/timekeeper/internal/desktop"
	"github.com/JustSebNL/timekeeper/internal/knowledge"
	"github.com/JustSebNL/timekeeper/internal/model"
)

const historyRequestDeadline = 2 * time.Minute

// TestConnection performs a read-only compatibility probe for the configured
// local profile. It does not launch a child Core or mutate SQLite state.
func (a *DesktopApp) TestConnection() (desktop.ConnectionDiagnostic, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return desktop.ProbeConnection(ctx, a.cfg)
}

// AgentWorkStates returns Core's current agent activity projection through the
// same authoritative API used by the browser client.
func (a *DesktopApp) AgentWorkStates() ([]model.AgentWorkState, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.AgentWorkStates(ctx)
}

// ProjectHandoff exposes Core's bounded read-only Project snapshot to Svelte.
func (a *DesktopApp) ProjectHandoff(projectID string) (model.HandoffSnapshot, error) {
	client, err := a.client()
	if err != nil {
		return model.HandoffSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.ProjectHandoff(ctx, projectID)
}

// ProjectUsageSummary exposes Core's server-priced usage projection to Svelte.
func (a *DesktopApp) ProjectUsageSummary(projectID string) (model.ProjectUsageSummary, error) {
	client, err := a.client()
	if err != nil {
		return model.ProjectUsageSummary{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.ProjectUsageSummary(ctx, projectID)
}

// ProjectOperationalSummary exposes Core's durable Sprint metrics to Svelte.
func (a *DesktopApp) ProjectOperationalSummary(projectID string) (model.ProjectOperationalSummary, error) {
	client, err := a.client()
	if err != nil {
		return model.ProjectOperationalSummary{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.ProjectOperationalSummary(ctx, projectID)
}

// KnowledgeStatus exposes Core's provider-neutral Memory Galaxy status to Svelte.
func (a *DesktopApp) KnowledgeStatus() (knowledge.ProviderStatus, error) {
	client, err := a.client()
	if err != nil {
		return knowledge.ProviderStatus{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.KnowledgeStatus(ctx)
}

// SearchKnowledge delegates a bounded read-only cross-project query to Core.
func (a *DesktopApp) SearchKnowledge(query string, limit int) ([]knowledge.KnowledgeResult, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.SearchKnowledge(ctx, knowledge.KnowledgeQuery{Query: query, Limit: limit})
}

// GlobalSearch forwards the query to Core so one ranking and pagination contract
// serves the browser, desktop, CLI and agent clients.
func (a *DesktopApp) GlobalSearch(query string, limit int, scopes []string) (model.GlobalSearchResponse, error) {
	client, err := a.client()
	if err != nil {
		return model.GlobalSearchResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.GlobalSearch(ctx, query, limit, scopes)
}

// ProjectExecutionTree exposes the existing read-only Core hierarchy projection
// through the Wails bridge; it contains no domain or Store logic.
func (a *DesktopApp) ProjectExecutionTree(projectID string) (model.ExecutionTree, error) {
	client, err := a.client()
	if err != nil {
		return model.ExecutionTree{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.ExecutionTree(ctx, projectID)
}

// TaskDetail exposes Core's typed Task detail projection through Wails.
func (a *DesktopApp) TaskDetail(taskID string) (desktop.TaskDetail, error) {
	client, err := a.client()
	if err != nil {
		return desktop.TaskDetail{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.TaskDetail(ctx, taskID)
}

// UpdateTaskStatus delegates an explicit Task status to Core.
func (a *DesktopApp) UpdateTaskStatus(taskID, status string) (model.Task, error) {
	client, err := a.client()
	if err != nil {
		return model.Task{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.UpdateTaskStatus(ctx, taskID, status)
}

// TransitionSprint delegates a supported Sprint lifecycle action to Core.
func (a *DesktopApp) TransitionSprint(sprintID, action, reason string) (model.Sprint, error) {
	client, err := a.client()
	if err != nil {
		return model.Sprint{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.TransitionSprint(ctx, sprintID, action, reason)
}

// ReadFileVersion loads one saved file version through the authoritative Core.
func (a *DesktopApp) ReadFileVersion(fileID, versionID string) (desktop.FileVersionContent, error) {
	client, err := a.client()
	if err != nil {
		return desktop.FileVersionContent{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyRequestDeadline)
	defer cancel()
	return client.ReadFileVersion(ctx, fileID, versionID)
}

// ProjectFileRoot reads the source folder explicitly configured for a Project.
func (a *DesktopApp) ProjectFileRoot(projectID string) (model.ProjectFileRoot, error) {
	client, err := a.client()
	if err != nil {
		return model.ProjectFileRoot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyRequestDeadline)
	defer cancel()
	return client.ProjectFileRoot(ctx, projectID)
}

// ProjectFiles lists Core-tracked file metadata without scanning the local filesystem.
func (a *DesktopApp) ProjectFiles(projectID string) ([]model.TrackedFile, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyRequestDeadline)
	defer cancel()
	return client.ProjectFiles(ctx, projectID)
}

// FileVersions reads immutable version metadata for a tracked file.
func (a *DesktopApp) FileVersions(fileID string) ([]model.FileVersion, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyRequestDeadline)
	defer cancel()
	return client.FileVersions(ctx, fileID)
}

// SetProjectFileRoot delegates source-folder validation and persistence to Core.
func (a *DesktopApp) SetProjectFileRoot(projectID, rootPath string) (model.ProjectFileRoot, error) {
	client, err := a.client()
	if err != nil {
		return model.ProjectFileRoot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyRequestDeadline)
	defer cancel()
	return client.SetProjectFileRoot(ctx, projectID, rootPath)
}

// ScanProjectFiles runs the explicit, bounded source scan in Core.
func (a *DesktopApp) ScanProjectFiles(projectID string) (desktop.ProjectFileScanResult, error) {
	client, err := a.client()
	if err != nil {
		return desktop.ProjectFileScanResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyRequestDeadline)
	defer cancel()
	return client.ScanProjectFiles(ctx, projectID)
}

// CompareFileVersions delegates a version diff to Core.
func (a *DesktopApp) CompareFileVersions(fileID, fromVersionID, toVersionID string) (desktop.FileCompareResult, error) {
	client, err := a.client()
	if err != nil {
		return desktop.FileCompareResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyRequestDeadline)
	defer cancel()
	return client.CompareFileVersions(ctx, fileID, fromVersionID, toVersionID)
}

// SaveFileVersion creates a version with optimistic concurrency enforced by Core.
func (a *DesktopApp) SaveFileVersion(fileID string, input desktop.FileHistorySaveInput) (desktop.FileHistoryMutationResult, error) {
	client, err := a.client()
	if err != nil {
		return desktop.FileHistoryMutationResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyRequestDeadline)
	defer cancel()
	return client.SaveFileVersion(ctx, fileID, input)
}

// RestoreFileVersion asks Core to append a new reversion version.
func (a *DesktopApp) RestoreFileVersion(fileID string, input desktop.FileHistoryRestoreInput) (desktop.FileHistoryMutationResult, error) {
	client, err := a.client()
	if err != nil {
		return desktop.FileHistoryMutationResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyRequestDeadline)
	defer cancel()
	return client.RestoreFileVersion(ctx, fileID, input)
}

// PluginCatalog exposes Core's read-only plugin registry/catalog to Svelte.
func (a *DesktopApp) PluginCatalog() (desktop.PluginCatalog, error) {
	client, err := a.client()
	if err != nil {
		return desktop.PluginCatalog{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.PluginCatalog(ctx)
}
