package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func runTaskBatch(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	var filePath, format string
	checkOnly := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--check-only":
			checkOnly = true
		case strings.HasPrefix(arg, "--format="):
			format = strings.TrimPrefix(arg, "--format=")
		case arg == "--format" && index+1 < len(args):
			index++
			format = args[index]
		case strings.HasPrefix(arg, "-") && arg != "-":
			_, _ = fmt.Fprintf(errOut, "unknown task batch option %q\nusage: tk t batch <project-id> <file> [--format format] [--check-only]\n", arg)
			return 2
		default:
			if filePath != "" {
				_, _ = fmt.Fprintln(errOut, "provide exactly one batch file")
				return 2
			}
			filePath = arg
		}
	}
	if strings.TrimSpace(filePath) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk t batch <project-id> <file> [--format format] [--check-only]")
		return 2
	}

	nested, legacyTasks, err := ParseExecutionBatch(filePath, format)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "parse batch: %s\n", boundedDiagnosticText(err.Error(), 2048))
		return 1
	}
	isNested := nested.Version != ""
	var body []byte
	var endpoint string
	if isNested {
		localValidation := model.ValidateExecutionBatchDocument(nested)
		if !localValidation.Valid {
			_, _ = fmt.Fprintf(errOut, "✗ Smoke test failed locally with %d issue(s):\n", len(localValidation.Issues))
			writeBatchIssues(errOut, localValidation.Issues)
			return 1
		}
		body, err = json.Marshal(nested)
		endpoint = "/api/v1/projects/" + projectID + "/execution-batches/validate"
	} else {
		localValidation := validateLegacyTasksLocally(legacyTasks)
		if !localValidation.Valid {
			_, _ = fmt.Fprintf(errOut, "✗ Smoke test failed locally with %d issue(s):\n", len(localValidation.Issues))
			writeBatchIssues(errOut, localValidation.Issues)
			return 1
		}
		body, err = json.Marshal(model.CreateTasksBatchInput{Tasks: legacyTasks})
		endpoint = "/api/v1/projects/" + projectID + "/tasks/batch/validate"
	}
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode batch smoke test: %v\n", err)
		return 1
	}
	if !checkOnly {
		return queueTaskBatchRegistration(projectID, body, endpoint, isNested, out, errOut, baseURL)
	}
	validation, err := postBatchValidation(baseURL, endpoint, body)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "smoke test failed: %s\n", boundedDiagnosticText(err.Error(), 2048))
		return 1
	}
	if !validation.Valid {
		_, _ = fmt.Fprintf(errOut, "✗ Smoke test failed with %d issue(s):\n", len(validation.Issues))
		writeBatchIssues(errOut, validation.Issues)
		if len(validation.Issues) == 0 {
			_, _ = fmt.Fprintln(errOut, "The server rejected the batch without returning repair details; correct the document or check the server logs.")
		}
		return 1
	}
	_, _ = fmt.Fprintf(out, "✓ Smoke test passed: Tasks=%d Subtasks=%d Sprints=%d Total=%d\n", validation.Counts.Tasks, validation.Counts.Subtasks, validation.Counts.Sprints, validation.Counts.TotalObjects)
	_, _ = fmt.Fprintln(out, "check-only requested; no objects were created.")
	return 0
}

func validateLegacyTasksLocally(tasks []model.CreateTaskInput) model.BatchValidationResult {
	result := model.BatchValidationResult{Valid: true, Counts: model.BatchCounts{Tasks: len(tasks), TotalObjects: len(tasks)}, Issues: []model.ValidationIssue{}}
	add := func(path, field, code, message, suggestion string) {
		result.Valid = false
		result.Issues = append(result.Issues, model.ValidationIssue{Path: path, Field: field, Code: code, Message: message, Suggestion: suggestion})
	}
	if len(tasks) == 0 {
		add("", "tasks", "empty_batch", "task batch contains no Tasks", "Add at least one Task before running the smoke test.")
	}
	if len(tasks) > 100 {
		add("", "tasks", "task_limit", "a batch may create at most 100 Tasks", "Split the file into batches of no more than 100 Tasks.")
	}
	for index, task := range tasks {
		path := fmt.Sprintf("tasks[%d]", index)
		if strings.TrimSpace(task.Name) == "" {
			add(path, "name", "required", "name is required", "Set name to a non-empty, human-readable value.")
		}
		if task.EstimatedDurationSeconds < 0 {
			add(path, "estimated_duration_seconds", "invalid_estimate", "estimated duration must not be negative", "Set estimated_duration_seconds to zero or a positive number of seconds.")
		}
	}
	return result
}

func postBatchValidation(baseURL, endpoint string, body []byte) (model.BatchValidationResult, error) {
	response, err := postBatchRequest(baseURL, endpoint, body)
	if err != nil {
		return model.BatchValidationResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return model.BatchValidationResult{}, fmt.Errorf("%s", readHTTPErrorDetail(response))
	}
	var result model.BatchValidationResult
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxAPIErrorBody))
	if err := decoder.Decode(&result); err != nil {
		return model.BatchValidationResult{}, fmt.Errorf("API returned invalid smoke-test JSON: %w", err)
	}
	return result, nil
}

func postBatchRequest(baseURL, endpoint string, body []byte) (*http.Response, error) {
	url := strings.TrimRight(baseURL, "/") + endpoint
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	return response, nil
}

func writeBatchIssues(out io.Writer, issues []model.ValidationIssue) {
	for _, issue := range issues {
		path := boundedDiagnosticText(issue.Path, 256)
		field := boundedDiagnosticText(issue.Field, 256)
		if path != "" && field != "" {
			path += "."
		}
		_, _ = fmt.Fprintf(out, "- %s%s [%s] %s\n  Fix: %s\n", path, field, boundedDiagnosticText(issue.Code, 128), boundedDiagnosticText(issue.Message, 1024), boundedDiagnosticText(issue.Suggestion, 1024))
	}
}

func writeCreatedBatchRows(out io.Writer, tasks []model.Task, subtasks []model.Subtask, sprints []model.Sprint) {
	for _, task := range tasks {
		_, _ = fmt.Fprintf(out, "%s\tTask\t%s\t%s\n", task.TaskID, task.Name, task.ItemAddress)
	}
	for _, subtask := range subtasks {
		_, _ = fmt.Fprintf(out, "%s\tSubtask\t%s\t%s\n", subtask.SubtaskID, subtask.Name, subtask.ItemAddress)
	}
	for _, sprint := range sprints {
		_, _ = fmt.Fprintf(out, "%s\tSprint\t%s\t%s\n", sprint.SprintID, sprint.Name, sprint.ItemAddress)
	}
}
