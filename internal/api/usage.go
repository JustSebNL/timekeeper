// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func usageSnapshotsHandler(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			recordUsageSnapshot(database).ServeHTTP(w, r)
		case http.MethodGet:
			listUsageSnapshots(database).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		}
	}
}

func recordUsageSnapshot(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.UsageSnapshotInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_usage_snapshot", "Usage snapshot input must be valid JSON.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_usage_snapshot", "Usage snapshot input must contain one JSON object.")
			return
		}
		input.SessionID = r.PathValue("sessionID")
		result, err := database.RecordUsageSnapshot(r.Context(), r.PathValue("projectID"), input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_usage_snapshot", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func listUsageSnapshots(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		sessionID := r.PathValue("sessionID")
		limit := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			n, err := strconv.Atoi(l)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a non-negative integer.")
				return
			}
			limit = n
		}
		snapshots, err := database.ListUsageSnapshots(r.Context(), projectID, sessionID, limit)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "session_not_found", "Usage session was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_usage_snapshots_failed", "Could not list usage snapshots.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": snapshots})
	}
}

func projectUsageSummary(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := database.ProjectUsageSummary(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "usage_summary_failed", fmt.Sprintf("Could not load usage summary: %v", err))
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
}