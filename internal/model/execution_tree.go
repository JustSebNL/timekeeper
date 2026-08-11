// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

// ExecutionTree is a read projection of all executable work below one project.
type ExecutionTree struct {
	Project    Project             `json:"project"`
	Categories []ExecutionCategory `json:"categories"`
}

// ExecutionCategory groups a project's tasks beneath one category.
type ExecutionCategory struct {
	Category   Category            `json:"category"`
	Categories []ExecutionCategory `json:"categories"`
	Tasks      []ExecutionTask     `json:"tasks"`
}

// ExecutionTask keeps direct Sprint ownership distinct from nested Subtask ownership.
type ExecutionTask struct {
	Task     Task               `json:"task"`
	Sprints  []Sprint           `json:"sprints"`
	Subtasks []ExecutionSubtask `json:"subtasks"`
}

// ExecutionSubtask contains its directly owned Sprints.
type ExecutionSubtask struct {
	Subtask Subtask  `json:"subtask"`
	Sprints []Sprint `json:"sprints"`
}
