package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/routing"
	"github.com/JustSebNL/timekeeper/internal/store"
)

type routingApplyRequest struct {
	Intent             string           `json:"intent"`
	AgentID            string           `json:"agent_id,omitempty"`
	SessionID          string           `json:"session_id,omitempty"`
	DecisionID         string           `json:"decision_id"`
	CandidateStateHash string           `json:"candidate_state_hash"`
	Confirmed          bool             `json:"confirmed"`
	Model              string           `json:"model,omitempty"`
	Decision           routing.Decision `json:"decision"`
}

type routingApplyResponse struct {
	DecisionID         string                       `json:"decision_id"`
	CandidateStateHash string                       `json:"candidate_state_hash"`
	QueueID            string                       `json:"queue_id,omitempty"`
	RegistrationStatus string                       `json:"registration_status,omitempty"`
	Applied            bool                         `json:"applied"`
	Duplicate          bool                         `json:"duplicate"`
	Outcome            routing.Outcome              `json:"outcome"`
	Result             *model.ExecutionBatchResult  `json:"result,omitempty"`
	Validation         *model.BatchValidationResult `json:"validation,omitempty"`
}

// routingApply is the only Core-owned write path for confirmed Jev proposals.
// It supports new Sprints under existing owners and new Task+Sprint pairs via
// the idempotent registration queue. Category creation and Sprint resumption
// are rejected until their exact lifecycle contracts are designed.
func routingApply(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input routingApplyRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRoutingDecisionRequestBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_routing_apply", "Routing Apply input must be one valid JSON object.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_routing_apply", "Routing Apply input must contain exactly one JSON object.")
			return
		}

		projectID := strings.TrimSpace(r.PathValue("projectID"))
		input.Intent = strings.TrimSpace(input.Intent)
		input.DecisionID = strings.TrimSpace(input.DecisionID)
		input.CandidateStateHash = strings.TrimSpace(input.CandidateStateHash)
		input.Model = strings.TrimSpace(input.Model)
		if input.Intent == "" || utf8.RuneCountInString(input.Intent) > routing.DefaultLimits().MaxIntentRunes {
			writeError(w, http.StatusBadRequest, "invalid_routing_apply", "Routing intent is required and must be at most 2048 characters.")
			return
		}
		if !input.Confirmed {
			writeError(w, http.StatusConflict, "routing_confirmation_required", "Routing Apply requires explicit human confirmation.")
			return
		}
		if input.DecisionID == "" || len(input.DecisionID) > 160 || input.Decision.Category.DecisionID != input.DecisionID {
			writeError(w, http.StatusBadRequest, "invalid_routing_apply", "decision_id must match the Category decision ID.")
			return
		}
		if input.CandidateStateHash == "" || !strings.HasPrefix(input.CandidateStateHash, "sha256:") {
			writeError(w, http.StatusBadRequest, "invalid_routing_apply", "candidate_state_hash is required.")
			return
		}
		if input.Decision.Category.PolicyVersion != routing.PolicyVersion {
			writeError(w, http.StatusConflict, "routing_policy_version_mismatch", "The decision uses an unsupported routing policy version.")
			return
		}
		if input.Decision.Work.Confirmed {
			writeError(w, http.StatusBadRequest, "invalid_routing_apply", "The model decision cannot assert human confirmation; use the confirmed field.")
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idempotencyKey == "" || len(idempotencyKey) > 160 {
			writeError(w, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key is required and must be at most 160 characters.")
			return
		}
		r.Header.Set("Idempotency-Key", idempotencyKey)

		batch, err := routingApplyBatch(input)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "routing_action_not_applicable", err.Error())
			return
		}
		payloadHash, err := executionBatchPayloadHash(batch)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_routing_apply", "Could not encode the confirmed routing payload.")
			return
		}
		if previous, err := database.GetRegistrationQueueItemByIdempotencyKey(r.Context(), projectID, idempotencyKey); err == nil {
			if previous.PayloadHash != payloadHash {
				writeRegistrationError(w, store.ErrRegistrationIdempotencyConflict)
				return
			}
			outcome := routingApplyOutcome(projectID, input, previous.Result)
			writeRoutingApplyQueueResult(w, previous, true, input, outcome)
			return
		} else if !errors.Is(err, store.ErrRegistrationQueueItemNotFound) {
			writeStoreError(w, err)
			return
		}

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
		if candidates.Truncated || candidates.StateHash != input.CandidateStateHash {
			writeError(w, http.StatusConflict, "routing_candidate_state_changed", "Candidate state changed since this decision was produced; refresh and re-confirm.")
			return
		}

		decision := input.Decision
		decision.Work.Confirmed = true
		service := routing.Service{Provider: submittedRoutingDecision{decision: decision}, Limits: routing.DefaultLimits()}
		outcome, err := service.Route(r.Context(), tree, routing.Request{Intent: input.Intent, AgentID: input.AgentID, SessionID: input.SessionID})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "routing_apply_validation_failed", "Could not revalidate the confirmed routing decision.")
			return
		}
		if outcome.CandidateStateHash != input.CandidateStateHash {
			writeError(w, http.StatusConflict, "routing_candidate_state_changed", "Candidate state changed during Apply validation; refresh and re-confirm.")
			return
		}
		if outcome.Outcome != routing.OutcomeProposed {
			writeJSON(w, http.StatusUnprocessableEntity, routingApplyResponse{DecisionID: input.DecisionID, CandidateStateHash: input.CandidateStateHash, Outcome: outcome})
			return
		}
		if outcome.Action != routing.ActionCreateSprint && outcome.Action != routing.ActionCreateTaskAndSprint {
			writeJSON(w, http.StatusUnprocessableEntity, routingApplyResponse{DecisionID: input.DecisionID, CandidateStateHash: input.CandidateStateHash, Outcome: outcome})
			return
		}

		item, duplicate, err := submitRegistration(database, r, projectID, model.RegistrationQueueKindExecutionBatch, batch)
		if err != nil {
			writeRegistrationError(w, err)
			return
		}
		writeRoutingApplyQueueResult(w, item, duplicate, input, outcome)
	}
}

