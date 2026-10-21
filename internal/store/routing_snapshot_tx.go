package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/routing"
)

// validateRoutingEvidenceTx rechecks the routing snapshot inside the same
// transaction that registers an execution batch. A queue item that waited
// after a transient failure cannot apply against a newer hierarchy.
func validateRoutingEvidenceTx(ctx context.Context, tx *sql.Tx, projectID string, evidence *model.ExecutionBatchRoutingEvidence) (*model.ValidationIssue, error) {
	if evidence == nil {
		return nil, nil
	}
	if !evidence.Confirmed || evidence.DecisionID == "" || evidence.PolicyVersion != routing.PolicyVersion || evidence.CandidateStateHash == "" || evidence.IntentHash == "" || len(evidence.Decision) == 0 || !json.Valid(evidence.Decision) {
		return &model.ValidationIssue{
			Path:       "planning.routing_evidence",
			Field:      "routing_evidence",
			Code:       "invalid_routing_evidence",
			Message:    "routing provenance is incomplete or unconfirmed",
			Suggestion: "Use Core routing Apply with a confirmed decision, current candidate hash, and idempotency key.",
		}, nil
	}
	tree, err := projectExecutionTreeTx(ctx, tx, projectID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &model.ValidationIssue{
				Path:       "planning.routing_evidence",
				Field:      "candidate_state_hash",
				Code:       "project_not_found",
				Message:    "routing Project no longer exists",
				Suggestion: "Refresh the Project snapshot and produce a new routing decision.",
			}, nil
		}
		return nil, fmt.Errorf("read routing snapshot in registration transaction: %w", err)
	}
	set := routing.CandidateSetFromTree(tree, routing.DefaultLimits())
	if set.Truncated || set.StateHash != evidence.CandidateStateHash {
		return &model.ValidationIssue{
			Path:       "planning.routing_evidence",
			Field:      "candidate_state_hash",
			Code:       "routing_candidate_state_changed",
			Message:    "candidate state changed before the queued routing Apply could commit",
			Suggestion: "Refresh the Core candidate snapshot and obtain a new confirmed routing decision.",
		}, nil
	}
	return nil, nil
}

