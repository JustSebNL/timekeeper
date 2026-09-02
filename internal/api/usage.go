// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func recordUsageSnapshot(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.UsageSnapshotInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_usage_snapshot", "Usage snapshot must be valid JSON.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_usage_snapshot", "Usage snapshot must contain one JSON object.")
			return
		}
		input.SessionID = r.PathValue("sessionID")
		result, err := database.RecordUsageSnapshot(r.Context(), r.PathValue("projectID"), input)
		if err != nil {
			status := http.StatusBadRequest
			code := "invalid_usage_snapshot"
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
				code = "project_not_found"
			}
			writeError(w, status, code, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, result)
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
