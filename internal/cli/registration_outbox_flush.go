package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func flushRegistrationOutboxEntry(entry registrationOutboxEntry, baseURL string) (registrationOutboxState, error) {
	latest, err := loadRegistrationOutboxEntry(entry.Offer.LocalID)
	if err != nil {
		return registrationOutboxState{}, err
	}
	itemDir := registrationOutboxItemDir(latest.Offer.LocalID)
	itemLock, err := acquireRegistrationOutboxLock(filepath.Join(itemDir, "flush"))
	if err != nil {
		return latest.State, fmt.Errorf("lock registration offer %s: %w", latest.Offer.LocalID, err)
	}
	defer itemLock.Close()
	entry, err = loadRegistrationOutboxEntry(latest.Offer.LocalID)
	if err != nil {
		return registrationOutboxState{}, err
	}
	state := entry.State
	if state.Status == registrationOutboxRegistered || state.Status == registrationOutboxRejected {
		return state, nil
	}
	state.Attempts++
	state.LastError = ""
	state.UpdatedAt = time.Now().UTC()

	if state.Status == registrationOutboxAwaitingCore && state.CoreQueueID != "" {
		state, err := pollCoreRegistration(entry.Offer, state, baseURL)
		if persistErr := appendRegistrationOutboxState(entry.Offer.LocalID, state); persistErr != nil {
			return state, errors.Join(err, fmt.Errorf("persist registration outbox state: %w", persistErr))
		}
		return state, err
	}

	if entry.Offer.ValidationPath != "" {
		response, err := sendRegistrationOutboxRequest(baseURL, entry.Offer.ValidationPath, "", entry.Offer.Body)
		if err != nil {
			state.LastError = boundedDiagnosticText(err.Error(), 2048)
			_ = appendRegistrationOutboxState(entry.Offer.LocalID, state)
			return state, err
		}
		validationBody, readErr := readAndCloseResponse(response)
		if readErr != nil {
			state.LastError = boundedDiagnosticText(readErr.Error(), 2048)
			_ = appendRegistrationOutboxState(entry.Offer.LocalID, state)
			return state, readErr
		}
		if response.StatusCode != http.StatusOK {
			message := formatAPIError(response.Status, validationBody, response.Header.Get("X-Request-ID"))
			state.LastError = boundedDiagnosticText(message, 2048)
			if response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests {
				state.Status = registrationOutboxRejected
			}
			_ = appendRegistrationOutboxState(entry.Offer.LocalID, state)
			if state.Status == registrationOutboxRejected {
				return state, nil
			}
			return state, errors.New(state.LastError)
		}
		var validation model.BatchValidationResult
		if err := json.Unmarshal(validationBody, &validation); err != nil {
			state.LastError = fmt.Sprintf("Core returned invalid validation JSON: %s", boundedDiagnosticText(err.Error(), 1024))
			_ = appendRegistrationOutboxState(entry.Offer.LocalID, state)
			return state, errors.New(state.LastError)
		}
		state.Validation, _ = json.Marshal(validation)
		if !validation.Valid {
			state.Status = registrationOutboxRejected
			state.LastError = fmt.Sprintf("Core smoke test rejected the offer with %d issue(s)", len(validation.Issues))
			if err := appendRegistrationOutboxState(entry.Offer.LocalID, state); err != nil {
				return state, err
			}
			return state, nil
		}
	}

	response, err := sendRegistrationOutboxRequest(baseURL, entry.Offer.SubmitPath, entry.Offer.IdempotencyKey, entry.Offer.Body)
	if err != nil {
		state.LastError = boundedDiagnosticText(err.Error(), 2048)
		if persistErr := appendRegistrationOutboxState(entry.Offer.LocalID, state); persistErr != nil {
			return state, errors.Join(err, fmt.Errorf("persist registration outbox state: %w", persistErr))
		}
		return state, err
	}
	body, readErr := readAndCloseResponse(response)
	if readErr != nil {
		state.LastError = boundedDiagnosticText(readErr.Error(), 2048)
		if persistErr := appendRegistrationOutboxState(entry.Offer.LocalID, state); persistErr != nil {
			return state, errors.Join(readErr, fmt.Errorf("persist registration outbox state: %w", persistErr))
		}
		return state, readErr
	}
	state.Result = append(json.RawMessage(nil), body...)
	state.CoreQueueID = response.Header.Get("X-TimeKeeper-Registration-Queue-ID")
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		state.Status = registrationOutboxRegistered
	case http.StatusAccepted:
		var queued model.RegistrationQueueItem
		if json.Unmarshal(body, &queued) == nil {
			if state.CoreQueueID == "" {
				state.CoreQueueID = queued.QueueID
			}
			state.CoreProjectID = queued.ProjectID
		}
		if state.CoreQueueID == "" {
			state.Status = registrationOutboxPending
			state.LastError = "Core accepted the offer but did not return a registration queue ID; retry is idempotent."
		} else {
			state.Status = registrationOutboxAwaitingCore
			state.LastError = "accepted by Core; waiting for registration worker"
		}
	case http.StatusUnprocessableEntity, http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
		state.Status = registrationOutboxRejected
		state.LastError = formatAPIError(response.Status, body, response.Header.Get("X-Request-ID"))
		if response.StatusCode == http.StatusUnprocessableEntity {
			var validation model.BatchValidationResult
			if json.Unmarshal(body, &validation) == nil {
				state.Validation, _ = json.Marshal(validation)
			}
		}
	default:
		state.LastError = formatAPIError(response.Status, body, response.Header.Get("X-Request-ID"))
		if response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests {
			state.Status = registrationOutboxRejected
		}
		if state.Status != registrationOutboxRejected {
			if persistErr := appendRegistrationOutboxState(entry.Offer.LocalID, state); persistErr != nil {
				return state, errors.Join(errors.New(state.LastError), fmt.Errorf("persist registration outbox state: %w", persistErr))
			}
			return state, errors.New(state.LastError)
		}
	}
	if err := appendRegistrationOutboxState(entry.Offer.LocalID, state); err != nil {
		return state, fmt.Errorf("persist registration outbox state: %w", err)
	}
	return state, nil
}

