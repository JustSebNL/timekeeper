package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/routing"
	"github.com/JustSebNL/timekeeper/internal/store"
)

const maxRoutingDecisionRequestBytes = 64 << 10

type routingDecisionRequest struct {
	Intent             string           `json:"intent"`
	AgentID            string           `json:"agent_id,omitempty"`
	SessionID          string           `json:"session_id,omitempty"`
	CandidateStateHash string           `json:"candidate_state_hash"`
	Decision           routing.Decision `json:"decision"`
}

type routingCategoryRequest struct {
	Intent             string                   `json:"intent"`
	CandidateStateHash string                   `json:"candidate_state_hash"`
	Decision           routing.CategoryDecision `json:"decision"`
}

type submittedRoutingDecision struct {
	decision routing.Decision
}

func (provider submittedRoutingDecision) Status(context.Context) (routing.ProviderStatus, error) {
	return routing.ProviderStatus{
		Available:       true,
		Provider:        "external-typed-decision",
		ProtocolVersion: routing.ProtocolVersion,
	}, nil
}

func (provider submittedRoutingDecision) Route(context.Context, routing.Request) (routing.Decision, error) {
	return provider.decision, nil
}

// routingDecisionEvaluation is a read-only Core policy boundary for typed
// proposals produced by an optional adapter such as ShadowBox. The Core fetches
// candidates itself; callers cannot supply or widen the candidate set.
func routingDecisionEvaluation(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input routingDecisionRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRoutingDecisionRequestBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_routing_decision", "Routing decision input must be one valid JSON object.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_routing_decision", "Routing decision input must contain exactly one JSON object.")
			return
		}
		if strings.TrimSpace(input.Intent) == "" {
			writeError(w, http.StatusBadRequest, "invalid_routing_decision", "Routing intent is required.")
			return
		}
		if strings.TrimSpace(input.CandidateStateHash) == "" {
			writeError(w, http.StatusBadRequest, "invalid_routing_decision", "Candidate state hash is required.")
			return
		}

		projectID := strings.TrimSpace(r.PathValue("projectID"))
		tree, err := database.ProjectExecutionTree(r.Context(), projectID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeStoreError(w, err)
			return
		}
		candidateSet := routing.CandidateSetFromTree(tree, routing.DefaultLimits())
		if input.CandidateStateHash != candidateSet.StateHash {
			writeError(w, http.StatusConflict, "routing_candidate_state_changed", "Candidate state changed since the routing decision was produced.")
			return
		}

		service := routing.Service{Provider: submittedRoutingDecision{decision: input.Decision}, Limits: routing.DefaultLimits()}
		outcome, err := service.Route(r.Context(), tree, routing.Request{
			Intent:    input.Intent,
			AgentID:   input.AgentID,
			SessionID: input.SessionID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "routing_evaluation_failed", "Could not evaluate the typed routing proposal.")
			return
		}
		writeJSON(w, http.StatusOK, outcome)
	}
}

func routingDecisionRoutes(mux *http.ServeMux, database *store.Store) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/routing/candidates", routingCandidateSnapshot(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/routing/category", routingCategoryEvaluation(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/routing/decide", routingDecisionEvaluation(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/routing/apply", routingApply(database))
}

// routingCandidateSnapshot exposes the exact bounded candidate projection and
// hash used by Core policy so an adapter can bind its model pass to this state.
func routingCandidateSnapshot(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := strings.TrimSpace(r.PathValue("projectID"))
		tree, err := database.ProjectExecutionTree(r.Context(), projectID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, routing.CandidateSetFromTree(tree, routing.DefaultLimits()))
	}
}

// routingCategoryEvaluation validates the first, Category-only pass of a
// hierarchical decision. It is read-only and fetches candidates from Core.
func routingCategoryEvaluation(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input routingCategoryRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRoutingDecisionRequestBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_routing_category", "Category routing input must be one valid JSON object.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_routing_category", "Category routing input must contain exactly one JSON object.")
			return
		}
		if strings.TrimSpace(input.Intent) == "" {
			writeError(w, http.StatusBadRequest, "invalid_routing_category", "Routing intent is required.")
			return
		}
		if input.Decision.PolicyVersion != routing.PolicyVersion {
			writeError(w, http.StatusConflict, "routing_policy_version_mismatch", "Category decision policy version is unsupported.")
			return
		}
		if strings.TrimSpace(input.CandidateStateHash) == "" {
			writeError(w, http.StatusBadRequest, "invalid_routing_category", "Candidate state hash is required.")
			return
		}
		projectID := strings.TrimSpace(r.PathValue("projectID"))
		tree, err := database.ProjectExecutionTree(r.Context(), projectID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeStoreError(w, err)
			return
		}
		candidates := routing.CandidateSetFromTree(tree, routing.DefaultLimits())
		if input.CandidateStateHash != candidates.StateHash {
			writeError(w, http.StatusConflict, "routing_candidate_state_changed", "Candidate state changed since the Category decision was produced.")
			return
		}
		writeJSON(w, http.StatusOK, routing.EvaluateCategory(candidates, input.Decision))
	}
}
