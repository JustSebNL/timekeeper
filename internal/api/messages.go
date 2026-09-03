// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func listProjectMessages(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		var kind model.MessageKind
		if k := r.URL.Query().Get("kind"); k != "" {
			kind = model.MessageKind(k)
			if !model.IsValidMessageKind(kind) {
				writeError(w, http.StatusBadRequest, "invalid_message_kind", "kind must be one of: note, decision, observation, link, lesson, question, answer.")
				return
			}
		}
		limit := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			n, err := strconv.Atoi(l)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a non-negative integer.")
				return
			}
			limit = n
		}
		messages, err := database.ListProjectMessages(r.Context(), projectID, kind, limit)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_project_messages_failed", "Could not list project messages.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": messages})
	}
}

func createProjectMessage(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		var input model.CreateProjectMessageInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_project_message", "Project message input must be valid JSON.")
			return
		}
		// Default author to the agent that made the request, mirroring notes.
		if input.Author == "" {
			input.Author = r.Header.Get("X-Agent-ID")
		}
		msg, err := database.CreateProjectMessage(r.Context(), projectID, input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_project_message", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, msg)
	}
}

func getProjectMessage(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		id, err := strconv.ParseInt(r.PathValue("messageID"), 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "invalid_message_id", "message id must be a positive integer.")
			return
		}
		msg, err := database.GetProjectMessage(r.Context(), projectID, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "message_not_found", "Message was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "get_project_message_failed", "Could not fetch project message.")
			return
		}
		writeJSON(w, http.StatusOK, msg)
	}
}

func searchProjectMessages(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		q := r.URL.Query().Get("q")
		limit := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			n, err := strconv.Atoi(l)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a non-negative integer.")
				return
			}
			limit = n
		}
		hits, err := database.SearchProjectMessages(r.Context(), projectID, q, limit)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "search_project_messages_failed", "Could not search project messages.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": hits, "query": q})
	}
}
