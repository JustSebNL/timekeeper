// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/knowledge"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

// KnowledgeService is deliberately narrower than the rest of the API. The
// knowledge provider is supplied by a plugin; a nil service produces a
// typed unavailable response rather than preventing TimeKeeper from
// starting. Core never names a concrete plugin here.
type KnowledgeService interface {
	Status(context.Context) (knowledge.ProviderStatus, error)
	Search(context.Context, knowledge.KnowledgeQuery) ([]knowledge.KnowledgeResult, error)
	Get(context.Context, string) (knowledge.KnowledgeItem, error)
	Create(context.Context, knowledge.KnowledgeCandidate, string) (knowledge.KnowledgeItem, error)
}

func knowledgeRoutes(mux *http.ServeMux, service KnowledgeService, database *store.Store) {
	mux.HandleFunc("GET /api/v1/knowledge/status", knowledgeStatus(service))
	mux.HandleFunc("POST /api/v1/knowledge/search", knowledgeSearch(service))
	mux.HandleFunc("GET /api/v1/knowledge/{id}", knowledgeGet(service))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/messages/{messageID}/promote", resolveMessageProject(database, promoteProjectMessage(database, service)))
}

func knowledgeStatus(service KnowledgeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "provider_unavailable", "Memory Galaxy is unavailable.")
			return
		}
		status, err := service.Status(r.Context())
		if err != nil {
			writeKnowledgeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}

func knowledgeSearch(service KnowledgeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "provider_unavailable", "Memory Galaxy is unavailable.")
			return
		}
		var query knowledge.KnowledgeQuery
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&query); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_knowledge_query", "Knowledge search input must be valid JSON.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_knowledge_query", "Knowledge search input must contain one JSON object.")
			return
		}
		results, err := service.Search(r.Context(), query)
		if err != nil {
			writeKnowledgeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": results})
	}
}

func knowledgeGet(service KnowledgeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "provider_unavailable", "Memory Galaxy is unavailable.")
			return
		}
		item, err := service.Get(r.Context(), strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			writeKnowledgeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

type promoteMessageRequest struct {
	Title           string               `json:"title"`
	Type            knowledge.ItemType   `json:"type"`
	Visibility      knowledge.Visibility `json:"visibility"`
	RedactionStatus string               `json:"redaction_status"`
}

func promoteProjectMessage(database *store.Store, service KnowledgeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "provider_unavailable", "Memory Galaxy is unavailable.")
			return
		}
		messageID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("messageID")), 10, 64)
		if err != nil || messageID <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_message_id", "messageID must be a positive integer.")
			return
		}
		var input promoteMessageRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_promotion_request", "Promotion input must be one valid JSON object.")
			return
		}
		if input.RedactionStatus != "verified" {
			writeError(w, http.StatusBadRequest, "redaction_required", "Promotion requires redaction_status=verified.")
			return
		}
		projectID := r.PathValue("projectID")
		message, err := database.GetProjectMessage(r.Context(), projectID, messageID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "message_not_found", "Trail entry was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "trail_read_failed", "Could not read the Trail entry.")
			return
		}
		if sensitiveKnowledgeContent(message.Body) {
			writeError(w, http.StatusBadRequest, "sensitive_content", "Trail entry contains content that cannot be promoted.")
			return
		}
		title := strings.TrimSpace(input.Title)
		if title == "" {
			title = strings.TrimSpace(strings.SplitN(message.Body, "\n", 2)[0])
			if title == "" {
				title = message.Ref
			}
		}
		itemType := input.Type
		if itemType == "" {
			itemType = knowledge.ItemTypeKnowledge
		}
		visibility := input.Visibility
		if visibility == "" {
			visibility = knowledge.VisibilityPrivate
		}
		candidate := knowledge.KnowledgeCandidate{Title: title, Content: message.Body, Type: itemType, Visibility: visibility, Origin: knowledge.Origin{System: "timekeeper", ProjectID: projectID, TrailID: message.Ref}}
		promotionKey := fmt.Sprintf("trail:%s:promotion:%d", message.Ref, message.Revision)
		link, duplicate, err := database.CreateKnowledgeLink(r.Context(), model.CreateKnowledgeLinkInput{SourceType: model.KnowledgeSourceTrail, SourceID: message.Ref, PromotionKey: promotionKey})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "knowledge_link_failed", "Could not persist the promotion mapping.")
			return
		}
		item, err := service.Create(r.Context(), candidate, promotionKey)
		if err != nil {
			status := model.KnowledgePromotionFailedPermanent
			if knowledge.IsRetryable(err) {
				status = model.KnowledgePromotionFailedRetryable
			}
			_, _ = database.UpdateKnowledgeLink(r.Context(), link.LinkID, status, "", safeKnowledgeError(err))
			writeKnowledgeError(w, err)
			return
		}
		link, err = database.UpdateKnowledgeLink(r.Context(), link.LinkID, model.KnowledgePromotionCreated, item.ID, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "knowledge_link_failed", "Galaxy item was created but its promotion mapping could not be updated.")
			return
		}
		status := http.StatusCreated
		if duplicate {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]any{"link": link, "item": item, "duplicate": duplicate})
	}
}

func sensitiveKnowledgeContent(content string) bool {
	lower := strings.ToLower(content)
	for _, marker := range []string{"-----begin", "bearer ", "api_key", "api-key", "access_token", "refresh_token", "password=", "passwd=", "cookie:", "private key", "connection string", "sk-", "ghp_"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func safeKnowledgeError(err error) string {
	return knowledge.ErrorCode(err)
}

func writeKnowledgeError(w http.ResponseWriter, err error) {
	if providerErr := knowledge.AsProviderError(err); providerErr != nil {
		status := knowledge.HTTPStatus(err)
		if providerErr.Status == http.StatusNotFound {
			status = http.StatusNotFound
		}
		writeError(w, status, providerErr.Code, providerErr.Message)
		return
	}
	if errors.Is(err, knowledge.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "provider_unavailable", "Memory Galaxy is unavailable.")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_knowledge_request", err.Error())
}
