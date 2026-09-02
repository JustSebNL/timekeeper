// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	_, _ = fmt.Fprintln(errOut, "usage: tk usage <project-id> | tk usage record <project-id> <session-id> <agent-id> <model> <turn> <input> <output> [cache-write] [cache-read] [messages] [sprint-id]")
	return 2
}

func usageSummary(projectID string, out, errOut io.Writer, baseURL string) int {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + url.PathEscape(projectID) + "/usage-summary"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(endpoint)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
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
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
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

func parseNonNegativeUsageInt(value string, errOut io.Writer) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		_, _ = fmt.Fprintf(errOut, "usage value %q must be a non-negative integer\n", value)
		return -1, fmt.Errorf("invalid usage value")
	}
	return parsed, nil
}
