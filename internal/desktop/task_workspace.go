package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// TaskChildrenSummary is Core's completion-blocker projection for a Task.
type TaskChildrenSummary struct {
	SubtaskIDs   []string `json:"subtask_ids"`
	SprintIDs    []string `json:"sprint_ids"`
	OpenSubtasks int      `json:"open_subtasks"`
	OpenSprints  int      `json:"open_sprints"`
	CanComplete  bool     `json:"can_complete"`
}

// TaskDetail is the existing Core task-detail response exposed through Wails.
type TaskDetail struct {
	Task     model.Task          `json:"task"`
	Subtasks []model.Subtask     `json:"subtasks"`
	Sprints  []model.Sprint      `json:"sprints"`
	Children TaskChildrenSummary `json:"children"`
}

// TaskDetail reads one Task and its direct children from the existing Core API.
func (c *CoreClient) TaskDetail(ctx context.Context, taskID string) (TaskDetail, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return TaskDetail{}, fmt.Errorf("Task ID is required")
	}
	var result TaskDetail
	route := "/api/v1/tasks/" + url.PathEscape(taskID)
	if err := c.get(ctx, route, &result); err != nil {
		return TaskDetail{}, err
	}
	if result.Task.TaskID != taskID || result.Subtasks == nil || result.Sprints == nil || result.Children.SubtaskIDs == nil || result.Children.SprintIDs == nil {
		return TaskDetail{}, fmt.Errorf("Core returned an incomplete Task detail response")
	}
	return result, nil
}

// UpdateTaskStatus requests one Core-supported explicit Task status.
func (c *CoreClient) UpdateTaskStatus(ctx context.Context, taskID, status string) (model.Task, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return model.Task{}, fmt.Errorf("Task ID is required")
	}
	switch status {
	case "Open", "On Hold", "Completed", "Cancelled":
	default:
		return model.Task{}, fmt.Errorf("unsupported Task status")
	}
	var result model.Task
	route := "/api/v1/tasks/" + url.PathEscape(taskID) + "/status"
	if err := c.post(ctx, route, map[string]string{"status": status}, &result); err != nil {
		return model.Task{}, err
	}
	return result, nil
}

// TransitionSprint delegates a supported Sprint lifecycle action to Core.
// Core remains authoritative for transition legality, claim conflicts, and reasons.
func (c *CoreClient) TransitionSprint(ctx context.Context, sprintID, action, reason string) (model.Sprint, error) {
	sprintID = strings.TrimSpace(sprintID)
	if sprintID == "" {
		return model.Sprint{}, fmt.Errorf("Sprint ID is required")
	}
	switch action {
	case "start", "hold", "resume", "complete", "cancel":
	default:
		return model.Sprint{}, fmt.Errorf("unsupported Sprint action")
	}
	var result model.Sprint
	route := "/api/v1/sprints/" + url.PathEscape(sprintID) + "/" + action
	if err := c.post(ctx, route, map[string]string{"reason": reason}, &result); err != nil {
		return model.Sprint{}, err
	}
	return result, nil
}

func (c *CoreClient) post(ctx context.Context, route string, input, target any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode Core request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+route, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Core request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCoreUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Core POST %s returned HTTP %d", route, resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("Core POST %s returned malformed JSON", route)
	}
	return nil
}
