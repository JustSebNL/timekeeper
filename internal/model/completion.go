// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "math"

// CalculateExecutionTreeCompletion derives calculated completion from completed
// Sprint structure. It does not overwrite reported completion or mutate storage.
// Sprint weights use estimate plus declared planning buffer; leaf Tasks/Subtasks
// without Sprints use their own estimate and explicit Completed status.
func CalculateExecutionTreeCompletion(tree *ExecutionTree) {
	var projectTotal, projectCompleted int64
	for categoryIndex := range tree.Categories {
		total, completed := calculateCategoryCompletion(&tree.Categories[categoryIndex])
		projectTotal += total
		projectCompleted += completed
	}
	tree.Project.CalculatedCompletionPct = completionPct(projectCompleted, projectTotal)
}

func calculateCategoryCompletion(category *ExecutionCategory) (int64, int64) {
	var total, completed int64
	for taskIndex := range category.Tasks {
		task := &category.Tasks[taskIndex]
		taskTotal, taskCompleted := calculateTaskCompletion(task)
		task.Task.CalculatedCompletionPct = completionPct(taskCompleted, taskTotal)
		total += taskTotal
		completed += taskCompleted
	}
	for childIndex := range category.Categories {
		childTotal, childCompleted := calculateCategoryCompletion(&category.Categories[childIndex])
		total += childTotal
		completed += childCompleted
	}
	category.Category.ProgressPct = completionPct(completed, total)
	return total, completed
}

func calculateTaskCompletion(task *ExecutionTask) (int64, int64) {
	total, completed := sprintCompletionWeight(task.Sprints)
	for subtaskIndex := range task.Subtasks {
		subtask := &task.Subtasks[subtaskIndex]
		subtaskTotal, subtaskCompleted := calculateSubtaskCompletion(subtask)
		subtask.Subtask.CalculatedCompletionPct = completionPct(subtaskCompleted, subtaskTotal)
		total += subtaskTotal
		completed += subtaskCompleted
	}
	if total == 0 {
		total = nonzeroWeight(task.Task.EstimatedDurationSeconds)
		if task.Task.Status == "Completed" {
			completed = total
		}
	}
	return total, completed
}

func calculateSubtaskCompletion(subtask *ExecutionSubtask) (int64, int64) {
	total, completed := sprintCompletionWeight(subtask.Sprints)
	if total == 0 {
		total = nonzeroWeight(subtask.Subtask.EstimatedDurationSeconds)
		if subtask.Subtask.Status == "Completed" {
			completed = total
		}
	}
	return total, completed
}

func sprintCompletionWeight(sprints []Sprint) (int64, int64) {
	var total, completed int64
	for _, sprint := range sprints {
		weight := nonzeroWeight(sprint.EstimatedDurationSeconds + sprint.BufferDurationSeconds)
		total += weight
		if sprint.Status == "Completed" {
			completed += weight
		}
	}
	return total, completed
}

func nonzeroWeight(seconds int64) int64 {
	if seconds > 0 {
		return seconds
	}
	return 1
}

func completionPct(completed, total int64) float64 {
	if total == 0 {
		return 0
	}
	return math.Round((float64(completed)*100/float64(total))*10) / 10
}