func routingApplyBatch(input routingApplyRequest) (model.ExecutionBatchInput, error) {
	decisionJSON, err := json.Marshal(input.Decision)
	if err != nil {
		return model.ExecutionBatchInput{}, fmt.Errorf("encode routing decision: %w", err)
	}
	intentDigest := sha256.Sum256([]byte(input.Intent))
	batch := model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Planning: &model.ExecutionBatchPlanning{RoutingEvidence: &model.ExecutionBatchRoutingEvidence{
			DecisionID:         input.DecisionID,
			PolicyVersion:      input.Decision.Category.PolicyVersion,
			CandidateStateHash: input.CandidateStateHash,
			IntentHash:         "sha256:" + hex.EncodeToString(intentDigest[:]),
			Model:              input.Model,
			Confirmed:          input.Confirmed,
			Decision:           decisionJSON,
		}},
	}
	work := input.Decision.Work
	switch work.Disposition {
	case routing.ActionCreateSprint:
		sprint := model.CreateSprintInput{
			Name:                     strings.TrimSpace(work.ProposedName),
			Description:              input.Intent,
			Goal:                     input.Intent,
			Priority:                 "Normal",
			EstimatedDurationSeconds: work.EstimatedDurationSeconds,
			BufferPct:                0,
		}
		extension := model.ExecutionBatchExtension{Sprints: []model.CreateSprintInput{sprint}}
		if strings.TrimSpace(work.SubtaskID) != "" {
			extension.SubtaskID = strings.TrimSpace(work.SubtaskID)
		} else {
			extension.TaskID = strings.TrimSpace(work.TaskID)
		}
		batch.Extensions = []model.ExecutionBatchExtension{extension}
	case routing.ActionCreateTaskAndSprint:
		name := strings.TrimSpace(work.ProposedName)
		sprintName := "Initial Sprint: " + name
		task := model.ExecutionBatchTask{
			CategoryID:  strings.TrimSpace(input.Decision.Category.SelectedID),
			Name:        name,
			Description: input.Intent,
			Goal:        input.Intent,
			Priority:    "Normal",
			Sprints: []model.CreateSprintInput{{
				Name:                     sprintName,
				Description:              input.Intent,
				Goal:                     input.Intent,
				Priority:                 "Normal",
				EstimatedDurationSeconds: work.EstimatedDurationSeconds,
				BufferPct:                0,
			}},
		}
		batch.Tasks = []model.ExecutionBatchTask{task}
	default:
		return model.ExecutionBatchInput{}, fmt.Errorf("Core Apply currently supports only create_sprint and create_task_and_sprint; %q is read-only", work.Disposition)
	}
	return batch, nil
}

