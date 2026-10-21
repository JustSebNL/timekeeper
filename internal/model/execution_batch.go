package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExecutionBatchInput is a versioned request to create a hierarchy inside an
// existing Project. Planning data is descriptive only and never creates rows.
type ExecutionBatchInput struct {
	Version    string                    `json:"version" yaml:"version"`
	Tasks      []ExecutionBatchTask      `json:"tasks" yaml:"tasks"`
	Extensions []ExecutionBatchExtension `json:"extensions,omitempty" yaml:"extensions,omitempty"`
	Planning   *ExecutionBatchPlanning   `json:"planning,omitempty" yaml:"planning,omitempty"`
}

// ExecutionBatchTask is a new Task and any new descendants beneath it.
type ExecutionBatchTask struct {
	CategoryID               string                  `json:"category_id" yaml:"category_id"`
	Name                     string                  `json:"name" yaml:"name"`
	Description              string                  `json:"description,omitempty" yaml:"description,omitempty"`
	Goal                     string                  `json:"goal,omitempty" yaml:"goal,omitempty"`
	Priority                 string                  `json:"priority,omitempty" yaml:"priority,omitempty"`
	EstimatedDurationSeconds int64                   `json:"estimated_duration_seconds,omitempty" yaml:"estimated_duration_seconds,omitempty"`
	AutoClose                *bool                   `json:"auto_close,omitempty" yaml:"auto_close,omitempty"`
	Sprints                  []CreateSprintInput     `json:"sprints,omitempty" yaml:"sprints,omitempty"`
	Subtasks                 []ExecutionBatchSubtask `json:"subtasks,omitempty" yaml:"subtasks,omitempty"`
}

// ExecutionBatchSubtask is a new Subtask and its new Sprints.
type ExecutionBatchSubtask struct {
	Name                     string              `json:"name" yaml:"name"`
	Description              string              `json:"description,omitempty" yaml:"description,omitempty"`
	Goal                     string              `json:"goal,omitempty" yaml:"goal,omitempty"`
	Priority                 string              `json:"priority,omitempty" yaml:"priority,omitempty"`
	EstimatedDurationSeconds int64               `json:"estimated_duration_seconds,omitempty" yaml:"estimated_duration_seconds,omitempty"`
	Sprints                  []CreateSprintInput `json:"sprints,omitempty" yaml:"sprints,omitempty"`
}

