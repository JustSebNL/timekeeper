package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// CreateExecutionBatch revalidates persisted ownership/state and writes the
// complete executable hierarchy in one SQLite transaction. A non-nil error is
// reserved for storage failures; user-correctable findings are returned in
// validation with an empty result and no writes.
func (s *Store) CreateExecutionBatch(ctx context.Context, projectID string, input model.ExecutionBatchInput) (model.ExecutionBatchResult, model.BatchValidationResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ExecutionBatchResult{}, model.BatchValidationResult{}, fmt.Errorf("begin execution batch creation: %w", err)
	}
	defer tx.Rollback()
	result, validation, err := createExecutionBatchTx(ctx, tx, projectID, input)
	if err != nil || !validation.Valid {
		return result, validation, err
	}
	if err := tx.Commit(); err != nil {
		return model.ExecutionBatchResult{}, validation, fmt.Errorf("commit execution batch: %w", err)
	}
	return result, validation, nil
}

func createExecutionBatchTx(ctx context.Context, tx *sql.Tx, projectID string, input model.ExecutionBatchInput) (model.ExecutionBatchResult, model.BatchValidationResult, error) {
	validation, err := validateExecutionBatchTx(ctx, tx, projectID, input)
	if err != nil {
		return model.ExecutionBatchResult{}, model.BatchValidationResult{}, fmt.Errorf("revalidate execution batch: %w", err)
	}
	if !validation.Valid {
		return model.ExecutionBatchResult{}, validation, nil
	}
	result := model.ExecutionBatchResult{Counts: validation.Counts}
	result.Tasks = make([]model.Task, 0, validation.Counts.Tasks)
	result.Subtasks = make([]model.Subtask, 0, validation.Counts.Subtasks)
	result.Sprints = make([]model.Sprint, 0, validation.Counts.Sprints)
	now := time.Now().UTC().Round(0)

	for _, taskInput := range input.Tasks {
		categoryID, categoryAddress, err := resolveCategoryID(ctx, tx, projectID, taskInput.CategoryID)
		if err != nil {
			return model.ExecutionBatchResult{}, validation, fmt.Errorf("resolve batch Task Category: %w", err)
		}
		task, err := insertExecutionBatchTask(ctx, tx, projectID, categoryID, categoryAddress, taskInput, now)
		if err != nil {
			return model.ExecutionBatchResult{}, validation, err
		}
		result.Tasks = append(result.Tasks, task)
		for _, sprintInput := range taskInput.Sprints {
			sprint, err := insertExecutionBatchSprint(ctx, tx, task.ProjectID, task.CategoryID, task.TaskID, "", task.ItemAddress, sprintInput, now)
			if err != nil {
				return model.ExecutionBatchResult{}, validation, err
			}
			result.Sprints = append(result.Sprints, sprint)
		}
		for _, subtaskInput := range taskInput.Subtasks {
			subtask, err := insertExecutionBatchSubtask(ctx, tx, task, subtaskInput, now)
			if err != nil {
				return model.ExecutionBatchResult{}, validation, err
			}
			result.Subtasks = append(result.Subtasks, subtask)
			for _, sprintInput := range subtaskInput.Sprints {
				sprint, err := insertExecutionBatchSprint(ctx, tx, subtask.ProjectID, subtask.CategoryID, subtask.TaskID, subtask.SubtaskID, subtask.ItemAddress, sprintInput, now)
				if err != nil {
					return model.ExecutionBatchResult{}, validation, err
				}
				result.Sprints = append(result.Sprints, sprint)
			}
		}
	}
	for _, extension := range input.Extensions {
		var task model.Task
		var subtask model.Subtask
		var project, category, address, taskID string
		if extension.TaskID != "" {
			task, err = scanTask(tx.QueryRowContext(ctx, `SELECT task_id, project_id, category_id, item_address, task_name, task_description, task_goal, status, priority, estimated_duration_seconds, reported_completion_pct, calculated_completion_pct, auto_close, created_at, updated_at FROM tasks WHERE task_id = ?`, extension.TaskID))
			if err != nil {
				return model.ExecutionBatchResult{}, validation, fmt.Errorf("read batch Task extension owner: %w", err)
			}
			project, category, address, taskID = task.ProjectID, task.CategoryID, task.ItemAddress, task.TaskID
		} else {
			subtask, err = scanSubtask(tx.QueryRowContext(ctx, `SELECT subtask_id, project_id, category_id, task_id, item_address, subtask_name, subtask_description, subtask_goal, status, priority, estimated_duration_seconds, reported_completion_pct, calculated_completion_pct, created_at, updated_at FROM subtasks WHERE subtask_id = ?`, extension.SubtaskID))
			if err != nil {
				return model.ExecutionBatchResult{}, validation, fmt.Errorf("read batch Subtask extension owner: %w", err)
			}
			project, category, address, taskID = subtask.ProjectID, subtask.CategoryID, subtask.ItemAddress, subtask.TaskID
		}
		for _, sprintInput := range extension.Sprints {
			ownerTaskID, ownerSubtaskID := taskID, ""
			if extension.SubtaskID != "" {
				ownerTaskID, ownerSubtaskID = subtask.TaskID, subtask.SubtaskID
			}
			sprint, err := insertExecutionBatchSprint(ctx, tx, project, category, ownerTaskID, ownerSubtaskID, address, sprintInput, now)
			if err != nil {
				return model.ExecutionBatchResult{}, validation, err
			}
			result.Sprints = append(result.Sprints, sprint)
		}
		for _, subtaskInput := range extension.Subtasks {
			if extension.TaskID == "" {
				continue // Structural validation rejects this unsupported level.
			}
			subtask, err := insertExecutionBatchSubtask(ctx, tx, task, subtaskInput, now)
			if err != nil {
				return model.ExecutionBatchResult{}, validation, err
			}
			result.Subtasks = append(result.Subtasks, subtask)
			for _, sprintInput := range subtaskInput.Sprints {
				sprint, err := insertExecutionBatchSprint(ctx, tx, subtask.ProjectID, subtask.CategoryID, subtask.TaskID, subtask.SubtaskID, subtask.ItemAddress, sprintInput, now)
				if err != nil {
					return model.ExecutionBatchResult{}, validation, err
				}
				result.Sprints = append(result.Sprints, sprint)
			}
		}
	}
	return result, validation, nil
}

