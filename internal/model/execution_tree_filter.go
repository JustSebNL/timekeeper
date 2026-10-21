package model

import "strings"

// FilterExecutionTree keeps matching Tasks and their Category ownership paths.
func FilterExecutionTree(tree ExecutionTree, taskID, status string) (ExecutionTree, bool) {
	categories, found := filterExecutionCategories(tree.Categories, taskID, status)
	if !found {
		return tree, false
	}
	tree.Categories = categories
	return tree, true
}

func filterExecutionCategories(categories []ExecutionCategory, taskID, status string) ([]ExecutionCategory, bool) {
	filtered := make([]ExecutionCategory, 0, len(categories))
	found := false
	for _, category := range categories {
		tasks := make([]ExecutionTask, 0, len(category.Tasks))
		for _, task := range category.Tasks {
			matchesID := taskID == "" || task.Task.TaskID == taskID
			matchesStatus := status == "" || strings.EqualFold(task.Task.Status, status)
			if matchesID && matchesStatus {
				tasks = append(tasks, task)
				found = true
			}
		}
		children, childFound := filterExecutionCategories(category.Categories, taskID, status)
		if childFound {
			found = true
		}
		if len(tasks) == 0 && len(children) == 0 {
			continue
		}
		category.Tasks = tasks
		category.Categories = children
		filtered = append(filtered, category)
	}
	return filtered, found
}

// CountExecutionTreeTasks counts Tasks at every nested Category depth.
func CountExecutionTreeTasks(tree ExecutionTree) int {
	count := 0
	var countCategories func([]ExecutionCategory)
	countCategories = func(categories []ExecutionCategory) {
		for _, category := range categories {
			count += len(category.Tasks)
			countCategories(category.Categories)
		}
	}
	countCategories(tree.Categories)
	return count
}

// LimitExecutionTreeTasks returns a stable depth-first prefix and its metadata.
func LimitExecutionTreeTasks(tree ExecutionTree, limit int) (ExecutionTree, int, bool) {
	total := CountExecutionTreeTasks(tree)
	remaining := limit
	var limitCategories func([]ExecutionCategory) []ExecutionCategory
	limitCategories = func(categories []ExecutionCategory) []ExecutionCategory {
		filtered := make([]ExecutionCategory, 0, len(categories))
		for _, category := range categories {
			tasks := make([]ExecutionTask, 0, len(category.Tasks))
			for _, task := range category.Tasks {
				if remaining <= 0 {
					break
				}
				tasks = append(tasks, task)
				remaining--
			}
			children := limitCategories(category.Categories)
			if len(tasks) == 0 && len(children) == 0 {
				continue
			}
			category.Tasks = tasks
			category.Categories = children
			filtered = append(filtered, category)
		}
		return filtered
	}
	tree.Categories = limitCategories(tree.Categories)
	return tree, limit - remaining, total > limit
}

// LimitExecutionTreeDepth removes descendants deeper than maxDepth.
func LimitExecutionTreeDepth(tree ExecutionTree, maxDepth int) ExecutionTree {
	tree.Categories = limitExecutionCategoryDepth(tree.Categories, 1, maxDepth)
	return tree
}

func limitExecutionCategoryDepth(categories []ExecutionCategory, categoryDepth, maxDepth int) []ExecutionCategory {
	limited := make([]ExecutionCategory, 0, len(categories))
	for _, category := range categories {
		if categoryDepth >= maxDepth {
			category.Categories = nil
			category.Tasks = nil
			limited = append(limited, category)
			continue
		}
		taskDepth := categoryDepth + 1
		if taskDepth > maxDepth {
			category.Tasks = nil
		} else {
			for i := range category.Tasks {
				childDepth := taskDepth + 1
				if childDepth > maxDepth {
					category.Tasks[i].Sprints = nil
					category.Tasks[i].Subtasks = nil
					continue
				}
				for j := range category.Tasks[i].Subtasks {
					if childDepth+1 > maxDepth {
						category.Tasks[i].Subtasks[j].Sprints = nil
					}
				}
			}
		}
		category.Categories = limitExecutionCategoryDepth(category.Categories, categoryDepth+1, maxDepth)
		limited = append(limited, category)
	}
	return limited
}

// FilterExecutionTreeToActiveSprints keeps only Tasks/Subtasks owning Active Sprints.
func FilterExecutionTreeToActiveSprints(tree ExecutionTree) (ExecutionTree, bool) {
	categories, found := filterExecutionCategoriesToActiveSprints(tree.Categories)
	if !found {
		return tree, false
	}
	tree.Categories = categories
	return tree, true
}

func filterExecutionCategoriesToActiveSprints(categories []ExecutionCategory) ([]ExecutionCategory, bool) {
	filtered := make([]ExecutionCategory, 0, len(categories))
	found := false
	for _, category := range categories {
		tasks := make([]ExecutionTask, 0, len(category.Tasks))
		for _, task := range category.Tasks {
			activeSprints := make([]Sprint, 0, len(task.Sprints))
			for _, sprint := range task.Sprints {
				if strings.EqualFold(sprint.Status, "Active") {
					activeSprints = append(activeSprints, sprint)
				}
			}
			subtasks := make([]ExecutionSubtask, 0, len(task.Subtasks))
			for _, subtask := range task.Subtasks {
				sprints := make([]Sprint, 0, len(subtask.Sprints))
				for _, sprint := range subtask.Sprints {
					if strings.EqualFold(sprint.Status, "Active") {
						sprints = append(sprints, sprint)
					}
				}
				if len(sprints) > 0 {
					subtask.Sprints = sprints
					subtasks = append(subtasks, subtask)
				}
			}
			if len(activeSprints) == 0 && len(subtasks) == 0 {
				continue
			}
			task.Sprints = activeSprints
			task.Subtasks = subtasks
			tasks = append(tasks, task)
			found = true
		}
		children, childFound := filterExecutionCategoriesToActiveSprints(category.Categories)
		if childFound {
			found = true
		}
		if len(tasks) == 0 && len(children) == 0 {
			continue
		}
		category.Tasks = tasks
		category.Categories = children
		filtered = append(filtered, category)
	}
	return filtered, found
}
