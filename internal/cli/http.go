// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/JustSebNL/timekeeper/internal/logging"
	"github.com/JustSebNL/timekeeper/internal/version"
)

const maxAPIErrorBody = 16 << 10

type apiErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// formatAPIError preserves the server's structured reason instead of reducing
// every failed request to an opaque HTTP status line. The body is bounded so a
// broken proxy cannot flood the terminal or log.
func formatAPIError(status string, body []byte, requestID string) string {
	status = boundedDiagnosticText(status, 256)
	var envelope apiErrorEnvelope
	if json.Unmarshal(body, &envelope) == nil && (envelope.Error.Code != "" || envelope.Error.Message != "") {
		parts := []string{status}
		if code := boundedDiagnosticText(envelope.Error.Code, 256); code != "" {
			parts = append(parts, "code="+code)
		}
		if message := boundedDiagnosticText(envelope.Error.Message, 2048); message != "" {
			parts = append(parts, "message="+message)
		}
		if requestID := boundedDiagnosticText(requestID, 256); requestID != "" {
			parts = append(parts, "request_id="+requestID)
		}
		return strings.Join(parts, " ")
	}
	fallback := boundedDiagnosticText(string(body), 2048)
	if fallback != "" {
		status += " body=" + fallback
	}
	if requestID := boundedDiagnosticText(requestID, 256); requestID != "" {
		status += " request_id=" + requestID
	}
	return status
}

func boundedDiagnosticText(value string, limit int) string {
	value = strings.ToValidUTF8(value, "�")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > limit {
		value = string([]rune(value)[:limit]) + "..."
	}
	return value
}

// readHTTPErrorDetail returns the bounded structured reason for a failed
// response. Prefer it over response.Status so callers never reduce a named
// server constraint to an opaque status line.
func readHTTPErrorDetail(response *http.Response) string {
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxAPIErrorBody))
	return formatAPIError(response.Status, body, response.Header.Get("X-Request-ID"))
}

// reportHTTPStatus reads a bounded error response and prints its structured
// code/message. Call it only on a non-success response.
func reportHTTPStatus(errOut io.Writer, operation string, response *http.Response) {
	_, _ = fmt.Fprintf(errOut, "%s: %s\n", operation, readHTTPErrorDetail(response))
}

func formatConnectionFailure(method, endpoint string, err error) string {
	reason := err.Error()
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		reason = fmt.Sprintf("DNS lookup failed for %s: %s", dnsErr.Name, dnsErr.Err)
	} else {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			reason = fmt.Sprintf("network %s failed: %s", opErr.Op, opErr.Err)
		}
	}
	return fmt.Sprintf("%s %s: connection failed: %s", method, endpoint, reason)
}

type diagnosticTransport struct {
	base http.RoundTripper
}

var activeCommand atomic.Value // string

func setActiveCommand(args []string) {
	if len(args) == 0 {
		activeCommand.Store("")
		return
	}
	label := args[0]
	if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		label += " " + args[1]
	}
	activeCommand.Store(label)
}

func currentCommand() string {
	value := activeCommand.Load()
	if value == nil {
		return ""
	}
	return value.(string)
}

type connectionFailure struct {
	method    string
	endpoint  string
	requestID string
	client    string
	agentID   string
	command   string
	kind      string
	cause     error
}

func (e *connectionFailure) Error() string {
	return fmt.Sprintf("%s %s: connection failed (%s): %v [request_id=%s client_version=%s agent_id=%s command=%s]",
		e.method, e.endpoint, e.kind, e.cause, e.requestID, e.client, e.agentID, e.command)
}

func (e *connectionFailure) Unwrap() error { return e.cause }

