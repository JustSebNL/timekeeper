// Copyright (c) 2026 Seb. All rights reserved.

package model_test

import (
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestCalculateExecutionTreeCompletionUsesWeightedCompletedStructure(t *testing.T) {
	tree := model.ExecutionTree{
		Project: model.Project{ProjectID: "P-10000"},
		Categories: []model.ExecutionCategory{{
			Tasks: []model.ExecutionTask{
				{
					Task: model.Task{TaskID: "T-10001", EstimatedDurationSeconds: 1000},
					Sprints: []model.Sprint{
						{SprintID: "SP-10002", Status: "Completed", EstimatedDurationSeconds: 100},
						{SprintID: "SP-10003", Status: "Open", EstimatedDurationSeconds: 300},
					},
					Subtasks: []model.ExecutionSubtask{{
						Subtask: model.Subtask{SubtaskID: "ST-10004", Status: "Open", EstimatedDurationSeconds: 600},
						Sprints: []model.Sprint{{SprintID: "SP-10005", Status: "Completed", EstimatedDurationSeconds: 600}},
					}},
				},
				{Task: model.Task{TaskID: "T-10006", Status: "Completed", EstimatedDurationSeconds: 100}},
			},
		}},
	}

	model.CalculateExecutionTreeCompletion(&tree)

	first := tree.Categories[0].Tasks[0]
	if got := first.Subtasks[0].Subtask.CalculatedCompletionPct; got != 100 {
		t.Fatalf("subtask completion=%v, want 100", got)
	}
	if got := first.Task.CalculatedCompletionPct; got != 70 {
		t.Fatalf("task completion=%v, want 70", got)
	}
	if got := tree.Categories[0].Tasks[1].Task.CalculatedCompletionPct; got != 100 {
		t.Fatalf("leaf task completion=%v, want 100", got)
	}
	if got := tree.Categories[0].Category.ProgressPct; got != 72.7 {
		t.Fatalf("category completion=%v, want 72.7", got)
	}
	if got := tree.Project.CalculatedCompletionPct; got != 72.7 {
		t.Fatalf("project completion=%v, want 72.7", got)
	}
}