func pollCoreRegistration(offer registrationOutboxOffer, state registrationOutboxState, baseURL string) (registrationOutboxState, error) {
	projectID := state.CoreProjectID
	if projectID == "" {
		projectID = offer.ProjectID
	}
	if projectID == "" || state.CoreQueueID == "" {
		state.Status = registrationOutboxPending
		state.LastError = "local offer is missing the Project or Core queue ID; retrying with the same idempotency key"
		return state, errors.New(state.LastError)
	}
	path := "/api/v1/projects/" + projectID + "/registration-queue/" + state.CoreQueueID
	response, err := sendRegistrationOutboxRequest(baseURL, path, "", nil)
	if err != nil {
		state.LastError = boundedDiagnosticText(err.Error(), 2048)
		return state, err
	}
	body, err := readAndCloseResponse(response)
	if err != nil {
		state.LastError = boundedDiagnosticText(err.Error(), 2048)
		return state, err
	}
	if response.StatusCode != http.StatusOK {
		state.LastError = formatAPIError(response.Status, body, response.Header.Get("X-Request-ID"))
		if response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests {
			state.Status = registrationOutboxRejected
			state.Result = append(json.RawMessage(nil), body...)
			return state, nil
		}
		return state, errors.New(state.LastError)
	}
	var item model.RegistrationQueueItem
	if err := json.Unmarshal(body, &item); err != nil {
		state.LastError = fmt.Sprintf("Core returned invalid queue status JSON: %s", boundedDiagnosticText(err.Error(), 1024))
		return state, errors.New(state.LastError)
	}
	state.Result = append(json.RawMessage(nil), body...)
	switch item.Status {
	case model.RegistrationQueueStatusRegistered:
		state.Status = registrationOutboxRegistered
		state.LastError = ""
	case model.RegistrationQueueStatusRejected:
		state.Status = registrationOutboxRejected
		state.LastError = item.LastError
		if state.LastError == "" {
			state.LastError = "Core rejected the registration offer; see its retained validation details."
		}
	case model.RegistrationQueueStatusQueued:
		state.Status = registrationOutboxAwaitingCore
		state.LastError = item.LastError
		if state.LastError == "" {
			state.LastError = "accepted by Core; waiting for registration worker"
		}
	default:
		state.LastError = fmt.Sprintf("Core returned unknown registration status %q", boundedDiagnosticText(item.Status, 128))
		return state, errors.New(state.LastError)
	}
	return state, nil
}

func sendRegistrationOutboxRequest(baseURL, path, idempotencyKey string, body []byte) (*http.Response, error) {
	if !isSafeRegistrationOutboxPath(path) {
		return nil, fmt.Errorf("refusing unsafe registration API path %q", boundedDiagnosticText(path, 256))
	}
	method := http.MethodGet
	var requestBody io.Reader
	if body != nil {
		method = http.MethodPost
		requestBody = bytes.NewReader(body)
	}
	endpoint := strings.TrimRight(baseURL, "/") + path
	request, err := http.NewRequest(method, endpoint, requestBody)
	if err != nil {
		return nil, fmt.Errorf("create registration request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	client := newHTTPClient(10 * time.Second)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, endpoint, err)
	}
	return response, nil
}

func readAndCloseResponse(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAPIErrorBody))
	if err != nil {
		return nil, err
	}
	return body, nil
}
