// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/history"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

type FileHistoryService interface {
	GetTrackedFile(context.Context, string) (model.TrackedFile, error)
	ListTrackedFiles(context.Context, string) ([]model.TrackedFile, error)
	ListFileVersions(context.Context, string) ([]model.FileVersion, error)
	Capture(context.Context, history.CaptureInput) (model.TrackedFile, model.FileVersion, error)
	ReadVersion(context.Context, string, string) ([]byte, model.FileVersion, error)
	SaveContent(context.Context, string, string, string, string, string, string) (model.TrackedFile, model.FileVersion, error)
	Restore(context.Context, history.RestoreInput) (model.TrackedFile, model.FileVersion, error)
}

func fileHistoryRoutes(mux *http.ServeMux, service FileHistoryService) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/files", projectFiles(service))
	mux.HandleFunc("GET /api/v1/files/{fileID}", fileInfo(service))
	mux.HandleFunc("GET /api/v1/files/{fileID}/versions", fileVersions(service))
	mux.HandleFunc("GET /api/v1/files/{fileID}/versions/{versionID}", fileVersion(service))
	mux.HandleFunc("POST /api/v1/files/{fileID}/versions", saveFileVersion(service))
	mux.HandleFunc("POST /api/v1/files/{fileID}/snapshots", captureFileSnapshot(service))
	mux.HandleFunc("GET /api/v1/files/{fileID}/compare", fileCompare(service))
	mux.HandleFunc("POST /api/v1/files/{fileID}/restore", restoreFile(service))
}

func projectFiles(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		files, err := service.ListTrackedFiles(r.Context(), r.PathValue("projectID"))
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": files})
	}
}

func fileInfo(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		file, err := service.GetTrackedFile(r.Context(), r.PathValue("fileID"))
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, file)
	}
}

func fileVersions(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		versions, err := service.ListFileVersions(r.Context(), r.PathValue("fileID"))
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": versions})
	}
}

func fileVersion(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		content, version, err := service.ReadVersion(r.Context(), r.PathValue("fileID"), r.PathValue("versionID"))
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"version": version, "content": string(content)})
	}
}

func captureFileSnapshot(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		var body struct {
			Phase         string `json:"phase"`
			AgentID       string `json:"agent_id"`
			SessionID     string `json:"session_id"`
			Model         string `json:"model"`
			TaskID        string `json:"task_id"`
			SubtaskID     string `json:"subtask_id"`
			SprintID      string `json:"sprint_id"`
			CheckpointID  string `json:"checkpoint_id"`
			ChangeSummary string `json:"change_summary"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_snapshot_request", "Snapshot input must be one JSON object.")
			return
		}
		if body.Phase != "code_start" && body.Phase != "code_end" {
			writeError(w, http.StatusBadRequest, "invalid_snapshot_phase", "phase must be code_start or code_end.")
			return
		}
		file, err := service.GetTrackedFile(r.Context(), r.PathValue("fileID"))
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		summary := strings.TrimSpace(body.ChangeSummary)
		if summary == "" {
			summary = body.Phase
		} else {
			summary = body.Phase + ": " + summary
		}
		tracked, version, err := service.Capture(r.Context(), history.CaptureInput{ProjectID: file.ProjectID, RelativePath: file.CurrentPath, FileType: file.FileType, TrackingPolicy: file.TrackingPolicy, AgentID: body.AgentID, SessionID: body.SessionID, Model: body.Model, TaskID: body.TaskID, SubtaskID: body.SubtaskID, SprintID: body.SprintID, CheckpointID: body.CheckpointID, ChangeSummary: summary, SnapshotType: "full"})
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"file": tracked, "version": version})
	}
}

func saveFileVersion(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		var body struct {
			Content                  string `json:"content"`
			ExpectedCurrentVersionID string `json:"expected_current_version_id"`
			AgentID                  string `json:"agent_id"`
			SessionID                string `json:"session_id"`
			Model                    string `json:"model"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 11*1024*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_file_save", "File save input must be one JSON object.")
			return
		}
		file, version, err := service.SaveContent(r.Context(), r.PathValue("fileID"), body.Content, body.ExpectedCurrentVersionID, body.AgentID, body.SessionID, body.Model)
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"file": file, "version": version})
	}
}

func fileCompare(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		fromID := strings.TrimSpace(r.URL.Query().Get("from"))
		toID := strings.TrimSpace(r.URL.Query().Get("to"))
		if fromID == "" || toID == "" {
			writeError(w, http.StatusBadRequest, "compare_versions_required", "from and to version IDs are required.")
			return
		}
		fromContent, from, err := service.ReadVersion(r.Context(), r.PathValue("fileID"), fromID)
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		toContent, to, err := service.ReadVersion(r.Context(), r.PathValue("fileID"), toID)
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		added, removed := lineDelta(string(fromContent), string(toContent))
		writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "from_content": string(fromContent), "to_content": string(toContent), "added_lines": added, "removed_lines": removed})
	}
}

func restoreFile(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		var body struct {
			VersionID                string `json:"version_id"`
			ExpectedCurrentVersionID string `json:"expected_current_version_id"`
			AgentID                  string `json:"agent_id"`
			SessionID                string `json:"session_id"`
			Model                    string `json:"model"`
			TaskID                   string `json:"task_id"`
			SubtaskID                string `json:"subtask_id"`
			SprintID                 string `json:"sprint_id"`
			CheckpointID             string `json:"checkpoint_id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF || strings.TrimSpace(body.VersionID) == "" {
			writeError(w, http.StatusBadRequest, "invalid_restore_request", "version_id is required and the body must contain one JSON object.")
			return
		}
		file, version, err := service.Restore(r.Context(), history.RestoreInput{FileID: r.PathValue("fileID"), VersionID: body.VersionID, ExpectedCurrentVersionID: body.ExpectedCurrentVersionID, AgentID: body.AgentID, SessionID: body.SessionID, Model: body.Model, TaskID: body.TaskID, SubtaskID: body.SubtaskID, SprintID: body.SprintID, CheckpointID: body.CheckpointID})
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"file": file, "version": version})
	}
}

func lineDelta(from, to string) (int, int) {
	fromLines := strings.Split(from, "\n")
	toLines := strings.Split(to, "\n")
	common := make(map[string]bool, len(fromLines))
	for _, line := range fromLines {
		common[line] = true
	}
	added, removed := 0, 0
	for _, line := range toLines {
		if !common[line] {
			added++
		}
	}
	common = make(map[string]bool, len(toLines))
	for _, line := range toLines {
		common[line] = true
	}
	for _, line := range fromLines {
		if !common[line] {
			removed++
		}
	}
	return added, removed
}

func writeFileHistoryError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrFileVersionConflict) {
		writeError(w, http.StatusConflict, "file_version_conflict", err.Error())
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "file_not_found", "Tracked file or version was not found.")
		return
	}
	if errors.Is(err, store.ErrValidation) {
		writeError(w, http.StatusBadRequest, "file_validation_error", err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "file_history_failed", "File history operation failed.")
}
