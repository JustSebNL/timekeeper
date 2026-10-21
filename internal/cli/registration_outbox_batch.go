package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func queueTaskBatchRegistration(projectID string, body []byte, validationPath string, nested bool, out, errOut io.Writer, baseURL string) int {
	kind := model.RegistrationQueueKindTaskBatch
	submitPath := "/api/v1/projects/" + projectID + "/tasks/batch"
	if nested {
		kind = model.RegistrationQueueKindExecutionBatch
		submitPath = "/api/v1/projects/" + projectID + "/execution-batches"
	}
	entry, err := enqueueRegistrationOutbox(projectID, kind, validationPath, submitPath, body)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "could not persist local registration offer: %s\n", boundedDiagnosticText(err.Error(), 2048))
		return 1
	}
	state, flushErr := flushRegistrationOutboxEntry(entry, baseURL)
	var validation model.BatchValidationResult
	validationKnown := len(state.Validation) > 0 && json.Unmarshal(state.Validation, &validation) == nil
	if validationKnown && validation.Valid && !jsonOutputEnabled {
		_, _ = fmt.Fprintf(out, "✓ Smoke test passed: Tasks=%d Subtasks=%d Sprints=%d Total=%d\n", validation.Counts.Tasks, validation.Counts.Subtasks, validation.Counts.Sprints, validation.Counts.TotalObjects)
	}
	if state.Status == registrationOutboxRejected {
		if validationKnown && !validation.Valid {
			_, _ = fmt.Fprintf(errOut, "✗ Smoke test failed; Core rejected the offer with %d issue(s):\n", len(validation.Issues))
			writeBatchIssues(errOut, validation.Issues)
		} else {
			_, _ = fmt.Fprintf(errOut, "registration offer %s rejected: %s\n", entry.Offer.LocalID, boundedDiagnosticText(state.LastError, 2048))
		}
		return 1
	}
	if state.Status == registrationOutboxPending {
		if jsonOutputEnabled {
			receipt := map[string]any{"local_queue_id": entry.Offer.LocalID, "status": state.Status, "core_queue_id": state.CoreQueueID, "delivery_error": state.LastError}
			if validationKnown {
				receipt["validation"] = validation
			}
			writeJSONResponse(out, receipt)
		} else if validationKnown && validation.Valid {
			_, _ = fmt.Fprintf(out, "Apply outcome is unknown; offer queued locally as %s and retained with the same retry key. Run `tk queue flush` when Core is reachable.\n", entry.Offer.LocalID)
		} else {
			_, _ = fmt.Fprintf(out, "Core is unavailable before the smoke test; offer queued locally as %s. `tk queue flush` will smoke-test before apply.\n", entry.Offer.LocalID)
		}
		if state.LastError != "" {
			_, _ = fmt.Fprintf(errOut, "%s\n", boundedDiagnosticText(state.LastError, 2048))
		}
		return 0
	}
	if state.Status == registrationOutboxAwaitingCore {
		if jsonOutputEnabled {
			writeJSONResponse(out, map[string]any{"local_queue_id": entry.Offer.LocalID, "core_queue_id": state.CoreQueueID, "status": state.Status})
		} else {
			_, _ = fmt.Fprintf(out, "Offer accepted by Core registration queue as %s; local receipt %s is retained. Run `tk queue flush` to refresh its status.\n", state.CoreQueueID, entry.Offer.LocalID)
		}
		return 0
	}
	if state.Status != registrationOutboxRegistered {
		message := "Core returned an unexpected registration state"
		if flushErr != nil {
			message = boundedDiagnosticText(flushErr.Error(), 2048)
		}
		_, _ = fmt.Fprintf(errOut, "offer %s: %s\n", entry.Offer.LocalID, message)
		return 1
	}
	if state.CoreQueueID != "" && !jsonOutputEnabled {
		_, _ = fmt.Fprintf(out, "Core registration queue: %s\n", state.CoreQueueID)
	}
	if nested {
		var created model.ExecutionBatchResult
		if err := json.Unmarshal(state.Result, &created); err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid batch result JSON: %s\n", boundedDiagnosticText(err.Error(), 1024))
			return 1
		}
		if jsonOutputEnabled {
			writeJSONResponse(out, created)
		} else {
			writeCreatedBatchRows(out, created.Tasks, created.Subtasks, created.Sprints)
		}
		return 0
	}
	var created struct {
		Tasks []model.Task `json:"tasks"`
	}
	if err := json.Unmarshal(state.Result, &created); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid task batch JSON: %s\n", boundedDiagnosticText(err.Error(), 1024))
		return 1
	}
	if jsonOutputEnabled {
		writeJSONResponse(out, created)
		return 0
	}
	for _, task := range created.Tasks {
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", task.TaskID, task.Name, task.ItemAddress)
	}
	return 0
}
