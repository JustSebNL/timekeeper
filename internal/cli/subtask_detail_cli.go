package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

type subtaskDetailResponse struct {
	Subtask model.Subtask  `json:"subtask"`
	Sprints []model.Sprint `json:"sprints"`
}

func subtaskShow(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, _ = fmt.Fprintln(out, "usage: tk st show <subtask-id> [--json]")
		return 0
	}
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk st show <subtask-id> [--json]")
		return 2
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/subtasks/" + url.PathEscape(args[0])
	response, err := newHTTPClient(10 * time.Second).Get(endpoint)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %s\n", endpoint, boundedDiagnosticText(err.Error(), 2048))
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "read subtask detail", response)
		return 1
	}
	var detail subtaskDetailResponse
	if err := json.NewDecoder(response.Body).Decode(&detail); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid Subtask detail JSON: %s\n", boundedDiagnosticText(err.Error(), 1024))
		return 1
	}
	if jsonOutputEnabled {
		writeJSONResponse(out, detail)
		return 0
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", detail.Subtask.SubtaskID, detail.Subtask.Status, detail.Subtask.Name)
	_, _ = fmt.Fprintf(out, "Project: %s\tCategory: %s\tTask: %s\tAddress: %s\n", detail.Subtask.ProjectID, detail.Subtask.CategoryID, detail.Subtask.TaskID, detail.Subtask.ItemAddress)
	if detail.Subtask.Goal != "" {
		_, _ = fmt.Fprintf(out, "Goal: %s\n", detail.Subtask.Goal)
	}
	if detail.Subtask.Description != "" {
		_, _ = fmt.Fprintf(out, "Description: %s\n", detail.Subtask.Description)
	}
	_, _ = fmt.Fprintf(out, "Priority: %s\tEstimate: %ds\tReported: %.1f%%\tCalculated: %.1f%%\n", detail.Subtask.Priority, detail.Subtask.EstimatedDurationSeconds, detail.Subtask.ReportedCompletionPct, detail.Subtask.CalculatedCompletionPct)
	_, _ = fmt.Fprintf(out, "Created: %s\tUpdated: %s\n", detail.Subtask.CreatedAt.Format(time.RFC3339), detail.Subtask.UpdatedAt.Format(time.RFC3339))
	for _, sprint := range detail.Sprints {
		_, _ = fmt.Fprintf(out, "Sprint\t%s\t%s\t%s\t%d s\n", sprint.SprintID, sprint.Status, sprint.Name, sprint.EstimatedDurationSeconds)
	}
	return 0
}
