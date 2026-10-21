package cli

import (
	"fmt"
	"io"
)

func submitSingleRegistrationOffer(projectID, kind, submitPath, operation string, body []byte, baseURL string, out, errOut io.Writer) (registrationOutboxEntry, registrationOutboxState, int) {
	entry, err := enqueueRegistrationOutbox(projectID, kind, "", submitPath, body)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "%s: could not persist local registration offer: %s\n", operation, boundedDiagnosticText(err.Error(), 2048))
		return registrationOutboxEntry{}, registrationOutboxState{}, 1
	}
	state, flushErr := flushRegistrationOutboxEntry(entry, baseURL)
	switch state.Status {
	case registrationOutboxRegistered:
		return entry, state, 0
	case registrationOutboxPending:
		if jsonOutputEnabled {
			writeJSONResponse(out, map[string]any{"local_queue_id": entry.Offer.LocalID, "status": state.Status, "core_queue_id": state.CoreQueueID, "delivery_error": state.LastError})
		} else {
			_, _ = fmt.Fprintf(out, "Offer queued locally as %s; Core delivery is not confirmed. Run `tk queue flush` when Core is reachable.\n", entry.Offer.LocalID)
		}
		if state.LastError != "" {
			_, _ = fmt.Fprintf(errOut, "%s\n", boundedDiagnosticText(state.LastError, 2048))
		}
		if flushErr != nil && state.LastError == "" {
			_, _ = fmt.Fprintf(errOut, "%s\n", boundedDiagnosticText(flushErr.Error(), 2048))
		}
		return entry, state, 0
	case registrationOutboxAwaitingCore:
		if jsonOutputEnabled {
			writeJSONResponse(out, map[string]any{"local_queue_id": entry.Offer.LocalID, "core_queue_id": state.CoreQueueID, "status": state.Status})
		} else {
			_, _ = fmt.Fprintf(out, "Offer accepted by Core registration queue as %s; local receipt %s is retained. Run `tk queue flush` to refresh its status.\n", state.CoreQueueID, entry.Offer.LocalID)
		}
		return entry, state, 0
	case registrationOutboxRejected:
		_, _ = fmt.Fprintf(errOut, "%s: registration offer %s rejected: %s\n", operation, entry.Offer.LocalID, boundedDiagnosticText(state.LastError, 2048))
		return entry, state, 1
	default:
		message := "Core returned an unexpected registration state"
		if flushErr != nil {
			message = boundedDiagnosticText(flushErr.Error(), 2048)
		}
		_, _ = fmt.Fprintf(errOut, "%s: registration offer %s: %s\n", operation, entry.Offer.LocalID, message)
		return entry, state, 1
	}
}
