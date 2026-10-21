// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func resolveMessageProject(database *store.Store, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := database.ResolveProjectID(r.Context(), r.PathValue("projectID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_project_id", err.Error())
			return
		}
		r.SetPathValue("projectID", projectID)
		next.ServeHTTP(w, r)
	}
}

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

func createProjectMessageCheckpoint(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.ProjectMessageCheckpointInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_project_message_checkpoint", "Checkpoint input must be valid JSON.")
			return
		}
		if input.Author == "" {
			input.Author = r.Header.Get("X-Agent-ID")
		}
		bundle, err := database.CreateProjectMessageCheckpoint(r.Context(), r.PathValue("projectID"), input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project or parent message was not found.")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_project_message_checkpoint", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, bundle)
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

func setProjectMessagePinned(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("messageID"), 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "invalid_message_id", "message id must be a positive integer.")
			return
		}
		var input struct {
			Pinned *bool `json:"pinned"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || input.Pinned == nil {
			writeError(w, http.StatusBadRequest, "invalid_message_pin", "pinned must be an explicit boolean.")
			return
		}
		message, err := database.SetProjectMessagePinned(r.Context(), r.PathValue("projectID"), id, *input.Pinned)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "message_not_found", "Message was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "set_project_message_pin_failed", "Could not change message pin state.")
			return
		}
		writeJSON(w, http.StatusOK, message)
	}
}

func updateProjectMessage(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("messageID"), 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "invalid_message_id", "message id must be a positive integer.")
			return
		}
		var input model.UpdateProjectMessageInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_project_message", "Project message input must be valid JSON.")
			return
		}
		if input.Author == "" {
			input.Author = r.Header.Get("X-Agent-ID")
		}
		msg, err := database.UpdateProjectMessage(r.Context(), r.PathValue("projectID"), id, input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "message_not_found", "Message was not found.")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_project_message", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, msg)
	}
}

func listProjectMessageRevisions(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("messageID"), 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "invalid_message_id", "message id must be a positive integer.")
			return
		}
		revisions, err := database.ListProjectMessageRevisions(r.Context(), r.PathValue("projectID"), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "message_not_found", "Message was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_project_message_revisions_failed", "Could not list message revisions.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": revisions})
	}
}

func getProjectMessageContext(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("messageID"), 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "invalid_message_id", "message id must be a positive integer.")
			return
		}
		options := model.ProjectMessageContextOptions{Query: r.URL.Query().Get("q")}
		for name, target := range map[string]*int{
			"thread_limit":   &options.ThreadLimit,
			"related_limit":  &options.RelatedLimit,
			"revision_limit": &options.RevisionLimit,
		} {
			raw := r.URL.Query().Get(name)
			if raw == "" {
				continue
			}
			value, parseErr := strconv.Atoi(raw)
			if parseErr != nil || value < 0 {
				writeError(w, http.StatusBadRequest, "invalid_context_limit", name+" must be a non-negative integer.")
				return
			}
			*target = value
		}
		bundle, err := database.GetProjectMessageContext(r.Context(), r.PathValue("projectID"), id, options)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "message_not_found", "Message was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "get_project_message_context_failed", "Could not hydrate message context.")
			return
		}
		writeJSON(w, http.StatusOK, bundle)
	}
}

func getProjectMessageThread(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("messageID"), 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "invalid_message_id", "message id must be a positive integer.")
			return
		}
		messages, err := database.GetProjectMessageThread(r.Context(), r.PathValue("projectID"), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "message_not_found", "Message was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "get_project_message_thread_failed", "Could not fetch message thread.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": messages})
	}
}

func searchProjectMessages(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		query := r.URL.Query()
		q := query.Get("q")
		limit := 0
		if rawLimit := query.Get("limit"); rawLimit != "" {
			n, err := strconv.Atoi(rawLimit)
			if err != nil || n < 0 || n > model.MaxGlobalSearchLimit {
				writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 0 and 200.")
				return
			}
			limit = n
		}
		tags := make([]string, 0)
		for _, raw := range query["tags"] {
			for _, tag := range strings.Split(raw, ",") {
				if tag = strings.TrimSpace(tag); tag != "" {
					tags = append(tags, tag)
				}
			}
		}
		after, err := parseGlobalSearchDate(query.Get("after"), false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_search_date", "after must be an ISO date or RFC3339 timestamp.")
			return
		}
		before, err := parseGlobalSearchDate(query.Get("before"), true)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_search_date", "before must be an ISO date or RFC3339 timestamp.")
			return
		}
		kind := strings.TrimSpace(query.Get("kind"))
		if kind != "" || len(tags) > 0 || after != nil || before != nil {
			hits, total, effectiveLimit, err := database.SearchProjectMessagesFiltered(r.Context(), projectID, q, model.ProjectMessageSearchFilter{Limit: limit, Kind: kind, Tags: tags, After: after, Before: before})
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
					return
				}
				if errors.Is(err, store.ErrValidation) {
					writeError(w, http.StatusBadRequest, "invalid_search", err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, "search_project_messages_failed", "Could not search project messages.")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": hits, "query": q, "total": total, "limit": effectiveLimit, "has_more": total > len(hits)})
			return
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
