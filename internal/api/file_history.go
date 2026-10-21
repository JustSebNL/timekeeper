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
	GetProjectRoot(context.Context, string) (model.ProjectFileRoot, error)
	SetProjectRoot(context.Context, string, string) (model.ProjectFileRoot, error)
	ScanProjectFiles(context.Context, string) (history.ProjectScanResult, error)
	ListFileVersions(context.Context, string) ([]model.FileVersion, error)
	Capture(context.Context, history.CaptureInput) (model.TrackedFile, model.FileVersion, error)
	ReadVersion(context.Context, string, string) ([]byte, model.FileVersion, error)
	SaveContent(context.Context, string, string, string, string, string, string, model.FileChangeKind) (model.TrackedFile, model.FileVersion, error)
	Restore(context.Context, history.RestoreInput) (model.TrackedFile, model.FileVersion, error)
}

func fileHistoryRoutes(mux *http.ServeMux, service FileHistoryService) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/files", projectFiles(service))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/file-root", projectFileRoot(service))
	mux.HandleFunc("PUT /api/v1/projects/{projectID}/file-root", setProjectFileRoot(service))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/files/scan", scanProjectFiles(service))
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
		if files == nil {
			files = []model.TrackedFile{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": files})
	}
}

func projectFileRoot(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		projectID := r.PathValue("projectID")
		root, err := service.GetProjectRoot(r.Context(), projectID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, model.ProjectFileRoot{ProjectID: projectID})
			return
		}
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, root)
	}
}

func setProjectFileRoot(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		var body struct {
			RootPath string `json:"root_path"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_project_file_root", "Source root input must be one JSON object.")
			return
		}
		root, err := service.SetProjectRoot(r.Context(), r.PathValue("projectID"), body.RootPath)
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, root)
	}
}

func scanProjectFiles(service FileHistoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "history_unavailable", "File history is not configured.")
			return
		}
		var body struct{}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_project_scan", "Project scan input must be one JSON object.")
			return
		}
		result, err := service.ScanProjectFiles(r.Context(), r.PathValue("projectID"))
		if err != nil {
			writeFileHistoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
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
			Phase         string               `json:"phase"`
			AgentID       string               `json:"agent_id"`
			SessionID     string               `json:"session_id"`
			Model         string               `json:"model"`
			TaskID        string               `json:"task_id"`
			SubtaskID     string               `json:"subtask_id"`
			SprintID      string               `json:"sprint_id"`
			CheckpointID  string               `json:"checkpoint_id"`
			ChangeSummary string               `json:"change_summary"`
			ChangeKind    model.FileChangeKind `json:"change_kind"`
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
		tracked, version, err := service.Capture(r.Context(), history.CaptureInput{ProjectID: file.ProjectID, RelativePath: file.CurrentPath, FileType: file.FileType, TrackingPolicy: file.TrackingPolicy, AgentID: body.AgentID, SessionID: body.SessionID, Model: body.Model, TaskID: body.TaskID, SubtaskID: body.SubtaskID, SprintID: body.SprintID, CheckpointID: body.CheckpointID, ChangeSummary: summary, ChangeKind: body.ChangeKind, SnapshotType: "full"})
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
			Content                  string               `json:"content"`
			ExpectedCurrentVersionID string               `json:"expected_current_version_id"`
			AgentID                  string               `json:"agent_id"`
			SessionID                string               `json:"session_id"`
			Model                    string               `json:"model"`
			ChangeKind               model.FileChangeKind `json:"change_kind"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 11*1024*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_file_save", "File save input must be one JSON object.")
			return
		}
		file, version, err := service.SaveContent(r.Context(), r.PathValue("fileID"), body.Content, body.ExpectedCurrentVersionID, body.AgentID, body.SessionID, body.Model, body.ChangeKind)
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
		hunks := lineDelta(string(fromContent), string(toContent))
		writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "from_content": string(fromContent), "to_content": string(toContent), "hunks": hunks})
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

type LineHunk struct {
	FromStart int      `json:"from_start"`
	FromCount int      `json:"from_count"`
	ToStart   int      `json:"to_start"`
	ToCount   int      `json:"to_count"`
	Removed   []string `json:"removed"`
	Added     []string `json:"added"`
}

func lineDelta(from, to string) []LineHunk {
	fromLines := strings.Split(from, "\n")
	toLines := strings.Split(to, "\n")
	n, m := len(fromLines), len(toLines)
	const maxDiffCells = int64(2_000_000)
	if int64(n+1)*int64(m+1) > maxDiffCells {
		return buildHunksFromCoarse(fromLines, toLines)
	}
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if fromLines[i] == toLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var hunks []LineHunk
	for i, j := 0, 0; i < n || j < m; {
		if i < n && j < m && fromLines[i] == toLines[j] {
			i++
			j++
			continue
		}
		fromStart, toStart := i+1, j+1
		var removed, added []string
		for (i < n || j < m) && !(i < n && j < m && fromLines[i] == toLines[j]) {
			if i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]) {
				removed = append(removed, fromLines[i])
				i++
			} else if j < m {
				added = append(added, toLines[j])
				j++
			}
		}
		hunks = append(hunks, LineHunk{FromStart: fromStart, FromCount: len(removed), ToStart: toStart, ToCount: len(added), Removed: removed, Added: added})
	}
	return hunks
}

