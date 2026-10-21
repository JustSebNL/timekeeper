package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func globalSearch(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		limit := 0
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > model.MaxGlobalSearchLimit {
				writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 200.")
				return
			}
			limit = parsed
		}
		after, err := parseGlobalSearchDate(r.URL.Query().Get("after"), false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_search_date", "after must be an ISO date or RFC3339 timestamp.")
			return
		}
		before, err := parseGlobalSearchDate(r.URL.Query().Get("before"), true)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_search_date", "before must be an ISO date or RFC3339 timestamp.")
			return
		}
		tagFilters := make([]string, 0)
		for _, raw := range r.URL.Query()["tags"] {
			for _, tag := range strings.Split(raw, ",") {
				if strings.TrimSpace(tag) != "" {
					tagFilters = append(tagFilters, strings.TrimSpace(tag))
				}
			}
		}
		response, err := database.SearchGlobal(r.Context(), model.GlobalSearchInput{
			Query: query, ProjectID: strings.TrimSpace(r.URL.Query().Get("project")),
			Scopes: r.URL.Query()["scope"], Limit: limit, Kind: r.URL.Query().Get("kind"),
			Tags: tagFilters, After: after, Before: before,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "The requested Project was not found.")
				return
			}
			if errors.Is(err, store.ErrValidation) {
				writeError(w, http.StatusBadRequest, "invalid_search", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "global_search_failed", "Could not search Projects, Tasks, and messages.")
			return
		}
		writeJSON(w, http.StatusOK, response)
	}
}
