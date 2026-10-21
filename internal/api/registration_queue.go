package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

const registrationQueueIDHeader = "X-TimeKeeper-Registration-Queue-ID"

func submitRegistration(database *store.Store, r *http.Request, projectID, kind string, input model.ExecutionBatchInput) (model.RegistrationQueueItem, bool, error) {
	item, duplicate, err := database.EnqueueRegistration(r.Context(), projectID, kind, r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		return model.RegistrationQueueItem{}, false, err
	}
	if item.Status != model.RegistrationQueueStatusQueued {
		return item, duplicate, nil
	}
	processed, err := database.ProcessRegistrationQueueItem(r.Context(), item.QueueID)
	if err == nil {
		return processed, duplicate, nil
	}
	// The offer is already durable. Leave it queued for the background worker
	// rather than turning a transient processor failure into a lost request.
	queued, readErr := database.GetRegistrationQueueItem(r.Context(), projectID, item.QueueID)
	if readErr == nil && queued.Status == model.RegistrationQueueStatusQueued {
		queued.LastError = "accepted; registration worker will retry"
		return queued, duplicate, nil
	}
	return model.RegistrationQueueItem{}, false, err
}

func writeRegistrationError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrRegistrationIdempotencyConflict) {
		writeError(w, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was already used for a different registration payload.")
		return
	}
	writeStoreError(w, err)
}

func writeRegistrationQueueHeaders(w http.ResponseWriter, item model.RegistrationQueueItem, duplicate bool) {
	w.Header().Set(registrationQueueIDHeader, item.QueueID)
	if duplicate {
		w.Header().Set("Idempotent-Replay", "true")
	}
}

func writeRegistrationPendingOrRejected(w http.ResponseWriter, item model.RegistrationQueueItem, duplicate bool) bool {
	writeRegistrationQueueHeaders(w, item, duplicate)
	switch item.Status {
	case model.RegistrationQueueStatusQueued:
		w.Header().Set("Retry-After", "1")
		writeJSON(w, http.StatusAccepted, item)
		return true
	case model.RegistrationQueueStatusRejected:
		if item.Validation == nil {
			writeError(w, http.StatusUnprocessableEntity, "registration_rejected", "Registration was rejected by Core validation.")
			return true
		}
		writeJSON(w, http.StatusUnprocessableEntity, item.Validation)
		return true
	default:
		return false
	}
}

func registrationSuccessStatus(duplicate bool) int {
	if duplicate {
		return http.StatusOK
	}
	return http.StatusCreated
}

func writeExecutionBatchRegistration(w http.ResponseWriter, item model.RegistrationQueueItem, duplicate bool) {
	if writeRegistrationPendingOrRejected(w, item, duplicate) {
		return
	}
	if item.Status != model.RegistrationQueueStatusRegistered || item.Result == nil {
		writeError(w, http.StatusInternalServerError, "registration_result_missing", "Registered queue item has no result.")
		return
	}
	writeJSON(w, registrationSuccessStatus(duplicate), item.Result)
}

func writeLegacyTaskBatchRegistration(w http.ResponseWriter, item model.RegistrationQueueItem, duplicate bool) {
	if writeRegistrationPendingOrRejected(w, item, duplicate) {
		return
	}
	if item.Status != model.RegistrationQueueStatusRegistered || item.Result == nil {
		writeError(w, http.StatusInternalServerError, "registration_result_missing", "Registered queue item has no result.")
		return
	}
	writeJSON(w, registrationSuccessStatus(duplicate), map[string]any{"tasks": item.Result.Tasks, "count": len(item.Result.Tasks)})
}

