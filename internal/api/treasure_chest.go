// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

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

func treasureGemStoreError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "treasure_gem_not_found", "Treasure Chest gem or Project was not found.")
	case errors.Is(err, store.ErrValidation):
		writeError(w, http.StatusBadRequest, "invalid_treasure_gem", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "treasure_chest_"+operation+"_failed", "Could not "+operation+" Treasure Chest data.")
	}
}

func decodeGemJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil { return fmt.Errorf("request body must contain one JSON value") }
		return err
	}
	return nil
}

func createProjectGem(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateProjectGemInput
		if err := decodeGemJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_treasure_gem", "Gem input must be one valid JSON object.")
			return
		}
		if input.CreatedBy == "" { input.CreatedBy = r.Header.Get("X-Agent-ID") }
		gem, err := database.CreateProjectGem(r.Context(), r.PathValue("projectID"), input)
		if err != nil { treasureGemStoreError(w, "create", err); return }
		writeJSON(w, http.StatusCreated, gem)
	}
}

func parseProjectGemFilter(r *http.Request) (model.ProjectGemFilter, error) {
	query := r.URL.Query()
	filter := model.ProjectGemFilter{Query: query.Get("q"), Category: query.Get("category"), Subcategory: query.Get("subcategory"), TaskID: query.Get("task_id"), SprintID: query.Get("sprint_id")}
	if raw := query.Get("parent_gem_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 { return filter, fmt.Errorf("parent_gem_id must be a positive integer") }
		filter.ParentGemID = &id
	}
	if raw := query.Get("pinned"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil { return filter, fmt.Errorf("pinned must be a boolean") }
		filter.PinnedOnly = value
	}
	if raw := query.Get("include_deleted"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil { return filter, fmt.Errorf("include_deleted must be a boolean") }
		filter.IncludeDeleted = value
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 0 || limit > model.MaxProjectGemListLimit { return filter, fmt.Errorf("limit must be between 0 and %d", model.MaxProjectGemListLimit) }
		filter.Limit = limit
	}
	return filter, nil
}

func listProjectGems(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseProjectGemFilter(r)
		if err != nil { writeError(w, http.StatusBadRequest, "invalid_treasure_gem_filter", err.Error()); return }
		result, err := database.ListProjectGems(r.Context(), r.PathValue("projectID"), filter)
		if err != nil { treasureGemStoreError(w, "list", err); return }
		writeJSON(w, http.StatusOK, result)
	}
}

func searchProjectGems(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseProjectGemFilter(r)
		if err != nil { writeError(w, http.StatusBadRequest, "invalid_treasure_gem_filter", err.Error()); return }
		if filter.Query == "" { writeError(w, http.StatusBadRequest, "missing_treasure_gem_query", "q is required."); return }
		result, err := database.ListProjectGems(r.Context(), r.PathValue("projectID"), filter)
		if err != nil { treasureGemStoreError(w, "search", err); return }
		writeJSON(w, http.StatusOK, result)
	}
}

func listProjectGemCategories(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := database.ListProjectGemCategories(r.Context(), r.PathValue("projectID"))
		if err != nil { treasureGemStoreError(w, "list_categories", err); return }
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func parseProjectGemID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("gemID"), 10, 64)
	if err != nil || id < 1 { return 0, fmt.Errorf("gem_id must be a positive integer") }
	return id, nil
}

func getProjectGem(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseProjectGemID(r)
		if err != nil { writeError(w, http.StatusBadRequest, "invalid_treasure_gem_id", err.Error()); return }
		includeDeleted, err := strconv.ParseBool(defaultQuery(r.URL.Query().Get("include_deleted"), "false"))
		if err != nil { writeError(w, http.StatusBadRequest, "invalid_treasure_gem_filter", "include_deleted must be a boolean."); return }
		gem, err := database.GetProjectGem(r.Context(), r.PathValue("projectID"), id, includeDeleted)
		if err != nil { treasureGemStoreError(w, "get", err); return }
		writeJSON(w, http.StatusOK, gem)
	}
}

func updateProjectGem(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseProjectGemID(r)
		if err != nil { writeError(w, http.StatusBadRequest, "invalid_treasure_gem_id", err.Error()); return }
		var input model.UpdateProjectGemInput
		if err := decodeGemJSON(w, r, &input); err != nil { writeError(w, http.StatusBadRequest, "invalid_treasure_gem", "Gem update must be one valid JSON object."); return }
		if input.EditedBy == "" { input.EditedBy = r.Header.Get("X-Agent-ID") }
		gem, err := database.UpdateProjectGem(r.Context(), r.PathValue("projectID"), id, input)
		if err != nil { treasureGemStoreError(w, "update", err); return }
		writeJSON(w, http.StatusOK, gem)
	}
}

func setProjectGemPinned(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseProjectGemID(r)
		if err != nil { writeError(w, http.StatusBadRequest, "invalid_treasure_gem_id", err.Error()); return }
		var input struct { Pinned *bool `json:"pinned"` }
		if err := decodeGemJSON(w, r, &input); err != nil || input.Pinned == nil {
			writeError(w, http.StatusBadRequest, "invalid_treasure_gem_pin", "pinned must be an explicit boolean.")
			return
		}
		gem, err := database.SetProjectGemPinned(r.Context(), r.PathValue("projectID"), id, *input.Pinned)
		if err != nil { treasureGemStoreError(w, "pin", err); return }
		writeJSON(w, http.StatusOK, gem)
	}
}

func deleteProjectGem(database *store.Store, restore bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseProjectGemID(r)
		if err != nil { writeError(w, http.StatusBadRequest, "invalid_treasure_gem_id", err.Error()); return }
		var gem model.ProjectGem
		if restore { gem, err = database.RestoreProjectGem(r.Context(), r.PathValue("projectID"), id) } else { gem, err = database.DeleteProjectGem(r.Context(), r.PathValue("projectID"), id) }
		if err != nil { treasureGemStoreError(w, "delete", err); return }
		writeJSON(w, http.StatusOK, gem)
	}
}

func listProjectGemRevisions(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseProjectGemID(r)
		if err != nil { writeError(w, http.StatusBadRequest, "invalid_treasure_gem_id", err.Error()); return }
		items, err := database.ListProjectGemRevisions(r.Context(), r.PathValue("projectID"), id)
		if err != nil { treasureGemStoreError(w, "list_revisions", err); return }
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func defaultQuery(value, fallback string) string {
	if value == "" { return fallback }
	return value
}
