// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package knowledge

import (
	"errors"
	"fmt"
	"net/http"
)

// ProviderError is the provider-agnostic error every knowledge plugin
// returns. Core classifies failures through this type alone, so a
// plugin never has to be imported by the HTTP layer to be understood.
//
// A plugin maps its own transport failures onto this shape at its
// boundary. Core never switches on a concrete plugin error type.
type ProviderError struct {
	Code      string
	Message   string
	Retryable bool
	Status    int
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "knowledge provider error"
	}
	if e.Message == "" {
		return "knowledge provider error: " + e.Code
	}
	return e.Message
}

// Is implements errors.Is so plugins may wrap their own errors and
// still be classified by core.
func (e *ProviderError) Is(target error) bool {
	t, ok := target.(*ProviderError)
	if !ok {
		return false
	}
	if e == nil {
		return t == nil
	}
	return t == nil || e.Code == t.Code
}

// AsProviderError extracts a *ProviderError from err, following the
// Unwrap chain. Returns nil when err is not a provider error, which
// callers treat as an opaque internal failure.
func AsProviderError(err error) *ProviderError {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr
	}
	return nil
}

// IsRetryable reports whether a failure is worth retrying. Anything
// unrecognised is treated as permanent rather than retried forever.
func IsRetryable(err error) bool {
	providerErr := AsProviderError(err)
	return providerErr != nil && providerErr.Retryable
}

// ErrorCode returns a stable, safe-to-log code for a failure.
// Unrecognised errors collapse to a generic code so internal details
// never reach a client or a persisted promotion record.
func ErrorCode(err error) string {
	if providerErr := AsProviderError(err); providerErr != nil && providerErr.Code != "" {
		return providerErr.Code
	}
	return "provider request failed"
}

// HTTPStatus maps a provider error onto a response status. Unknown
// failures become 502 rather than 500: the plugin is upstream of core.
func HTTPStatus(err error) int {
	providerErr := AsProviderError(err)
	if providerErr == nil {
		return http.StatusBadGateway
	}
	if providerErr.Status <= 0 {
		return http.StatusBadGateway
	}
	return providerErr.Status
}

// NewProviderError builds a ProviderError with a guaranteed code, so
// classification never sees an empty one.
func NewProviderError(code, message string, status int, retryable bool) *ProviderError {
	if code == "" {
		code = "provider_request_failed"
	}
	return &ProviderError{Code: code, Message: message, Status: status, Retryable: retryable}
}

// Wrapf annotates a provider error while keeping its classification.
func Wrapf(err error, format string, args ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}
