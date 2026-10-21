// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package routing

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const (
	DecisionVersion = "timekeeper-jev-decision/v1"
	PolicyVersion   = "jev-routing-policy/v1"
)

type Candidate struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	ParentID    string `json:"parent_id,omitempty"`
	Name        string `json:"name"`
	Goal        string `json:"goal,omitempty"`
	Status      string `json:"status"`
	Runnable    bool   `json:"runnable,omitempty"`
	SprintCount int    `json:"sprint_count,omitempty"`
}

type CandidateSet struct {
	ProjectID     string      `json:"project_id"`
	ProjectName   string      `json:"project_name"`
	ProjectStatus string      `json:"project_status"`
	Categories    []Candidate `json:"categories"`
	Tasks         []Candidate `json:"tasks"`
	Subtasks      []Candidate `json:"subtasks"`
	Sprints       []Candidate `json:"sprints"`
	Truncated     bool        `json:"truncated"`
	StateHash     string      `json:"state_hash"`
}

type Limits struct {
	MaxCandidates  int
	MaxNameRunes   int
	MaxGoalRunes   int
	MaxIntentRunes int
	MaxActorRunes  int
}

func DefaultLimits() Limits {
	return Limits{MaxCandidates: 200, MaxNameRunes: 120, MaxGoalRunes: 240, MaxIntentRunes: 2048, MaxActorRunes: 120}
}

func CandidateSetFromTree(tree model.ExecutionTree, limits Limits) CandidateSet {
	if limits.MaxCandidates <= 0 {
		limits.MaxCandidates = DefaultLimits().MaxCandidates
	}
	if limits.MaxNameRunes <= 0 {
		limits.MaxNameRunes = DefaultLimits().MaxNameRunes
	}
	if limits.MaxGoalRunes <= 0 {
		limits.MaxGoalRunes = DefaultLimits().MaxGoalRunes
	}

	set := CandidateSet{
		ProjectID:     tree.Project.ProjectID,
		ProjectName:   boundedText(tree.Project.ProjectName, limits.MaxNameRunes),
		ProjectStatus: tree.Project.Status,
	}
	var categories, tasks, subtasks, sprints []Candidate
	ownerCounts := map[string]int{}
	parentOpen := map[string]bool{tree.Project.ProjectID: tree.Project.Status == "Open"}

	var visitCategory func(model.ExecutionCategory, string)
	visitCategory = func(node model.ExecutionCategory, parentID string) {
		category := node.Category
		categories = append(categories, Candidate{
			ID:       category.CategoryID,
			Kind:     "category",
			ParentID: parentID,
			Name:     boundedText(category.Name, limits.MaxNameRunes),
			Goal:     boundedText(firstNonEmpty(category.Goal, category.Description), limits.MaxGoalRunes),
			Status:   category.Status,
		})
		parentOpen[category.CategoryID] = category.Status == "Open" && parentOpen[parentID]
		for _, child := range node.Categories {
			visitCategory(child, category.CategoryID)
		}
		for _, taskNode := range node.Tasks {
			task := taskNode.Task
			tasks = append(tasks, Candidate{
				ID:       task.TaskID,
				Kind:     "task",
				ParentID: category.CategoryID,
				Name:     boundedText(task.Name, limits.MaxNameRunes),
				Goal:     boundedText(firstNonEmpty(task.Goal, task.Description), limits.MaxGoalRunes),
				Status:   task.Status,
			})
			parentOpen[task.TaskID] = task.Status == "Open" && parentOpen[category.CategoryID]
			ownerCounts[task.TaskID] += len(taskNode.Sprints)
			for _, sprint := range taskNode.Sprints {
				sprints = append(sprints, sprintCandidate(sprint, task.TaskID, limits))
			}
			for _, subtaskNode := range taskNode.Subtasks {
				subtask := subtaskNode.Subtask
				subtasks = append(subtasks, Candidate{
					ID:       subtask.SubtaskID,
					Kind:     "subtask",
					ParentID: task.TaskID,
					Name:     boundedText(subtask.Name, limits.MaxNameRunes),
					Goal:     boundedText(firstNonEmpty(subtask.Goal, subtask.Description), limits.MaxGoalRunes),
					Status:   subtask.Status,
				})
				parentOpen[subtask.SubtaskID] = subtask.Status == "Open" && parentOpen[task.TaskID]
				ownerCounts[subtask.SubtaskID] += len(subtaskNode.Sprints)
				for _, sprint := range subtaskNode.Sprints {
					sprints = append(sprints, sprintCandidate(sprint, subtask.SubtaskID, limits))
				}
			}
		}
	}
	for _, category := range tree.Categories {
		visitCategory(category, tree.Project.ProjectID)
	}

	for index := range tasks {
		tasks[index].SprintCount = ownerCounts[tasks[index].ID]
	}
	for index := range subtasks {
		subtasks[index].SprintCount = ownerCounts[subtasks[index].ID]
	}
	for index := range sprints {
		sprints[index].Runnable = sprints[index].Status == "Open" && parentOpen[sprints[index].ParentID]
	}
	sortCandidates(categories)
	sortCandidates(tasks)
	sortCandidates(subtasks)
	sortCandidates(sprints)

	set.Categories = categories
	set.Tasks = tasks
	set.Subtasks = subtasks
	set.Sprints = sprints
	set.Truncate(limits.MaxCandidates)
	set.StateHash = set.Hash()
	return set
}

func sprintCandidate(sprint model.Sprint, parentID string, limits Limits) Candidate {
	return Candidate{
		ID:       sprint.SprintID,
		Kind:     "sprint",
		ParentID: parentID,
		Name:     boundedText(sprint.Name, limits.MaxNameRunes),
		Goal:     boundedText(firstNonEmpty(sprint.Goal, sprint.Description), limits.MaxGoalRunes),
		Status:   sprint.Status,
	}
}

func (set *CandidateSet) Truncate(max int) {
	total := len(set.Categories) + len(set.Tasks) + len(set.Subtasks) + len(set.Sprints)
	if total <= max {
		return
	}
	set.Truncated = true
	remaining := max
	set.Categories = takeCandidates(set.Categories, &remaining)
	set.Tasks = takeCandidates(set.Tasks, &remaining)
	set.Subtasks = takeCandidates(set.Subtasks, &remaining)
	set.Sprints = takeCandidates(set.Sprints, &remaining)
}

func (set CandidateSet) Hash() string {
	payload, _ := json.Marshal(struct {
		ProjectID     string      `json:"project_id"`
		ProjectName   string      `json:"project_name"`
		ProjectStatus string      `json:"project_status"`
		Categories    []Candidate `json:"categories"`
		Tasks         []Candidate `json:"tasks"`
		Subtasks      []Candidate `json:"subtasks"`
		Sprints       []Candidate `json:"sprints"`
		Truncated     bool        `json:"truncated"`
	}{
		ProjectID:     set.ProjectID,
		ProjectName:   set.ProjectName,
		ProjectStatus: set.ProjectStatus,
		Categories:    set.Categories,
		Tasks:         set.Tasks,
		Subtasks:      set.Subtasks,
		Sprints:       set.Sprints,
		Truncated:     set.Truncated,
	})
	return fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
}

func takeCandidates(candidates []Candidate, remaining *int) []Candidate {
	if *remaining <= 0 {
		return nil
	}
	if len(candidates) > *remaining {
		candidates = candidates[:*remaining]
	}
	*remaining -= len(candidates)
	return candidates
}

func sortCandidates(candidates []Candidate) {
	sort.Slice(candidates, func(left, right int) bool {
		return candidates[left].ID < candidates[right].ID
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func boundedText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}
