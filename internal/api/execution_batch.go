package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func validateTasksBatch(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateTasksBatchInput
		if err := decodeExecutionBatchJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_tasks_batch", "Tasks batch input must be one valid JSON object with no unknown fields.")
			return
		}
		result, err := database.ValidateExecutionBatch(r.Context(), r.PathValue("projectID"), executionBatchFromLegacyTasks(input.Tasks))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func validateExecutionBatch(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.ExecutionBatchInput
		if err := decodeExecutionBatchJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_execution_batch", "Execution batch input must be one valid JSON object with no unknown fields.")
			return
		}
		result, err := database.ValidateExecutionBatch(r.Context(), r.PathValue("projectID"), input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func createExecutionBatch(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.ExecutionBatchInput
		if err := decodeExecutionBatchJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_execution_batch", "Execution batch input must be one valid JSON object with no unknown fields.")
			return
		}
		item, duplicate, err := submitRegistration(database, r, r.PathValue("projectID"), model.RegistrationQueueKindExecutionBatch, input)
		if err != nil {
			writeRegistrationError(w, err)
			return
		}
		writeExecutionBatchRegistration(w, item, duplicate)
	}
}

func decodeExecutionBatchJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request contains multiple JSON values")
		}
		return err
	}
	return nil
}
