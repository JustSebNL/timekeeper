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

type taskDetailResponse struct {
	Task     model.Task      `json:"task"`
	Subtasks []model.Subtask `json:"subtasks"`
	Sprints  []model.Sprint  `json:"sprints"`
	Gems     []model.ProjectGem `json:"gems,omitempty"`
	Children *struct {
		SubtaskIDs   []string `json:"subtask_ids"`
		SprintIDs    []string `json:"sprint_ids"`
		OpenSubtasks int      `json:"open_subtasks"`
		OpenSprints  int      `json:"open_sprints"`
		CanComplete  bool     `json:"can_complete"`
	} `json:"children,omitempty"`
}

func taskShow(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, _ = fmt.Fprintln(out, "usage: tk t show <task-id> [--json]")
		return 0
	}
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk t show <task-id> [--json]")
		return 2
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + url.PathEscape(args[0])
	response, err := newHTTPClient(10 * time.Second).Get(endpoint)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %s\n", endpoint, boundedDiagnosticText(err.Error(), 2048))
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "read task detail", response)
		return 1
	}
	var detail taskDetailResponse
	if err := json.NewDecoder(response.Body).Decode(&detail); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid Task detail JSON: %s\n", boundedDiagnosticText(err.Error(), 1024))
		return 1
	}
	if jsonOutputEnabled {
		writeJSONResponse(out, detail)
		return 0
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", detail.Task.TaskID, detail.Task.Status, detail.Task.Name)
	_, _ = fmt.Fprintf(out, "Project: %s\tCategory: %s\tAddress: %s\n", detail.Task.ProjectID, detail.Task.CategoryID, detail.Task.ItemAddress)
	if detail.Task.Goal != "" {
		_, _ = fmt.Fprintf(out, "Goal: %s\n", detail.Task.Goal)
	}
	if detail.Task.Description != "" {
		_, _ = fmt.Fprintf(out, "Description: %s\n", detail.Task.Description)
	}
	_, _ = fmt.Fprintf(out, "Priority: %s\tEstimate: %ds\tReported: %.1f%%\tCalculated: %.1f%%\n", detail.Task.Priority, detail.Task.EstimatedDurationSeconds, detail.Task.ReportedCompletionPct, detail.Task.CalculatedCompletionPct)
	_, _ = fmt.Fprintf(out, "Created: %s\tUpdated: %s\n", detail.Task.CreatedAt.Format(time.RFC3339), detail.Task.UpdatedAt.Format(time.RFC3339))
	if detail.Children != nil {
		if detail.Children.CanComplete {
			_, _ = fmt.Fprintln(out, "Completion: can complete")
		} else {
			_, _ = fmt.Fprintf(out, "Completion: cannot complete (%d open subtasks, %d open sprints)\n", detail.Children.OpenSubtasks, detail.Children.OpenSprints)
		}
	}
	for _, subtask := range detail.Subtasks {
		_, _ = fmt.Fprintf(out, "Subtask\t%s\t%s\t%s\n", subtask.SubtaskID, subtask.Status, subtask.Name)
	}
	for _, sprint := range detail.Sprints {
			_, _ = fmt.Fprintf(out, "Sprint	%s	%s	%s	%d s\n", sprint.SprintID, sprint.Status, sprint.Name, sprint.EstimatedDurationSeconds)
		}
		if len(detail.Gems) > 0 {
			_, _ = fmt.Fprintln(out, "Treasure Chest gems:")
			for _, g := range detail.Gems {
				_, _ = fmt.Fprintf(out, "  %s	[%s]	%s\n", g.Ref, g.Category, g.Name)
			}
		}
		return 0
}