// projectExecutionTreeTx builds the same ownership projection as
// Store.ProjectExecutionTree, but reads every row through the caller's
// transaction so routing hash validation and the eventual batch writes share
// one SQLite snapshot.
func projectExecutionTreeTx(ctx context.Context, tx *sql.Tx, projectID string) (model.ExecutionTree, error) {
	project, err := scanProject(tx.QueryRowContext(ctx, `SELECT project_id, project_number, item_address, project_name,
		project_description, project_goal, status, priority, palette_id,
		reported_completion_pct, calculated_completion_pct, created_at, updated_at, project_alias,
		planned_start_date, target_end_date, actual_started_at, completed_at, cancelled_at,
		lifecycle_phase, schedule_timezone, deadline_warning_days
		FROM projects WHERE project_id = ?`, projectID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ExecutionTree{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.ExecutionTree{}, fmt.Errorf("read routing snapshot Project: %w", err)
	}

	categories, err := readCategoriesTx(ctx, tx, projectID)
	if err != nil {
		return model.ExecutionTree{}, err
	}
	tasks, err := readTasksTx(ctx, tx, projectID)
	if err != nil {
		return model.ExecutionTree{}, err
	}
	subtasks, err := readSubtasksTx(ctx, tx, projectID)
	if err != nil {
		return model.ExecutionTree{}, err
	}
	sprints, err := readSprintsTx(ctx, tx, projectID)
	if err != nil {
		return model.ExecutionTree{}, err
	}

	categoryNodes := make(map[string]*model.ExecutionCategory, len(categories))
	for _, category := range categories {
		categoryNodes[category.CategoryID] = &model.ExecutionCategory{
			Category:   category,
			Categories: make([]model.ExecutionCategory, 0),
			Tasks:      make([]model.ExecutionTask, 0),
		}
	}
	taskNodes := make(map[string]*model.ExecutionTask, len(tasks))
	for _, task := range tasks {
		taskNodes[task.TaskID] = &model.ExecutionTask{
			Task:     task,
			Sprints:  make([]model.Sprint, 0),
			Subtasks: make([]model.ExecutionSubtask, 0),
		}
	}
	type subtaskLocation struct {
		taskID string
		index  int
	}
	subtaskLocations := make(map[string]subtaskLocation, len(subtasks))
	for _, subtask := range subtasks {
		taskNode, ok := taskNodes[subtask.TaskID]
		if !ok {
			return model.ExecutionTree{}, fmt.Errorf("routing snapshot Subtask %s references a missing Task", subtask.SubtaskID)
		}
		index := len(taskNode.Subtasks)
		taskNode.Subtasks = append(taskNode.Subtasks, model.ExecutionSubtask{Subtask: subtask, Sprints: make([]model.Sprint, 0)})
		subtaskLocations[subtask.SubtaskID] = subtaskLocation{taskID: subtask.TaskID, index: index}
	}
	for _, sprint := range sprints {
		if sprint.SubtaskID != "" {
			location, ok := subtaskLocations[sprint.SubtaskID]
			if !ok || location.taskID != sprint.TaskID {
				return model.ExecutionTree{}, fmt.Errorf("routing snapshot Sprint %s references a missing Subtask owner", sprint.SprintID)
			}
			taskNode := taskNodes[location.taskID]
			taskNode.Subtasks[location.index].Sprints = append(taskNode.Subtasks[location.index].Sprints, sprint)
			continue
		}
		taskNode, ok := taskNodes[sprint.TaskID]
		if !ok {
			return model.ExecutionTree{}, fmt.Errorf("routing snapshot Sprint %s references a missing Task owner", sprint.SprintID)
		}
		taskNode.Sprints = append(taskNode.Sprints, sprint)
	}
	for _, task := range tasks {
		categoryNode, ok := categoryNodes[task.CategoryID]
		if !ok {
			return model.ExecutionTree{}, fmt.Errorf("routing snapshot Task %s references a missing Category", task.TaskID)
		}
		categoryNode.Tasks = append(categoryNode.Tasks, *taskNodes[task.TaskID])
	}

	tree := model.ExecutionTree{Project: project, Categories: make([]model.ExecutionCategory, 0)}
	var buildCategory func(string) model.ExecutionCategory
	buildCategory = func(categoryID string) model.ExecutionCategory {
		node := *categoryNodes[categoryID]
		node.Categories = make([]model.ExecutionCategory, 0)
		for _, child := range categories {
			if child.ParentCategoryID == categoryID {
				node.Categories = append(node.Categories, buildCategory(child.CategoryID))
			}
		}
		return node
	}
	for _, category := range categories {
		if category.ParentCategoryID == "" {
			tree.Categories = append(tree.Categories, buildCategory(category.CategoryID))
		} else if _, ok := categoryNodes[category.ParentCategoryID]; !ok {
			return model.ExecutionTree{}, fmt.Errorf("routing snapshot Category %s references a missing parent", category.CategoryID)
		}
	}
	model.CalculateExecutionTreeCompletion(&tree)
	return tree, nil
}

func readCategoriesTx(ctx context.Context, tx *sql.Tx, projectID string) ([]model.Category, error) {
	rows, err := tx.QueryContext(ctx, `SELECT category_id, project_id, parent_category_id, item_address, category_name,
		category_description, category_goal, status, priority, progress_pct, created_at, updated_at
		FROM categories WHERE project_id = ? ORDER BY created_at, category_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("read routing snapshot Categories: %w", err)
	}
	defer rows.Close()
	items := make([]model.Category, 0)
	for rows.Next() {
		item, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func readTasksTx(ctx context.Context, tx *sql.Tx, projectID string) ([]model.Task, error) {
	rows, err := tx.QueryContext(ctx, `SELECT task_id, project_id, category_id, item_address, task_name,
		task_description, task_goal, status, priority, estimated_duration_seconds,
		reported_completion_pct, calculated_completion_pct, auto_close, created_at, updated_at
		FROM tasks WHERE project_id = ? ORDER BY created_at, task_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("read routing snapshot Tasks: %w", err)
	}
	defer rows.Close()
	items := make([]model.Task, 0)
	for rows.Next() {
		item, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func readSubtasksTx(ctx context.Context, tx *sql.Tx, projectID string) ([]model.Subtask, error) {
	rows, err := tx.QueryContext(ctx, `SELECT subtask_id, project_id, category_id, task_id, item_address,
		subtask_name, subtask_description, subtask_goal, status, priority, estimated_duration_seconds,
		reported_completion_pct, calculated_completion_pct, created_at, updated_at
		FROM subtasks WHERE project_id = ? ORDER BY created_at, subtask_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("read routing snapshot Subtasks: %w", err)
	}
	defer rows.Close()
	items := make([]model.Subtask, 0)
	for rows.Next() {
		item, err := scanSubtask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func readSprintsTx(ctx context.Context, tx *sql.Tx, projectID string) ([]model.Sprint, error) {
	rows, err := tx.QueryContext(ctx, `SELECT sprint_id, project_id, category_id, task_id, subtask_id, item_address,
		sprint_name, sprint_description, sprint_goal, status, priority, estimated_duration_seconds,
		buffer_pct, buffer_duration_seconds, active_duration_seconds, hold_duration_seconds, hold_reason,
		started_at, ended_at, created_at, updated_at
		FROM sprints WHERE project_id = ? ORDER BY created_at, sprint_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("read routing snapshot Sprints: %w", err)
	}
	defer rows.Close()
	items := make([]model.Sprint, 0)
	for rows.Next() {
		item, err := scanSprint(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