func insertExecutionBatchTask(ctx context.Context, tx *sql.Tx, projectID, categoryID, categoryAddress string, input model.ExecutionBatchTask, now time.Time) (model.Task, error) {
	input.Name = strings.TrimSpace(input.Name)
	priority := strings.TrimSpace(input.Priority)
	if priority == "" {
		priority = "Normal"
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.Task{}, err
	}
	autoClose := true
	if input.AutoClose != nil {
		autoClose = *input.AutoClose
	}
	task := model.Task{TaskID: fmt.Sprintf("T-%d", number), ProjectID: projectID, CategoryID: categoryID, ItemAddress: categoryAddress + "." + fmt.Sprint(number), Name: input.Name, Description: strings.TrimSpace(input.Description), Goal: strings.TrimSpace(input.Goal), Status: "Open", Priority: priority, EstimatedDurationSeconds: input.EstimatedDurationSeconds, AutoClose: autoClose, CreatedAt: now, UpdatedAt: now}
	autoCloseInt := 0
	if autoClose {
		autoCloseInt = 1
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO tasks (task_id, project_id, category_id, item_address, task_name, task_description, task_goal, status, priority, estimated_duration_seconds, reported_completion_pct, calculated_completion_pct, auto_close, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, task.TaskID, task.ProjectID, task.CategoryID, task.ItemAddress, task.Name, task.Description, task.Goal, task.Status, task.Priority, task.EstimatedDurationSeconds, task.ReportedCompletionPct, task.CalculatedCompletionPct, autoCloseInt, stamp(now), stamp(now))
	if err != nil {
		return model.Task{}, fmt.Errorf("insert batch Task: %w", err)
	}
	return task, nil
}

func insertExecutionBatchSubtask(ctx context.Context, tx *sql.Tx, task model.Task, input model.ExecutionBatchSubtask, now time.Time) (model.Subtask, error) {
	priority := strings.TrimSpace(input.Priority)
	if priority == "" {
		priority = "Normal"
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.Subtask{}, err
	}
	subtask := model.Subtask{SubtaskID: fmt.Sprintf("ST-%d", number), ProjectID: task.ProjectID, CategoryID: task.CategoryID, TaskID: task.TaskID, ItemAddress: task.ItemAddress + "." + fmt.Sprint(number), Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), Goal: strings.TrimSpace(input.Goal), Status: "Open", Priority: priority, EstimatedDurationSeconds: input.EstimatedDurationSeconds, CreatedAt: now, UpdatedAt: now}
	_, err = tx.ExecContext(ctx, `INSERT INTO subtasks (subtask_id, project_id, category_id, task_id, item_address, subtask_name, subtask_description, subtask_goal, status, priority, estimated_duration_seconds, reported_completion_pct, calculated_completion_pct, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, subtask.SubtaskID, subtask.ProjectID, subtask.CategoryID, subtask.TaskID, subtask.ItemAddress, subtask.Name, subtask.Description, subtask.Goal, subtask.Status, subtask.Priority, subtask.EstimatedDurationSeconds, subtask.ReportedCompletionPct, subtask.CalculatedCompletionPct, stamp(now), stamp(now))
	if err != nil {
		return model.Subtask{}, fmt.Errorf("insert batch Subtask: %w", err)
	}
	return subtask, nil
}

func insertExecutionBatchSprint(ctx context.Context, tx *sql.Tx, projectID, categoryID, taskID, subtaskID, ownerAddress string, input model.CreateSprintInput, now time.Time) (model.Sprint, error) {
	priority := strings.TrimSpace(input.Priority)
	if priority == "" {
		priority = "Normal"
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.Sprint{}, err
	}
	sprint := model.Sprint{SprintID: fmt.Sprintf("SP-%d", number), ProjectID: projectID, CategoryID: categoryID, TaskID: taskID, SubtaskID: subtaskID, ItemAddress: ownerAddress + "." + fmt.Sprint(number), Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), Goal: strings.TrimSpace(input.Goal), Status: "Open", Priority: priority, EstimatedDurationSeconds: input.EstimatedDurationSeconds, BufferPct: input.BufferPct, BufferDurationSeconds: int64(float64(input.EstimatedDurationSeconds) * input.BufferPct / 100), CreatedAt: now, UpdatedAt: now}
	var storedSubtaskID any
	if subtaskID != "" {
		storedSubtaskID = subtaskID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sprints (sprint_id, project_id, category_id, task_id, subtask_id, item_address, sprint_name, sprint_description, sprint_goal, status, priority, estimated_duration_seconds, buffer_pct, buffer_duration_seconds, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, sprint.SprintID, sprint.ProjectID, sprint.CategoryID, sprint.TaskID, storedSubtaskID, sprint.ItemAddress, sprint.Name, sprint.Description, sprint.Goal, sprint.Status, sprint.Priority, sprint.EstimatedDurationSeconds, sprint.BufferPct, sprint.BufferDurationSeconds, stamp(now), stamp(now))
	if err != nil {
		return model.Sprint{}, fmt.Errorf("insert batch Sprint: %w", err)
	}
	return sprint, nil
}

// ValidateTasksBatch checks a legacy task-only batch without creating the
// default category, allocating IDs, or emitting Project events.
func (s *Store) ValidateTasksBatch(ctx context.Context, projectID string, inputs []model.CreateTaskInput) (model.BatchValidationResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return model.BatchValidationResult{}, fmt.Errorf("begin task batch validation: %w", err)
	}
	defer tx.Rollback()

	result := model.BatchValidationResult{Valid: true, Counts: model.BatchCounts{Tasks: len(inputs), TotalObjects: len(inputs)}}
	add := func(path, field, code, message, suggestion string) {
		result.Valid = false
		result.Issues = append(result.Issues, model.ValidationIssue{Path: path, Field: field, Code: code, Message: message, Suggestion: suggestion})
	}
	if len(inputs) == 0 {
		add("", "tasks", "empty_batch", "task batch contains no Tasks", "Add at least one Task to the batch.")
	}
	if len(inputs) > 100 {
		add("", "tasks", "task_limit", "a batch may create at most 100 Tasks", "Split the Task list into smaller batches of no more than 100 Tasks.")
	}
	projectStatus, err := readProjectStatus(ctx, tx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		add("", "project_id", "project_not_found", "target Project was not found", "Use an existing Project ID and retry the smoke test.")
	} else if err != nil {
		return model.BatchValidationResult{}, fmt.Errorf("read task batch Project: %w", err)
	} else if projectStatus != "Open" {
		add("", "project_id", "closed_project", "target Project is not Open", "Choose an Open Project before importing Tasks.")
	}
	for index, input := range inputs {
		path := fmt.Sprintf("tasks[%d]", index)
		if strings.TrimSpace(input.Name) == "" {
			add(path, "name", "required", "name is required", "Set name to a non-empty, human-readable value.")
		}
		if input.EstimatedDurationSeconds < 0 {
			add(path, "estimated_duration_seconds", "invalid_estimate", "estimated duration must not be negative", "Set estimated_duration_seconds to zero or a positive number of seconds.")
		}
		category := strings.TrimSpace(input.CategoryID)
		if category == "" || strings.EqualFold(category, "default") || strings.EqualFold(category, "uncategorized") || strings.EqualFold(category, "general") || strings.EqualFold(category, "C-00000") {
			var defaultStatus string
			err := tx.QueryRowContext(ctx, "SELECT status FROM categories WHERE project_id = ? AND category_id = 'C-00000'", projectID).Scan(&defaultStatus)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return model.BatchValidationResult{}, fmt.Errorf("validate default task batch Category: %w", err)
			}
			if err == nil && defaultStatus != "Open" {
				add(path, "category_id", "closed_category", "default Category is not Open", "Choose an Open Category before importing Tasks.")
			}
			continue
		}
		var categoryStatus string
		err := tx.QueryRowContext(ctx, `SELECT status FROM categories WHERE project_id = ? AND
			(category_id = ? OR category_name = ? COLLATE NOCASE) LIMIT 1`, projectID, category, category).Scan(&categoryStatus)
		if errors.Is(err, sql.ErrNoRows) {
			add(path, "category_id", "category_not_found", "Category was not found in the target Project", "Choose an existing Category ID or name from this Project.")
		} else if err != nil {
			return model.BatchValidationResult{}, fmt.Errorf("validate task batch Category: %w", err)
		} else if categoryStatus != "Open" {
			add(path, "category_id", "closed_category", "Category is not Open", "Choose an Open Category in the target Project.")
		}
	}
	sortBatchIssues(result.Issues)
	return result, nil
}

// ValidateExecutionBatch checks structural and persisted hierarchy rules using
// a read-only transaction. CreateExecutionBatch repeats database checks while
// holding its write transaction to prevent stale validation from authorizing a
// partial or out-of-policy write.
func (s *Store) ValidateExecutionBatch(ctx context.Context, projectID string, input model.ExecutionBatchInput) (model.BatchValidationResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return model.BatchValidationResult{}, fmt.Errorf("begin execution batch validation: %w", err)
	}
	defer tx.Rollback()
	return validateExecutionBatchTx(ctx, tx, projectID, input)
}

func validateExecutionBatchTx(ctx context.Context, tx *sql.Tx, projectID string, input model.ExecutionBatchInput) (model.BatchValidationResult, error) {
	result := model.ValidateExecutionBatchDocument(input)
	projectStatus, err := readProjectStatus(ctx, tx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		appendBatchIssue(&result, "", "project_id", "project_not_found", "target Project was not found", "Use an existing Project ID and retry the smoke test.")
	} else if err != nil {
		return model.BatchValidationResult{}, fmt.Errorf("read execution batch Project: %w", err)
	} else if projectStatus != "Open" {
		appendBatchIssue(&result, "", "project_id", "closed_project", "target Project is not Open", "Choose an Open Project before importing new execution work.")
	}
	if input.Planning != nil && input.Planning.RoutingEvidence != nil {
		issue, evidenceErr := validateRoutingEvidenceTx(ctx, tx, projectID, input.Planning.RoutingEvidence)
		if evidenceErr != nil {
			return model.BatchValidationResult{}, evidenceErr
		}
		if issue != nil {
			appendBatchIssue(&result, issue.Path, issue.Field, issue.Code, issue.Message, issue.Suggestion)
		}
	}

	for index, task := range input.Tasks {
		path := fmt.Sprintf("tasks[%d]", index)
		categoryID := strings.TrimSpace(task.CategoryID)
		if categoryID == "" {
			continue
		}
		categoryProject, categoryStatus, err := readCategoryOwner(ctx, tx, categoryID)
		if errors.Is(err, sql.ErrNoRows) && strings.EqualFold(categoryID, "C-00000") {
			// Core creates the default Category in the apply transaction when
			// a normal Task API request targets a Project without one yet.
			continue
		}
		if errors.Is(err, sql.ErrNoRows) {
			categoryProject, categoryStatus, err = readCategoryOwnerByName(ctx, tx, projectID, categoryID)
		}
		if errors.Is(err, sql.ErrNoRows) {
			appendBatchIssue(&result, path, "category_id", "category_not_found", "Category was not found", "Set category_id to an existing Category ID or name in the target Project.")
			continue
		}
		if err != nil {
			return model.BatchValidationResult{}, fmt.Errorf("read execution batch Category: %w", err)
		}
		if categoryProject != projectID {
			appendBatchIssue(&result, path, "category_id", "foreign_category", "Category belongs to a different Project", "Choose a Category ID from the target Project.")
			continue
		}
		if categoryStatus != "Open" || projectStatus != "Open" {
			appendBatchIssue(&result, path, "category_id", "closed_category", "new Task parent Project and Category must be Open", "Choose an Open Category in an Open Project.")
		}
	}
	for index, extension := range input.Extensions {
		path := fmt.Sprintf("extensions[%d]", index)
		if strings.TrimSpace(extension.TaskID) != "" {
			project, categoryStatus, taskStatus, err := readTaskOwnerStatus(ctx, tx, extension.TaskID)
			if errors.Is(err, sql.ErrNoRows) {
				appendBatchIssue(&result, path, "task_id", "owner_not_found", "Task owner was not found", "Choose an Open Task ID from the target Project.")
				continue
			}
			if err != nil {
				return model.BatchValidationResult{}, fmt.Errorf("read execution batch Task owner: %w", err)
			}
			if project != projectID {
				appendBatchIssue(&result, path, "task_id", "foreign_owner", "Task owner belongs to a different Project", "Choose an Open Task ID from the target Project.")
				continue
			}
			taskStatus, taskProjectStatus, taskCategoryStatus, err := readTaskParentStates(ctx, tx, extension.TaskID)
			if err != nil {
				return model.BatchValidationResult{}, fmt.Errorf("read execution batch Task parent states: %w", err)
			}
			if taskStatus != "Open" || taskProjectStatus != "Open" || taskCategoryStatus != "Open" || categoryStatus != "Open" || projectStatus != "Open" {
				appendBatchIssue(&result, path, "task_id", "closed_owner", "Task extension parent chain must be Open", "Choose an Open Task under an Open Category and Project.")
			}
			if err := validateTaskSprintCapacity(ctx, tx, extension.TaskID, "", len(extension.Sprints), path, &result); err != nil {
				return model.BatchValidationResult{}, err
			}
			continue
		}
		if strings.TrimSpace(extension.SubtaskID) != "" {
			project, taskID, taskStatus, subtaskStatus, categoryStatus, projectParentStatus, err := readSubtaskOwnerStatus(ctx, tx, extension.SubtaskID)
			if errors.Is(err, sql.ErrNoRows) {
				appendBatchIssue(&result, path, "subtask_id", "owner_not_found", "Subtask owner was not found", "Choose an Open Subtask ID in the target Project.")
				continue
			}
			if err != nil {
				return model.BatchValidationResult{}, fmt.Errorf("read execution batch Subtask owner: %w", err)
			}
			if project != projectID {
				appendBatchIssue(&result, path, "subtask_id", "foreign_owner", "Subtask owner belongs to a different Project", "Choose an Open Subtask ID from the target Project.")
				continue
			}
			if taskStatus != "Open" || subtaskStatus != "Open" || categoryStatus != "Open" || projectParentStatus != "Open" || projectStatus != "Open" {
				appendBatchIssue(&result, path, "subtask_id", "closed_owner", "Subtask extension parent chain must be Open", "Choose an Open Subtask under an Open Task, Category, and Project.")
			}
			if err := validateTaskSprintCapacity(ctx, tx, taskID, extension.SubtaskID, len(extension.Sprints), path, &result); err != nil {
				return model.BatchValidationResult{}, err
			}
		}
	}
	sortBatchIssues(result.Issues)
	result.Valid = len(result.Issues) == 0
	return result, nil
}

func appendBatchIssue(result *model.BatchValidationResult, path, field, code, message, suggestion string) {
	result.Valid = false
	result.Issues = append(result.Issues, model.ValidationIssue{Path: path, Field: field, Code: code, Message: message, Suggestion: suggestion})
}

func ensureProjectReadable(ctx context.Context, tx *sql.Tx, projectID string) error {
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return err
	}
	return nil
}

func readProjectStatus(ctx context.Context, tx *sql.Tx, projectID string) (string, error) {
	var status string
	err := tx.QueryRowContext(ctx, "SELECT status FROM projects WHERE project_id = ?", projectID).Scan(&status)
	return status, err
}

func readCategoryOwner(ctx context.Context, tx *sql.Tx, categoryID string) (projectID, status string, err error) {
	err = tx.QueryRowContext(ctx, "SELECT project_id, status FROM categories WHERE category_id = ?", categoryID).Scan(&projectID, &status)
	return projectID, status, err
}

func readCategoryOwnerByName(ctx context.Context, tx *sql.Tx, projectID, name string) (string, string, error) {
	var ownerProject, status string
	err := tx.QueryRowContext(ctx, "SELECT project_id, status FROM categories WHERE project_id = ? AND category_name = ? COLLATE NOCASE", projectID, strings.TrimSpace(name)).Scan(&ownerProject, &status)
	return ownerProject, status, err
}

func readTaskOwnerStatus(ctx context.Context, tx *sql.Tx, taskID string) (projectID, categoryStatus, taskStatus string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT t.project_id, c.status, t.status
		FROM tasks t JOIN categories c ON c.category_id = t.category_id WHERE t.task_id = ?`, taskID).Scan(&projectID, &categoryStatus, &taskStatus)
	return projectID, categoryStatus, taskStatus, err
}