func (t diagnosticTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	if clone.Header.Get("User-Agent") == "" {
		clone.Header.Set("User-Agent", fmt.Sprintf("tk/%s (%s/%s)", version.String(), runtime.GOOS, runtime.GOARCH))
	}
	clone.Header.Set("X-TimeKeeper-Client", version.String())
	clone.Header.Set("X-TimeKeeper-Surface", "cli")
	identity := resolveAgentIdentity()
	if tracked := currentFASE.Load(); tracked != nil {
		// Display-author options are not caller identity. Also remove explicitly
		// supplied or redirect-copied identity headers outside the enrolled Core.
		for _, key := range []string{"X-Agent-ID", "X-Agent-Session-ID", "X-Agent-Runtime"} {
			clone.Header.Del(key)
		}
		if !sameFASEOrigin(clone.URL, tracked.baseURL) {
			identity = agentIdentity{}
		}
	}
	if clone.Header.Get("X-Agent-ID") == "" {
		if identity.AgentID != "" {
			clone.Header.Set("X-Agent-ID", identity.AgentID)
		}
	}
	if clone.Header.Get("X-Agent-ID") == identity.AgentID && identity.SessionID != "" {
		if clone.Header.Get("X-Agent-Session-ID") == "" {
			clone.Header.Set("X-Agent-Session-ID", identity.SessionID)
		}
		if clone.Header.Get("X-Agent-Runtime") == "" {
			clone.Header.Set("X-Agent-Runtime", identity.Runtime)
		}
	}
	if clone.Header.Get("X-Request-ID") == "" {
		clone.Header.Set("X-Request-ID", fmt.Sprintf("tk-%d", time.Now().UnixNano()))
	}
	if command := currentCommand(); command != "" {
		clone.Header.Set("X-TimeKeeper-Command", command)
	}
	start := time.Now()
	// Retry on database_busy (503) with backoff instead of failing immediately.
	// This effectively queues the request until the database is available.
	var response *http.Response
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		// Clone the request body for each attempt
		if clone.Body != nil {
			bodyBytes, _ := io.ReadAll(clone.Body)
			clone.Body.Close()
			clone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			clone.ContentLength = int64(len(bodyBytes))
		}
		response, err = base.RoundTrip(clone)
		if err != nil {
			break
		}
		if response.StatusCode != http.StatusServiceUnavailable {
			break
		}
		// Check if it's database_busy
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		var envelope apiErrorEnvelope
		isBusy := false
		if json.Unmarshal(body, &envelope) == nil && envelope.Error.Code == "database_busy" {
			isBusy = true
		}
		if !isBusy {
			// Not a database_busy error; restore body and return
			response.Body = io.NopCloser(bytes.NewReader(body))
			break
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
		// Backoff before retry
		wait := time.Duration(attempt+1) * 200 * time.Millisecond
		if wait > 2*time.Second {
			wait = 2 * time.Second
		}
		time.Sleep(wait)
	}
	if err != nil {
		failure := &connectionFailure{
			method:    clone.Method,
			endpoint:  diagnosticEndpoint(clone),
			requestID: clone.Header.Get("X-Request-ID"),
			client:    clone.Header.Get("X-TimeKeeper-Client"),
			agentID:   clone.Header.Get("X-Agent-ID"),
			command:   clone.Header.Get("X-TimeKeeper-Command"),
			kind:      connectionErrorKind(err),
			cause:     err,
		}
		logging.L().Error("http connection failed",
			slog.String("method", failure.method),
			slog.String("endpoint", failure.endpoint),
			slog.String("request_id", failure.requestID),
			slog.String("client_version", failure.client),
			slog.String("user_agent", clone.Header.Get("User-Agent")),
			slog.String("agent_id", failure.agentID),
			slog.String("command", failure.command),
			slog.Int64("request_bytes", clone.ContentLength),
			slog.String("content_type", clone.Header.Get("Content-Type")),
			slog.String("error_kind", failure.kind),
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(start)),
		)
		return nil, failure
	} else if response.StatusCode >= http.StatusBadRequest {
		logging.L().Warn("http response failed",
			slog.String("method", clone.Method),
			slog.String("endpoint", diagnosticEndpoint(clone)),
			slog.Int("status", response.StatusCode),
			slog.String("request_id", response.Header.Get("X-Request-ID")),
			slog.String("client_version", clone.Header.Get("X-TimeKeeper-Client")),
			slog.String("agent_id", clone.Header.Get("X-Agent-ID")),
			slog.String("command", clone.Header.Get("X-TimeKeeper-Command")),
			slog.Int64("request_bytes", clone.ContentLength),
			slog.String("content_type", clone.Header.Get("Content-Type")),
			slog.Duration("duration", time.Since(start)),
		)
	}
	return response, err
}

func connectionErrorKind(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return "timeout"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if strings.Contains(strings.ToLower(opErr.Err.Error()), "refused") {
			return "connection_refused"
		}
		if strings.Contains(strings.ToLower(opErr.Err.Error()), "reset") {
			return "connection_reset"
		}
		return "network"
	}
	return "transport"
}

func diagnosticEndpoint(request *http.Request) string {
	if request == nil || request.URL == nil {
		return "unknown"
	}
	return request.URL.Scheme + "://" + request.URL.Host + request.URL.EscapedPath()
}

func dialTimeKeeperLocal(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err == nil && (strings.EqualFold(host, "timekeeper.local") || strings.EqualFold(host, "api.timekeeper.local") || strings.EqualFold(host, "guardian.local")) {
		// Keep the friendly hostname in the URL and Host header, but avoid
		// requiring permission to edit /etc/hosts in WSL. The service remains
		// loopback-only; this is a client-side alias, not network exposure.
		address = net.JoinHostPort("127.0.0.1", port)
	}
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

func newHTTPClient(timeout time.Duration) *http.Client {
	base := http.DefaultTransport
	if transport, ok := base.(*http.Transport); ok {
		transport = transport.Clone()
		transport.DialContext = dialTimeKeeperLocal
		base = transport
	}
	return &http.Client{Timeout: timeout, Transport: diagnosticTransport{base: base}}
}
