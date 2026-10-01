// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func usage(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 1 {
		return usageSummary(args[0], out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "record" {
		return usageRecord(args[1:], out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "ingest" {
		return usageIngest(args[1:], out, errOut, baseURL)
	}
	_, _ = fmt.Fprintln(errOut, "usage: tk usage <project-id> | tk usage record <project-id> <session-id> <agent-id> <model> <turn> <input> <output> [cache-write] [cache-read] [messages] [sprint-id] | tk usage ingest <project-id> <envelope.json>")
	return 2
}

func usageSummary(projectID string, out, errOut io.Writer, baseURL string) int {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + url.PathEscape(projectID) + "/usage-summary"
	response, err := newHTTPClient(10 * time.Second).Get(endpoint)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "read usage summary", response)
		return 1
	}
	var summary model.ProjectUsageSummary
	if err := json.NewDecoder(response.Body).Decode(&summary); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid usage JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\tsessions=%d\tinput=%d\toutput=%d\tcache-write=%d\tcache-read=%d\tmessages=%d\n",
		summary.ProjectID, summary.Totals.SessionCount, summary.Totals.InputTokens, summary.Totals.OutputTokens,
		summary.Totals.CacheCreationTokens, summary.Totals.CacheReadTokens, summary.Totals.Messages)
	for _, session := range summary.Sessions {
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\tturn=%d\tinput=%d\toutput=%d\tcache-write=%d\tcache-read=%d\tmessages=%d\n",
			session.SessionID, session.AgentID, session.Model, session.TurnSeq, session.InputTokens, session.OutputTokens,
			session.CacheCreationTokens, session.CacheReadTokens, session.Messages)
	}
	return 0
}

func usageRecord(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 7 || len(args) > 11 {
		_, _ = fmt.Fprintln(errOut, "usage: tk usage record <project-id> <session-id> <agent-id> <model> <turn> <input> <output> [cache-write] [cache-read] [messages] [sprint-id]")
		return 2
	}
	values := make([]int64, 5)
	for index, position := range []int{4, 5, 6} {
		parsed, err := parseNonNegativeUsageInt(args[position], errOut)
		if err != nil {
			return 2
		}
		values[index] = parsed
	}
	input := model.UsageSnapshotInput{AgentID: args[2], Model: args[3], TurnSeq: values[0], InputTokens: values[1], OutputTokens: values[2]}
	if len(args) >= 8 {
		parsed, err := parseNonNegativeUsageInt(args[7], errOut)
		if err != nil {
			return 2
		}
		input.CacheCreationTokens = parsed
	}
	if len(args) >= 9 {
		parsed, err := parseNonNegativeUsageInt(args[8], errOut)
		if err != nil {
			return 2
		}
		input.CacheReadTokens = parsed
	}
	if len(args) >= 10 {
		parsed, err := parseNonNegativeUsageInt(args[9], errOut)
		if err != nil {
			return 2
		}
		input.Messages = parsed
	}
	if len(args) == 11 {
		input.SprintID = args[10]
	}
	body, err := json.Marshal(input)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode usage snapshot: %v\n", err)
		return 1
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + url.PathEscape(args[0]) + "/usage-sessions/" + url.PathEscape(args[1]) + "/snapshots"
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create usage request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "record usage", response)
		return 1
	}
	var result model.UsageSnapshotResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid usage JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\tduplicate=%t\tinput=%d\toutput=%d\tcache-write=%d\tcache-read=%d\tmessages=%d\n",
		result.Session.SessionID, result.Duplicate, result.Delta.InputTokens, result.Delta.OutputTokens,
		result.Delta.CacheCreationTokens, result.Delta.CacheReadTokens, result.Delta.Messages)
	return 0
}

func usageIngest(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 {
		_, _ = fmt.Fprintln(errOut, "usage: tk usage ingest <project-id> <envelope.json>")
		return 2
	}
	body, err := os.ReadFile(args[1])
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "read usage envelope: %v\n", err)
		return 1
	}
	if len(body) > 64<<10 {
		_, _ = fmt.Fprintln(errOut, "usage envelope must be at most 64 KiB")
		return 2
	}
	if err := rejectUsageSensitiveKeys(body); err != nil {
		_, _ = fmt.Fprintf(errOut, "usage envelope rejected: %v\n", err)
		return 2
	}
	var envelope struct {
		RedactionStatus string                   `json:"redaction_status"`
		Source          string                   `json:"source"`
		Coverage        string                   `json:"coverage"`
		Snapshot        model.UsageSnapshotInput `json:"snapshot"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		_, _ = fmt.Fprintf(errOut, "decode usage envelope: %v\n", err)
		return 2
	}
	if envelope.RedactionStatus != "verified" {
		_, _ = fmt.Fprintln(errOut, "usage envelope requires redaction_status=verified")
		return 2
	}
	if strings.TrimSpace(envelope.Source) == "" || strings.TrimSpace(envelope.Coverage) == "" {
		_, _ = fmt.Fprintln(errOut, "usage envelope requires source and coverage")
		return 2
	}
	if strings.TrimSpace(envelope.Snapshot.SessionID) == "" {
		_, _ = fmt.Fprintln(errOut, "usage envelope snapshot requires session_id")
		return 2
	}
	envelope.Snapshot.Source = envelope.Source
	envelope.Snapshot.Coverage = envelope.Coverage
	payload, err := json.Marshal(envelope.Snapshot)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode usage snapshot: %v\n", err)
		return 1
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + url.PathEscape(args[0]) + "/usage-sessions/" + url.PathEscape(envelope.Snapshot.SessionID) + "/snapshots"
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create usage request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "ingest usage snapshot", response)
		return 1
	}
	var result model.UsageSnapshotResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid usage JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\tsource=%s\tcoverage=%s\tduplicate=%t\tinput=%d\toutput=%d\n",
		result.Session.SessionID, result.Session.Source, result.Session.Coverage, result.Duplicate,
		result.Delta.InputTokens, result.Delta.OutputTokens)
	return 0
}

func rejectUsageSensitiveKeys(body []byte) error {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var walk func(any) error
	walk = func(current any) error {
		switch value := current.(type) {
		case map[string]any:
			for key, child := range value {
				switch strings.ToLower(strings.ReplaceAll(key, "-", "_")) {
				case "prompt", "prompt_body", "transcript", "tool_output", "password", "secret", "api_key", "bearer", "cookie", "credential":
					return fmt.Errorf("sensitive field %q is not accepted; send counters only", key)
				}
				if err := walk(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range value {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(value)
}

func parseNonNegativeUsageInt(value string, errOut io.Writer) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		_, _ = fmt.Fprintf(errOut, "usage value %q must be a non-negative integer\n", value)
		return -1, fmt.Errorf("invalid usage value")
	}
	return parsed, nil
}