func readTaskParentStates(ctx context.Context, tx *sql.Tx, taskID string) (taskStatus, projectStatus, categoryStatus string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT t.status, p.status, c.status
		FROM tasks t JOIN projects p ON p.project_id = t.project_id JOIN categories c ON c.category_id = t.category_id
		WHERE t.task_id = ?`, taskID).Scan(&taskStatus, &projectStatus, &categoryStatus)
	return taskStatus, projectStatus, categoryStatus, err
}

func readSubtaskOwnerStatus(ctx context.Context, tx *sql.Tx, subtaskID string) (projectID, taskID, taskStatus, subtaskStatus, categoryStatus, projectStatus string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT st.project_id, st.task_id, t.status, st.status, c.status, p.status
		FROM subtasks st JOIN tasks t ON t.task_id = st.task_id JOIN categories c ON c.category_id = st.category_id JOIN projects p ON p.project_id = st.project_id
		WHERE st.subtask_id = ?`, subtaskID).Scan(&projectID, &taskID, &taskStatus, &subtaskStatus, &categoryStatus, &projectStatus)
	return projectID, taskID, taskStatus, subtaskStatus, categoryStatus, projectStatus, err
}

func validateTaskSprintCapacity(ctx context.Context, tx *sql.Tx, taskID, subtaskID string, adding int, path string, result *model.BatchValidationResult) error {
	var existing int
	var err error
	if subtaskID != "" {
		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sprints WHERE subtask_id = ?", subtaskID).Scan(&existing)
	} else {
		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sprints WHERE task_id = ? AND subtask_id IS NULL", taskID).Scan(&existing)
	}
	if err != nil {
		return fmt.Errorf("count execution batch owner Sprints: %w", err)
	}
	if existing+adding > 4 {
		appendBatchIssue(result, path, "sprints", "sprint_limit", "owner would have more than four direct Sprints", "Remove Sprints or split the remaining work into Subtasks so this owner has at most four direct Sprints.")
	}
	return nil
}

func sortBatchIssues(issues []model.ValidationIssue) {
	key := func(path string) string {
		if path == "" {
			return "0"
		}
		section, rest, found := strings.Cut(path, "[")
		if !found {
			return "9" + path
		}
		rank := "1"
		if section == "tasks" {
			rank = "1"
		} else if section == "extensions" {
			rank = "2"
		} else {
			rank = "3" + section
		}
		indexPart, suffix, _ := strings.Cut(rest, "]")
		index, _ := strconv.Atoi(indexPart)
		return fmt.Sprintf("%s%08d%s", rank, index, suffix)
	}
	sort.SliceStable(issues, func(i, j int) bool { return key(issues[i].Path) < key(issues[j].Path) })
}