// ExecutionBatchExtension adds new children to exactly one existing owner.
type ExecutionBatchExtension struct {
	TaskID    string                  `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	SubtaskID string                  `json:"subtask_id,omitempty" yaml:"subtask_id,omitempty"`
	Subtasks  []ExecutionBatchSubtask `json:"subtasks,omitempty" yaml:"subtasks,omitempty"`
	Sprints   []CreateSprintInput     `json:"sprints,omitempty" yaml:"sprints,omitempty"`
}

// ExecutionBatchPlanning retains migration context without making it executable.
type ExecutionBatchPlanning struct {
	ExecutableTaskMetadata []ExecutionBatchTaskMetadata   `json:"executable_task_metadata,omitempty" yaml:"executable_task_metadata,omitempty"`
	DeferredItems          []ExecutionBatchDeferredItem   `json:"deferred_items,omitempty" yaml:"deferred_items,omitempty"`
	ReleaseGates           []ExecutionBatchReleaseGate    `json:"release_gates,omitempty" yaml:"release_gates,omitempty"`
	ApplicationConstraints []string                       `json:"application_constraints,omitempty" yaml:"application_constraints,omitempty"`
	RoutingEvidence        *ExecutionBatchRoutingEvidence `json:"routing_evidence,omitempty" yaml:"routing_evidence,omitempty"`
}

// ExecutionBatchRoutingEvidence records why an idempotent Core Apply created
// this batch. It is retained in the durable registration payload; it never
// changes which hierarchy objects the batch creates.
type ExecutionBatchRoutingEvidence struct {
	DecisionID         string          `json:"decision_id" yaml:"decision_id"`
	PolicyVersion      string          `json:"policy_version" yaml:"policy_version"`
	CandidateStateHash string          `json:"candidate_state_hash" yaml:"candidate_state_hash"`
	IntentHash         string          `json:"intent_hash" yaml:"intent_hash"`
	Model              string          `json:"model,omitempty" yaml:"model,omitempty"`
	Confirmed          bool            `json:"confirmed" yaml:"confirmed"`
	Decision           json.RawMessage `json:"decision" yaml:"decision"`
}

// ExecutionBatchTaskMetadata maps planning fields to a new Task by array index.
type ExecutionBatchTaskMetadata struct {
	TaskIndex  int      `json:"task_index" yaml:"task_index"`
	Key        string   `json:"key" yaml:"key"`
	Phase      int      `json:"phase,omitempty" yaml:"phase,omitempty"`
	Phases     []int    `json:"phases,omitempty" yaml:"phases,omitempty"`
	Goal       string   `json:"goal,omitempty" yaml:"goal,omitempty"`
	Acceptance []string `json:"acceptance,omitempty" yaml:"acceptance,omitempty"`
	Condition  string   `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// ExecutionBatchDeferredItem is a planning-only idea that is not created.
type ExecutionBatchDeferredItem struct {
	Key        string   `json:"key" yaml:"key"`
	Condition  string   `json:"condition" yaml:"condition"`
	Acceptance []string `json:"acceptance,omitempty" yaml:"acceptance,omitempty"`
}

// ExecutionBatchReleaseGate records a non-executable release requirement.
type ExecutionBatchReleaseGate struct {
	Reference        string   `json:"reference" yaml:"reference"`
	Phase            int      `json:"phase,omitempty" yaml:"phase,omitempty"`
	Phases           []int    `json:"phases,omitempty" yaml:"phases,omitempty"`
	RequiredEvidence []string `json:"required_evidence" yaml:"required_evidence"`
}

// ValidationIssue is a user-correctable batch validation finding.
type ValidationIssue struct {
	Path       string `json:"path"`
	Field      string `json:"field"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// BatchCounts contains only executable objects, never planning metadata.
type BatchCounts struct {
	Tasks        int `json:"tasks"`
	Subtasks     int `json:"subtasks"`
	Sprints      int `json:"sprints"`
	TotalObjects int `json:"total_objects"`
}

// BatchValidationResult reports a no-write smoke test result.
type BatchValidationResult struct {
	Valid  bool              `json:"valid"`
	Counts BatchCounts       `json:"counts"`
	Issues []ValidationIssue `json:"issues"`
}

// ExecutionBatchResult reports every newly created executable entity.
type ExecutionBatchResult struct {
	Tasks    []Task      `json:"tasks"`
	Subtasks []Subtask   `json:"subtasks"`
	Sprints  []Sprint    `json:"sprints"`
	Counts   BatchCounts `json:"counts"`
}

// ValidateExecutionBatchDocument checks rules that do not require a Store and
// counts executable items only. Database ownership/state checks belong to Core.
func ValidateExecutionBatchDocument(input ExecutionBatchInput) BatchValidationResult {
	result := BatchValidationResult{Valid: true, Counts: BatchCounts{Tasks: len(input.Tasks)}}
	add := func(path, field, code, message, suggestion string) {
		result.Valid = false
		result.Issues = append(result.Issues, ValidationIssue{Path: path, Field: field, Code: code, Message: message, Suggestion: suggestion})
	}
	if input.Version != "timekeeper-execution-batch/v1" {
		add("", "version", "unsupported_version", "version must be timekeeper-execution-batch/v1", "Set version to timekeeper-execution-batch/v1.")
	}
	if len(input.Tasks) == 0 && len(input.Extensions) == 0 {
		add("", "tasks", "empty_batch", "batch contains no executable objects", "Add at least one Task or one extension with a Subtask or Sprint.")
	}
	if len(input.Tasks) > 100 {
		add("", "tasks", "task_limit", "a batch may create at most 100 Tasks", "Split the Task list into smaller batches of no more than 100 Tasks.")
	}
	for taskIndex, task := range input.Tasks {
		path := fmt.Sprintf("tasks[%d]", taskIndex)
		validateBatchTask(path, task.Name, task.EstimatedDurationSeconds, add)
		if strings.TrimSpace(task.CategoryID) == "" {
			add(path, "category_id", "required", "category_id is required", "Set category_id to an existing Category ID in the target Project.")
		}
		if len(task.Sprints) > 4 {
			add(path, "sprints", "sprint_limit", "a Task may own at most four direct Sprints", "Keep no more than four Sprints under this Task and split the remaining work into Subtasks.")
		}
		for sprintIndex, sprint := range task.Sprints {
			validateBatchSprint(fmt.Sprintf("%s.sprints[%d]", path, sprintIndex), sprint, add)
			result.Counts.Sprints++
		}
		for subtaskIndex, subtask := range task.Subtasks {
			subtaskPath := fmt.Sprintf("%s.subtasks[%d]", path, subtaskIndex)
			validateBatchTask(subtaskPath, subtask.Name, subtask.EstimatedDurationSeconds, add)
			result.Counts.Subtasks++
			if len(subtask.Sprints) > 4 {
				add(subtaskPath, "sprints", "sprint_limit", "a Subtask may own at most four direct Sprints", "Keep no more than four Sprints under this Subtask and split the remaining work into another Subtask.")
			}
			for sprintIndex, sprint := range subtask.Sprints {
				validateBatchSprint(fmt.Sprintf("%s.sprints[%d]", subtaskPath, sprintIndex), sprint, add)
				result.Counts.Sprints++
			}
		}
	}
	owners := make(map[string]int)
	for extensionIndex, extension := range input.Extensions {
		path := fmt.Sprintf("extensions[%d]", extensionIndex)
		if (strings.TrimSpace(extension.TaskID) == "") == (strings.TrimSpace(extension.SubtaskID) == "") {
			add(path, "owner_id", "invalid_owner", "extension must target exactly one existing Task or Subtask", "Set task_id or subtask_id, but not both, to an open owner in the target Project.")
		} else {
			owner := "task:" + strings.TrimSpace(extension.TaskID)
			field := "task_id"
			if strings.TrimSpace(extension.TaskID) == "" {
				owner = "subtask:" + strings.TrimSpace(extension.SubtaskID)
				field = "subtask_id"
			}
			if first, exists := owners[owner]; exists {
				add(path, field, "duplicate_owner", fmt.Sprintf("this owner is already extended by extensions[%d]", first), "Combine additions for this owner into one extension entry.")
			} else {
				owners[owner] = extensionIndex
			}
		}
		if strings.TrimSpace(extension.SubtaskID) != "" && len(extension.Subtasks) > 0 {
			add(path, "subtasks", "unsupported_child_level", "a Subtask extension cannot create another Subtask level", "Attach new Subtasks to a Task extension; use this Subtask extension only for Sprints.")
		}
		if len(extension.Subtasks) == 0 && len(extension.Sprints) == 0 {
			add(path, "subtasks", "empty_extension", "extension contains no new Subtasks or Sprints", "Add at least one new Subtask or Sprint, or remove this extension.")
		}
		if len(extension.Sprints) > 4 {
			add(path, "sprints", "sprint_limit", "an extension may add at most four direct Sprints to one owner", "Keep no more than four direct Sprints in this extension and split the remaining work into Subtasks.")
		}
		for sprintIndex, sprint := range extension.Sprints {
			validateBatchSprint(fmt.Sprintf("%s.sprints[%d]", path, sprintIndex), sprint, add)
			result.Counts.Sprints++
		}
		for subtaskIndex, subtask := range extension.Subtasks {
			subtaskPath := fmt.Sprintf("%s.subtasks[%d]", path, subtaskIndex)
			validateBatchTask(subtaskPath, subtask.Name, subtask.EstimatedDurationSeconds, add)
			result.Counts.Subtasks++
			if len(subtask.Sprints) > 4 {
				add(subtaskPath, "sprints", "sprint_limit", "a Subtask may own at most four direct Sprints", "Keep no more than four Sprints under this Subtask and split the remaining work into another Subtask.")
			}
			for sprintIndex, sprint := range subtask.Sprints {
				validateBatchSprint(fmt.Sprintf("%s.sprints[%d]", subtaskPath, sprintIndex), sprint, add)
				result.Counts.Sprints++
			}
		}
	}
	result.Counts.TotalObjects = result.Counts.Tasks + result.Counts.Subtasks + result.Counts.Sprints
	if result.Counts.TotalObjects > 500 {
		add("", "tasks", "object_limit", "a batch may create at most 500 total Tasks, Subtasks, and Sprints", "Split the executable hierarchy into batches of no more than 500 created objects.")
	}
	if input.Planning != nil {
		for index, metadata := range input.Planning.ExecutableTaskMetadata {
			if metadata.TaskIndex < 0 || metadata.TaskIndex >= len(input.Tasks) {
				path := fmt.Sprintf("planning.executable_task_metadata[%d]", index)
				add(path, "task_index", "invalid_task_index", "task_index does not reference an executable Task", "Set task_index to the zero-based index of a Task in tasks.")
			}
		}
	}
	return result
}

func validateBatchTask(path, name string, estimate int64, add func(string, string, string, string, string)) {
	if strings.TrimSpace(name) == "" {
		add(path, "name", "required", "name is required", "Set name to a non-empty, human-readable value.")
	}
	if estimate < 0 {
		add(path, "estimated_duration_seconds", "invalid_estimate", "estimated duration must not be negative", "Set estimated_duration_seconds to zero or a positive number of seconds.")
	}
}

func validateBatchSprint(path string, sprint CreateSprintInput, add func(string, string, string, string, string)) {
	if strings.TrimSpace(sprint.Name) == "" {
		add(path, "name", "required", "name is required", "Set name to a non-empty, human-readable value.")
	}
	if sprint.EstimatedDurationSeconds <= 0 {
		add(path, "estimated_duration_seconds", "invalid_estimate", "Sprint estimate must be greater than zero", "Set estimated_duration_seconds to a positive number of seconds.")
	}
	if sprint.BufferPct < 0 || sprint.BufferPct > 100 || sprint.BufferPct != float64(int(sprint.BufferPct)) {
		add(path, "buffer_pct", "invalid_buffer", "buffer_pct must be a whole number from 0 through 100", "Set buffer_pct to an integer between 0 and 100.")
	}
}