func writeTaskRegistration(w http.ResponseWriter, item model.RegistrationQueueItem, duplicate bool) {
	writeRegistrationQueueHeaders(w, item, duplicate)
	if item.Status == model.RegistrationQueueStatusQueued {
		w.Header().Set("Retry-After", "1")
		writeJSON(w, http.StatusAccepted, item)
		return
	}
	if item.Status == model.RegistrationQueueStatusRejected {
		if hasRegistrationIssue(item.Validation, "project_not_found", "category_not_found", "foreign_category") {
			writeError(w, http.StatusNotFound, "project_or_category_not_found", "Project or Category was not found.")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_task", registrationIssueMessage(item.Validation))
		return
	}
	if item.Status != model.RegistrationQueueStatusRegistered || item.Result == nil || len(item.Result.Tasks) == 0 {
		writeError(w, http.StatusInternalServerError, "registration_result_missing", "Registered queue item has no Task result.")
		return
	}
	writeJSON(w, registrationSuccessStatus(duplicate), item.Result.Tasks[0])
}

func writeSubtaskRegistration(w http.ResponseWriter, item model.RegistrationQueueItem, duplicate bool) {
	writeRegistrationQueueHeaders(w, item, duplicate)
	if item.Status == model.RegistrationQueueStatusQueued {
		w.Header().Set("Retry-After", "1")
		writeJSON(w, http.StatusAccepted, item)
		return
	}
	if item.Status == model.RegistrationQueueStatusRejected {
		if hasRegistrationIssue(item.Validation, "owner_not_found", "foreign_owner") {
			writeError(w, http.StatusNotFound, "task_not_found", "Task was not found.")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_subtask", registrationIssueMessage(item.Validation))
		return
	}
	if item.Status != model.RegistrationQueueStatusRegistered || item.Result == nil || len(item.Result.Subtasks) == 0 {
		writeError(w, http.StatusInternalServerError, "registration_result_missing", "Registered queue item has no Subtask result.")
		return
	}
	writeJSON(w, registrationSuccessStatus(duplicate), item.Result.Subtasks[0])
}

func writeSprintRegistration(w http.ResponseWriter, item model.RegistrationQueueItem, duplicate bool) {
	writeRegistrationQueueHeaders(w, item, duplicate)
	if item.Status == model.RegistrationQueueStatusQueued {
		w.Header().Set("Retry-After", "1")
		writeJSON(w, http.StatusAccepted, item)
		return
	}
	if item.Status == model.RegistrationQueueStatusRejected {
		if hasRegistrationIssue(item.Validation, "owner_not_found", "foreign_owner") {
			writeError(w, http.StatusNotFound, "task_or_subtask_not_found", "Sprint owner was not found.")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_sprint", registrationIssueMessage(item.Validation))
		return
	}
	if item.Status != model.RegistrationQueueStatusRegistered || item.Result == nil || len(item.Result.Sprints) == 0 {
		writeError(w, http.StatusInternalServerError, "registration_result_missing", "Registered queue item has no Sprint result.")
		return
	}
	writeJSON(w, registrationSuccessStatus(duplicate), item.Result.Sprints[0])
}

func hasRegistrationIssue(validation *model.BatchValidationResult, codes ...string) bool {
	if validation == nil {
		return false
	}
	for _, issue := range validation.Issues {
		for _, code := range codes {
			if issue.Code == code {
				return true
			}
		}
	}
	return false
}

func registrationIssueMessage(validation *model.BatchValidationResult) string {
	if validation == nil || len(validation.Issues) == 0 {
		return "Core could not register this offer. Inspect the registration queue for details."
	}
	return validation.Issues[0].Message
}

func listRegistrationQueue(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 100 {
				writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100.")
				return
			}
			limit = parsed
		}
		offset := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 0 || parsed > 1_000_000 {
				writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be between 0 and 1000000.")
				return
			}
			offset = parsed
		}
		items, hasMore, err := database.ListRegistrationQueuePage(r.Context(), r.PathValue("projectID"), r.URL.Query().Get("status"), limit, offset)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		var nextOffset any
		if hasMore {
			nextOffset = offset + len(items)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "limit": limit, "offset": offset, "has_more": hasMore, "next_offset": nextOffset})
	}
}

func getRegistrationQueueItem(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := database.GetRegistrationQueueItem(r.Context(), r.PathValue("projectID"), r.PathValue("queueID"))
		if err != nil {
			if errors.Is(err, store.ErrRegistrationQueueItemNotFound) {
				writeError(w, http.StatusNotFound, "registration_not_found", "Registration queue item was not found in this Project.")
				return
			}
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func executionBatchFromLegacyTasks(tasks []model.CreateTaskInput) model.ExecutionBatchInput {
	input := model.ExecutionBatchInput{Version: "timekeeper-execution-batch/v1", Tasks: make([]model.ExecutionBatchTask, 0, len(tasks))}
	for _, task := range tasks {
		categoryID := strings.TrimSpace(task.CategoryID)
		if categoryID == "" || strings.EqualFold(categoryID, "default") || strings.EqualFold(categoryID, "uncategorized") || strings.EqualFold(categoryID, "general") {
			categoryID = "C-00000"
		}
		input.Tasks = append(input.Tasks, model.ExecutionBatchTask{
			CategoryID:               categoryID,
			Name:                     task.Name,
			Description:              task.Description,
			Goal:                     task.Goal,
			Priority:                 task.Priority,
			EstimatedDurationSeconds: task.EstimatedDurationSeconds,
			AutoClose:                task.AutoClose,
		})
	}
	return input
}

func executionBatchFromTask(task model.CreateTaskInput) model.ExecutionBatchInput {
	return executionBatchFromLegacyTasks([]model.CreateTaskInput{task})
}

func executionBatchFromSubtask(taskID string, subtask model.CreateSubtaskInput) model.ExecutionBatchInput {
	return model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Extensions: []model.ExecutionBatchExtension{{
			TaskID: taskID,
			Subtasks: []model.ExecutionBatchSubtask{{
				Name:                     subtask.Name,
				Description:              subtask.Description,
				Goal:                     subtask.Goal,
				Priority:                 subtask.Priority,
				EstimatedDurationSeconds: subtask.EstimatedDurationSeconds,
			}},
		}},
	}
}

func executionBatchFromTaskSprints(taskID string, sprint model.CreateSprintInput) model.ExecutionBatchInput {
	return model.ExecutionBatchInput{
		Version:    "timekeeper-execution-batch/v1",
		Extensions: []model.ExecutionBatchExtension{{TaskID: taskID, Sprints: []model.CreateSprintInput{sprint}}},
	}
}

func executionBatchFromSubtaskSprints(subtaskID string, sprint model.CreateSprintInput) model.ExecutionBatchInput {
	return model.ExecutionBatchInput{
		Version:    "timekeeper-execution-batch/v1",
		Extensions: []model.ExecutionBatchExtension{{SubtaskID: subtaskID, Sprints: []model.CreateSprintInput{sprint}}},
	}
}