func buildHunksFromTrace(trace [][]int, offset int, fromLines, toLines []string) []LineHunk {
	n, m := len(fromLines), len(toLines)
	var edits [][3]int

	x, y := n, m
	for distance := len(trace) - 1; distance > 0; distance-- {
		frontier := trace[distance]
		diagonal := x - y
		index := offset + diagonal
		previousDiagonal := diagonal - 1
		if diagonal == -distance || (diagonal != distance && frontier[index-1] < frontier[index+1]) {
			previousDiagonal = diagonal + 1
		}
		previousX := frontier[offset+previousDiagonal]
		previousY := previousX - previousDiagonal
		for x > previousX && y > previousY {
			edits = append(edits, [3]int{x - 1, y - 1, 0})
			x--
			y--
		}
		if x == previousX {
			edits = append(edits, [3]int{x, y - 1, 1})
			y--
		} else {
			edits = append(edits, [3]int{x - 1, y, -1})
			x--
		}
	}
	for x > 0 && y > 0 {
		edits = append(edits, [3]int{x - 1, y - 1, 0})
		x--
		y--
	}

	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}

	var hunks []LineHunk
	var currentHunk *LineHunk
	for _, e := range edits {
		fx, ty, kind := e[0], e[1], e[2]
		switch kind {
		case -1:
			if currentHunk == nil || currentHunk.FromStart+currentHunk.FromCount != fx {
				if currentHunk != nil {
					hunks = append(hunks, *currentHunk)
				}
				currentHunk = &LineHunk{
					FromStart: fx,
					FromCount: 1,
					ToStart:   ty,
					ToCount:   0,
					Removed:   []string{fromLines[fx]},
					Added:     []string{},
				}
			} else {
				currentHunk.FromCount++
				currentHunk.Removed = append(currentHunk.Removed, fromLines[fx])
			}
		case 1:
			if currentHunk == nil || currentHunk.ToStart+currentHunk.ToCount != ty {
				if currentHunk != nil {
					hunks = append(hunks, *currentHunk)
				}
				currentHunk = &LineHunk{
					FromStart: fx,
					FromCount: 0,
					ToStart:   ty,
					ToCount:   1,
					Removed:   []string{},
					Added:     []string{toLines[ty]},
				}
			} else {
				currentHunk.ToCount++
				currentHunk.Added = append(currentHunk.Added, toLines[ty])
			}
		case 0:
			if currentHunk != nil {
				hunks = append(hunks, *currentHunk)
				currentHunk = nil
			}
		}
	}
	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}
	return hunks
}

func buildHunksFromCoarse(fromLines, toLines []string) []LineHunk {
	prefix := 0
	for prefix < len(fromLines) && prefix < len(toLines) && fromLines[prefix] == toLines[prefix] {
		prefix++
	}
	fromEnd, toEnd := len(fromLines), len(toLines)
	for fromEnd > prefix && toEnd > prefix && fromLines[fromEnd-1] == toLines[toEnd-1] {
		fromEnd--
		toEnd--
	}
	if fromEnd == prefix && toEnd == prefix {
		return nil
	}
	return []LineHunk{{
		FromStart: prefix + 1,
		FromCount: fromEnd - prefix,
		ToStart:   prefix + 1,
		ToCount:   toEnd - prefix,
		Removed:   fromLines[prefix:fromEnd],
		Added:     toLines[prefix:toEnd],
	}}
}

func countLineEdits(trace [][]int, offset, x, y int) (int, int) {
	added, removed := 0, 0
	for distance := len(trace) - 1; distance > 0; distance-- {
		frontier := trace[distance]
		diagonal := x - y
		index := offset + diagonal
		previousDiagonal := diagonal - 1
		if diagonal == -distance || (diagonal != distance && frontier[index-1] < frontier[index+1]) {
			previousDiagonal = diagonal + 1
		}
		previousX := frontier[offset+previousDiagonal]
		previousY := previousX - previousDiagonal
		for x > previousX && y > previousY {
			x--
			y--
		}
		if x == previousX {
			added++
			y--
		} else {
			removed++
			x--
		}
	}
	// The remaining prefix is the unchanged diagonal before the first edit.
	for x > 0 && y > 0 {
		x--
		y--
	}
	added += y
	removed += x
	return added, removed
}

// coarseLineDelta bounds worst-case work for huge unrelated files. The full
// contents remain available from the compare response; this fallback reports
// the unmatched middle after stripping an exact common prefix and suffix.
func coarseLineDelta(from, to []string) (int, int) {
	prefix := 0
	for prefix < len(from) && prefix < len(to) && from[prefix] == to[prefix] {
		prefix++
	}
	fromEnd, toEnd := len(from), len(to)
	for fromEnd > prefix && toEnd > prefix && from[fromEnd-1] == to[toEnd-1] {
		fromEnd--
		toEnd--
	}
	return toEnd - prefix, fromEnd - prefix
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