func executionBatchPayloadHash(input model.ExecutionBatchInput) (string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode execution batch payload: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func routingApplyOutcome(projectID string, input routingApplyRequest, result *model.ExecutionBatchResult) routing.Outcome {
	outcome := routing.Outcome{
		Version:            routing.DecisionVersion,
		PolicyVersion:      input.Decision.Category.PolicyVersion,
		Outcome:            routing.OutcomeProposed,
		Action:             input.Decision.Work.Disposition,
		ProjectID:          projectID,
		CategoryID:         input.Decision.Category.SelectedID,
		TaskID:             input.Decision.Work.TaskID,
		SubtaskID:          input.Decision.Work.SubtaskID,
		ProposedName:       input.Decision.Work.ProposedName,
		Confidence:         input.Decision.Work.Confidence,
		CandidateStateHash: input.CandidateStateHash,
		DecisionID:         input.DecisionID,
	}
	if result == nil {
		return outcome
	}
	if len(result.Tasks) > 0 {
		outcome.TaskID = result.Tasks[0].TaskID
		outcome.CategoryID = result.Tasks[0].CategoryID
	}
	if len(result.Subtasks) > 0 {
		outcome.SubtaskID = result.Subtasks[0].SubtaskID
		outcome.TaskID = result.Subtasks[0].TaskID
	}
	if len(result.Sprints) > 0 {
		outcome.SprintID = result.Sprints[0].SprintID
		outcome.TaskID = result.Sprints[0].TaskID
		outcome.SubtaskID = result.Sprints[0].SubtaskID
	}
	return outcome
}

func writeRoutingApplyQueueResult(w http.ResponseWriter, item model.RegistrationQueueItem, duplicate bool, input routingApplyRequest, outcome routing.Outcome) {
	response := routingApplyResponse{
		DecisionID:         input.DecisionID,
		CandidateStateHash: input.CandidateStateHash,
		QueueID:            item.QueueID,
		RegistrationStatus: item.Status,
		Duplicate:          duplicate,
		Outcome:            outcome,
	}
	switch item.Status {
	case model.RegistrationQueueStatusQueued:
		w.Header().Set("Retry-After", "1")
		writeJSON(w, http.StatusAccepted, response)
	case model.RegistrationQueueStatusRejected:
		response.Outcome.Outcome = routing.OutcomeNeedsReview
		response.Outcome.Action = routing.ActionNeedsReview
		response.Outcome.ReasonCode = "registration_rejected"
		response.Outcome.Reason = "Core rejected the execution batch during transactional revalidation."
		response.Validation = item.Validation
		writeJSON(w, http.StatusUnprocessableEntity, response)
	case model.RegistrationQueueStatusRegistered:
		response.Applied = true
		response.Result = item.Result
		writeJSON(w, registrationSuccessStatus(duplicate), response)
	default:
		writeError(w, http.StatusInternalServerError, "routing_apply_status_invalid", "Core Apply returned an unknown registration status.")
	}
}
