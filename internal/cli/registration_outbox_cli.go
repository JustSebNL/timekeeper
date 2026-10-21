package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func runRegistrationQueueCommand(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, _ = fmt.Fprintln(out, "usage: tk queue list | show <local-id> | flush [--limit N]")
		return 0
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			_, _ = fmt.Fprintln(errOut, "usage: tk queue list")
			return 2
		}
		return listRegistrationOutboxCommand(out, errOut)
	case "show":
		if len(args) != 2 {
			_, _ = fmt.Fprintln(errOut, "usage: tk queue show <local-id>")
			return 2
		}
		return showRegistrationOutboxCommand(args[1], out, errOut)
	case "flush":
		limit := 50
		for index := 1; index < len(args); index++ {
			arg := args[index]
			if strings.HasPrefix(arg, "--limit=") {
				value := strings.TrimPrefix(arg, "--limit=")
				parsed, err := strconv.Atoi(value)
				if err != nil || parsed < 1 || parsed > 1000 {
					_, _ = fmt.Fprintln(errOut, "--limit must be between 1 and 1000")
					return 2
				}
				limit = parsed
				continue
			}
			if arg == "--limit" && index+1 < len(args) {
				index++
				parsed, err := strconv.Atoi(args[index])
				if err != nil || parsed < 1 || parsed > 1000 {
					_, _ = fmt.Fprintln(errOut, "--limit must be between 1 and 1000")
					return 2
				}
				limit = parsed
				continue
			}
			_, _ = fmt.Fprintf(errOut, "unknown queue flush option %q\nusage: tk queue flush [--limit N]\n", arg)
			return 2
		}
		return flushRegistrationOutboxCommand(limit, baseURL, out, errOut)
	default:
		_, _ = fmt.Fprintf(errOut, "unknown queue command %q\nusage: tk queue list | show <local-id> | flush [--limit N]\n", args[0])
		return 2
	}
}

func listRegistrationOutboxCommand(out, errOut io.Writer) int {
	entries, err := listRegistrationOutbox()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "list local registration outbox: %s\n", boundedDiagnosticText(err.Error(), 2048))
		return 1
	}
	if jsonOutputEnabled {
		writeJSONResponse(out, entries)
		return 0
	}
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out, "Local registration outbox is empty.")
		return 0
	}
	_, _ = fmt.Fprintln(out, "LOCAL ID\tSTATUS\tKIND\tPROJECT\tCORE QUEUE ID\tCREATED")
	for _, entry := range entries {
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\n", entry.Offer.LocalID, entry.State.Status, entry.Offer.Kind, entry.Offer.ProjectID, entry.State.CoreQueueID, entry.Offer.CreatedAt.Format("2006-01-02 15:04:05"))
		if entry.State.LastError != "" {
			_, _ = fmt.Fprintf(out, "  last error: %s\n", boundedDiagnosticText(entry.State.LastError, 1024))
		}
	}
	return 0
}

func showRegistrationOutboxCommand(localID string, out, errOut io.Writer) int {
	entry, err := loadRegistrationOutboxEntry(localID)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "read local registration offer: %s\n", boundedDiagnosticText(err.Error(), 2048))
		return 1
	}
	if jsonOutputEnabled {
		writeJSONResponse(out, entry)
		return 0
	}
	_, _ = fmt.Fprintf(out, "Local ID: %s\nKind: %s\nProject: %s\nStatus: %s\nCreated: %s\nIdempotency-Key: %s\n", entry.Offer.LocalID, entry.Offer.Kind, entry.Offer.ProjectID, entry.State.Status, entry.Offer.CreatedAt.Format("2006-01-02 15:04:05"), entry.Offer.IdempotencyKey)
	if entry.State.CoreQueueID != "" {
		_, _ = fmt.Fprintf(out, "Core queue ID: %s\n", entry.State.CoreQueueID)
	}
	if entry.State.LastError != "" {
		_, _ = fmt.Fprintf(out, "Last error: %s\n", boundedDiagnosticText(entry.State.LastError, 2048))
	}
	if len(entry.State.Validation) > 0 {
		var validation model.BatchValidationResult
		if json.Unmarshal(entry.State.Validation, &validation) == nil {
			_, _ = fmt.Fprintf(out, "Smoke validation: valid=%t Tasks=%d Subtasks=%d Sprints=%d\n", validation.Valid, validation.Counts.Tasks, validation.Counts.Subtasks, validation.Counts.Sprints)
			if len(validation.Issues) > 0 {
				writeBatchIssues(out, validation.Issues)
			}
		}
	}
	var payload any
	if json.Unmarshal(entry.Offer.Body, &payload) == nil {
		_, _ = fmt.Fprintln(out, "Payload:")
		encoded, _ := json.MarshalIndent(payload, "  ", "  ")
		_, _ = fmt.Fprintln(out, string(encoded))
	}
	return 0
}

func flushRegistrationOutboxCommand(limit int, baseURL string, out, errOut io.Writer) int {
	entries, err := listRegistrationOutbox()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "read local registration outbox: %s\n", boundedDiagnosticText(err.Error(), 2048))
		return 1
	}
	processed := 0
	pending := 0
	deliveryFailed := false
	rejected := false
	results := make([]map[string]any, 0)
	for index, entry := range entries {
		if entry.State.Status != registrationOutboxPending && entry.State.Status != registrationOutboxAwaitingCore {
			continue
		}
		if processed >= limit {
			pending++
			continue
		}
		state, flushErr := flushRegistrationOutboxEntry(entry, baseURL)
		processed++
		result := map[string]any{"local_queue_id": entry.Offer.LocalID, "status": state.Status, "core_queue_id": state.CoreQueueID}
		if state.LastError != "" {
			result["last_error"] = state.LastError
		}
		results = append(results, result)
		if !jsonOutputEnabled {
			_, _ = fmt.Fprintf(out, "%s	%s", entry.Offer.LocalID, state.Status)
			if state.CoreQueueID != "" {
				_, _ = fmt.Fprintf(out, "	%s", state.CoreQueueID)
			}
			_, _ = fmt.Fprintln(out)
		}
		if state.LastError != "" {
			_, _ = fmt.Fprintf(errOut, "%s: %s\n", entry.Offer.LocalID, boundedDiagnosticText(state.LastError, 2048))
		}
		if state.Status == registrationOutboxPending || state.Status == registrationOutboxAwaitingCore {
			pending++
		}
		if state.Status == registrationOutboxPending || flushErr != nil {
			deliveryFailed = true
		}
		if state.Status == registrationOutboxRejected {
			rejected = true
		}
		if flushErr != nil {
			_, _ = fmt.Fprintf(errOut, "%s: delivery remains durable and retryable: %s\n", entry.Offer.LocalID, boundedDiagnosticText(flushErr.Error(), 2048))
			for _, later := range entries[index+1:] {
				if later.State.Status == registrationOutboxPending || later.State.Status == registrationOutboxAwaitingCore {
					pending++
				}
			}
			break
		}
	}
	if jsonOutputEnabled {
		writeJSONResponse(out, map[string]any{"processed": processed, "pending": pending, "results": results})
	}
	if processed == 0 {
		if !jsonOutputEnabled {
			_, _ = fmt.Fprintln(out, "No pending local registration offers to flush.")
		}
		return 0
	}
	if pending > 0 && !jsonOutputEnabled {
		_, _ = fmt.Fprintf(out, "%d local offer(s) remain pending or awaiting Core.\n", pending)
	}
	if rejected {
		if !jsonOutputEnabled {
			_, _ = fmt.Fprintln(out, "One or more offers were rejected by Core; inspect them with `tk queue show <local-id>`.")
		}
		return 1
	}
	if deliveryFailed {
		return 1
	}
	if !jsonOutputEnabled {
		_, _ = fmt.Fprintf(out, "Processed %d local registration offer(s).\n", processed)
	}
	return 0
}
