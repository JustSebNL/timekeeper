// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package routing

import (
	"math"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func testTree() model.ExecutionTree {
	return model.ExecutionTree{
		Project: model.Project{ProjectID: "P-10000", ProjectName: "TimeKeeper", Status: "Open"},
		Categories: []model.ExecutionCategory{{
			Category: model.Category{CategoryID: "C-10001", ProjectID: "P-10000", Name: "Runtime", Goal: "Core service work", Status: "Open"},
			Tasks: []model.ExecutionTask{{
				Task: model.Task{TaskID: "T-10002", ProjectID: "P-10000", CategoryID: "C-10001", Name: "Guardian", Status: "Open"},
				Sprints: []model.Sprint{{
					SprintID:   "SP-10003",
					ProjectID:  "P-10000",
					CategoryID: "C-10001",
					TaskID:     "T-10002",
					Name:       "Add telemetry",
					Status:     "Open",
				}},
				Subtasks: []model.ExecutionSubtask{{
					Subtask: model.Subtask{SubtaskID: "ST-10004", ProjectID: "P-10000", CategoryID: "C-10001", TaskID: "T-10002", Name: "Compat", Status: "Open"},
					Sprints: []model.Sprint{{
						SprintID:   "SP-10005",
						ProjectID:  "P-10000",
						CategoryID: "C-10001",
						TaskID:     "T-10002",
						SubtaskID:  "ST-10004",
						Name:       "Legacy path",
						Status:     "On Hold",
					}},
				}},
			}},
		}},
	}
}

func withWorkChoices(decision WorkDecision, target string) WorkDecision {
	decision.DispositionDistribution = map[string]float64{decision.Disposition: 0.95, ActionNeedsReview: 0.05}
	if target == "" {
		decision.TargetDistribution = map[string]float64{"none_of_the_above": 0.95, "T-10002": 0.05}
	} else {
		decision.TargetDistribution = map[string]float64{target: 0.95, "none_of_the_above": 0.05}
	}
	return decision
}

func TestCandidateSetFromTreeBoundsAndPreservesParentage(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	if set.ProjectID != "P-10000" || len(set.Categories) != 1 || len(set.Tasks) != 1 || len(set.Subtasks) != 1 || len(set.Sprints) != 2 {
		t.Fatalf("unexpected candidate set: %#v", set)
	}
	if set.Tasks[0].ParentID != "C-10001" || set.Tasks[0].SprintCount != 1 || set.Subtasks[0].SprintCount != 1 {
		t.Fatalf("candidate parentage/capacity not preserved: %#v %#v", set.Tasks[0], set.Subtasks[0])
	}
	if !set.Sprints[0].Runnable && !set.Sprints[1].Runnable {
		t.Fatalf("no runnable sprint marked: %#v", set.Sprints)
	}
	if set.Sprints[0].ID != "SP-10003" || !set.Sprints[0].Runnable {
		t.Fatalf("expected open direct sprint to be runnable first: %#v", set.Sprints)
	}
	if set.Sprints[1].ID != "SP-10005" || set.Sprints[1].Runnable {
		t.Fatalf("held subtask sprint should not be runnable: %#v", set.Sprints)
	}
	if !strings.HasPrefix(set.StateHash, "sha256:") {
		t.Fatalf("missing candidate hash: %#v", set)
	}
}

func TestCandidateStateHashIncludesProjectDisplayName(t *testing.T) {
	firstTree := testTree()
	first := CandidateSetFromTree(firstTree, DefaultLimits())
	if first.ProjectName != "TimeKeeper" {
		t.Fatalf("ProjectName = %q, want %q", first.ProjectName, "TimeKeeper")
	}
	secondTree := firstTree
	secondTree.Project.ProjectName = "Renamed Project"
	second := CandidateSetFromTree(secondTree, DefaultLimits())
	if first.StateHash == second.StateHash {
		t.Fatal("candidate state hash ignored ProjectName shown to the model")
	}
}

func TestCandidateSetMarksTruncatedInput(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxCandidates = 3
	set := CandidateSetFromTree(testTree(), limits)
	if !set.Truncated || len(set.Sprints) != 0 {
		t.Fatalf("expected bounded truncated set: %#v", set)
	}
}

func TestEvaluateCategoryReturnsExistingAndNewProposals(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	existing := EvaluateCategory(set, CategoryDecision{
		SelectedID:   "C-10001",
		Confidence:   0.91,
		Distribution: map[string]float64{"C-10001": 0.91, "create_new": 0.09},
		DecisionID:   "DEC-1",
	})
	if existing.Outcome != OutcomeProposed || existing.Action != ActionExistingCategory || existing.CategoryID != "C-10001" {
		t.Fatalf("unexpected existing category outcome: %#v", existing)
	}

	requiresNew := true
	conflict := EvaluateCategory(set, CategoryDecision{SelectedID: "C-10001", Confidence: 0.95, RequiresNew: &requiresNew})
	if conflict.Outcome != OutcomeNeedsReview || conflict.ReasonCode != ReasonConflictingAnswers {
		t.Fatalf("expected contradiction review: %#v", conflict)
	}

	created := EvaluateCategory(set, CategoryDecision{
		SelectedID:    "create_new",
		Confidence:    0.92,
		Distribution:  map[string]float64{"create_new": 0.92, "C-10001": 0.08},
		SuggestedName: "Diagnostics",
		SuggestedGoal: "Crash diagnostics",
	})
	if created.Outcome != OutcomeProposed || created.Action != ActionCreateNewCategory || !created.RequiresConfirmation {
		t.Fatalf("unexpected new category proposal: %#v", created)
	}
}

func TestEvaluateWorkRoutesOnlyRunnableSprint(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	resume := EvaluateWork(set, "C-10001", withWorkChoices(WorkDecision{
		Disposition: ActionResumeExisting,
		SprintID:    "SP-10003",
		Confidence:  0.90,
	}, "SP-10003"))
	if resume.Outcome != OutcomeProposed || resume.SprintID != "SP-10003" || resume.TaskID != "T-10002" || resume.SubtaskID != "" {
		t.Fatalf("unexpected resume proposal: %#v", resume)
	}

	held := EvaluateWork(set, "C-10001", withWorkChoices(WorkDecision{Disposition: ActionResumeExisting, SprintID: "SP-10005", Confidence: 0.99}, "SP-10005"))
	if held.Outcome != OutcomeNeedsReview || held.ReasonCode != ReasonSprintNotRunnable {
		t.Fatalf("expected held sprint review: %#v", held)
	}
}

func TestEvaluateWorkRequiresConfirmationAndCapacity(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	unconfirmed := EvaluateWork(set, "C-10001", withWorkChoices(WorkDecision{
		Disposition:              ActionCreateSprint,
		TaskID:                   "T-10002",
		Confidence:               0.95,
		ProposedName:             "Verify telemetry",
		EstimatedDurationSeconds: 1800,
	}, "T-10002"))
	if unconfirmed.Outcome != OutcomeNeedsReview || unconfirmed.ReasonCode != ReasonConfirmationRequired {
		t.Fatalf("expected confirmation review: %#v", unconfirmed)
	}

	set.Tasks[0].SprintCount = 4
	capacity := EvaluateWork(set, "C-10001", withWorkChoices(WorkDecision{
		Disposition:              ActionCreateSprint,
		TaskID:                   "T-10002",
		Confidence:               0.99,
		ProposedName:             "Another bounded sprint",
		EstimatedDurationSeconds: 1800,
		Confirmed:                true,
	}, "T-10002"))
	if capacity.Outcome != OutcomeNeedsReview || capacity.ReasonCode != ReasonSprintCapacity {
		t.Fatalf("expected capacity review: %#v", capacity)
	}
}

func TestEvaluateWorkCanProposeTaskAndSprintWithConfirmation(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	outcome := EvaluateWork(set, "C-10001", withWorkChoices(WorkDecision{
		Disposition:              ActionCreateTaskAndSprint,
		Confidence:               0.94,
		ProposedName:             "Instrument Guardian registrations",
		EstimatedDurationSeconds: 3600,
		Confirmed:                true,
	}, ""))
	if outcome.Outcome != OutcomeProposed || outcome.Action != ActionCreateTaskAndSprint || outcome.ProposedName == "" {
		t.Fatalf("unexpected task/sprint proposal: %#v", outcome)
	}
}

func TestEvaluateWorkRejectsSprintEstimateOutsideJevBounds(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	for _, estimate := range []int64{899, 3601} {
		outcome := EvaluateWork(set, "C-10001", withWorkChoices(WorkDecision{
			Disposition:              ActionCreateSprint,
			TaskID:                   "T-10002",
			Confidence:               0.95,
			ProposedName:             "Bounded work slice",
			EstimatedDurationSeconds: estimate,
			Confirmed:                true,
		}, "T-10002"))
		if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != "sprint_estimate_out_of_bounds" {
			t.Fatalf("estimate %d should require review, got %#v", estimate, outcome)
		}
	}
}

func TestEvaluateWorkAcceptsSprintEstimateAtJevBounds(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	for _, estimate := range []int64{900, 3600} {
		outcome := EvaluateWork(set, "C-10001", withWorkChoices(WorkDecision{
			Disposition:              ActionCreateSprint,
			TaskID:                   "T-10002",
			Confidence:               0.95,
			ProposedName:             "Bounded work slice",
			EstimatedDurationSeconds: estimate,
			Confirmed:                true,
		}, "T-10002"))
		if outcome.Outcome != OutcomeProposed || outcome.Action != ActionCreateSprint {
			t.Fatalf("estimate %d should be proposed, got %#v", estimate, outcome)
		}
	}
}

func TestEvaluateWorkRejectsOversizedTaskAndSprintEstimate(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	outcome := EvaluateWork(set, "C-10001", withWorkChoices(WorkDecision{
		Disposition:              ActionCreateTaskAndSprint,
		Confidence:               0.95,
		ProposedName:             "New bounded task",
		EstimatedDurationSeconds: 3601,
		Confirmed:                true,
	}, ""))
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != "sprint_estimate_out_of_bounds" {
		t.Fatalf("oversized Task/Sprint estimate should require review, got %#v", outcome)
	}
}

func TestEvaluateWorkRequiresSprintName(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	outcome := EvaluateWork(set, "C-10001", withWorkChoices(WorkDecision{
		Disposition:              ActionCreateSprint,
		TaskID:                   "T-10002",
		Confidence:               0.95,
		ProposedName:             "  ",
		EstimatedDurationSeconds: 1800,
		Confirmed:                true,
	}, "T-10002"))
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonMissingAnswer {
		t.Fatalf("unnamed Sprint should require review, got %#v", outcome)
	}
}

func TestEvaluateCategoryRejectsInvalidConfidence(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	for name, confidence := range map[string]float64{
		"negative":  -0.01,
		"above one": 1.01,
		"nan":       math.NaN(),
		"infinity":  math.Inf(1),
	} {
		outcome := EvaluateCategory(set, CategoryDecision{SelectedID: "C-10001", Confidence: confidence})
		if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != "invalid_confidence" {
			t.Errorf("%s confidence should require review, got %#v", name, outcome)
		}
	}
}

func TestEvaluateWorkRejectsInvalidConfidence(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	for name, confidence := range map[string]float64{
		"negative":  -0.01,
		"above one": 1.01,
		"nan":       math.NaN(),
		"infinity":  math.Inf(1),
	} {
		outcome := EvaluateWork(set, "C-10001", WorkDecision{
			Disposition: ActionResumeExisting,
			SprintID:    "SP-10003",
			Confidence:  confidence,
		})
		if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonInvalidConfidence {
			t.Errorf("%s confidence should require review, got %#v", name, outcome)
		}
	}
}

func TestEvaluateCategoryRejectsMalformedProbabilityDistribution(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	cases := map[string]map[string]float64{
		"missing":              nil,
		"sum below one":        {"C-10001": 0.8, "create_new": 0.1},
		"out of range":         {"C-10001": 1.1, "create_new": -0.1},
		"nan probability":      {"C-10001": math.NaN(), "create_new": 0},
		"unknown candidate":    {"C-10001": 0.9, "C-99999": 0.1},
		"selected missing":     {"create_new": 1},
		"selected not highest": {"C-10001": 0.4, "create_new": 0.6},
	}
	for name, distribution := range cases {
		outcome := EvaluateCategory(set, CategoryDecision{
			SelectedID:   "C-10001",
			Confidence:   0.95,
			Distribution: distribution,
		})
		if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != "invalid_probability_distribution" {
			t.Errorf("%s distribution should require review, got %#v", name, outcome)
		}
	}
}

func TestEvaluateCategoryAcceptsDistributionWithinTolerance(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	outcome := EvaluateCategory(set, CategoryDecision{
		SelectedID:   "C-10001",
		Confidence:   0.905,
		Distribution: map[string]float64{"C-10001": 0.905, "create_new": 0.1},
	})
	if outcome.Outcome != OutcomeProposed || outcome.Action != ActionExistingCategory {
		t.Fatalf("slight floating-point sum drift should be accepted, got %#v", outcome)
	}
}

func TestEvaluateWorkRequiresDispositionDistribution(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	outcome := EvaluateWork(set, "C-10001", WorkDecision{
		Disposition: ActionResumeExisting,
		SprintID:    "SP-10003",
		Confidence:  0.95,
	})
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonInvalidProbabilityDistribution {
		t.Fatalf("missing work-choice distribution should require review, got %#v", outcome)
	}
}

func TestEvaluateWorkRejectsInvalidTargetDistribution(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	decision := withWorkChoices(WorkDecision{
		Disposition: ActionResumeExisting,
		SprintID:    "SP-10003",
		Confidence:  0.95,
	}, "SP-10003")
	decision.TargetDistribution = map[string]float64{"SP-10005": 0.95, "none_of_the_above": 0.05}
	outcome := EvaluateWork(set, "C-10001", decision)
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonInvalidProbabilityDistribution {
		t.Fatalf("target distribution selecting a different Sprint should require review, got %#v", outcome)
	}
}

func TestEvaluateWorkRequiresTargetDistribution(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	decision := WorkDecision{
		Disposition:             ActionResumeExisting,
		DispositionDistribution: map[string]float64{ActionResumeExisting: 0.95, ActionNeedsReview: 0.05},
		SprintID:                "SP-10003",
		Confidence:              0.95,
	}
	outcome := EvaluateWork(set, "C-10001", decision)
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonInvalidProbabilityDistribution {
		t.Fatalf("missing target-choice distribution should require review, got %#v", outcome)
	}
}

func TestEvaluateWorkRejectsUnknownDispositionChoice(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	decision := withWorkChoices(WorkDecision{
		Disposition: ActionResumeExisting,
		SprintID:    "SP-10003",
		Confidence:  0.95,
	}, "SP-10003")
	decision.DispositionDistribution = map[string]float64{ActionResumeExisting: 0.95, "invented_action": 0.05}
	outcome := EvaluateWork(set, "C-10001", decision)
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonInvalidProbabilityDistribution {
		t.Fatalf("unknown disposition option should require review, got %#v", outcome)
	}
}

func TestEvaluateWorkRejectsMismatchedTaskAndSubtaskOwners(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	set.Tasks = append(set.Tasks, Candidate{ID: "T-10006", Kind: "task", ParentID: "C-10001", Name: "Other task", Status: "Open"})
	decision := withWorkChoices(WorkDecision{
		Disposition:              ActionCreateSprint,
		TaskID:                   "T-10006",
		SubtaskID:                "ST-10004",
		Confidence:               0.95,
		ProposedName:             "New bounded sprint",
		EstimatedDurationSeconds: 1800,
		Confirmed:                true,
	}, "ST-10004")
	outcome := EvaluateWork(set, "C-10001", decision)
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonConflictingAnswers {
		t.Fatalf("mismatched Task/Subtask owners should require review, got %#v", outcome)
	}
}

func TestEvaluateWorkRejectsMismatchedResumeOwner(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	set.Tasks = append(set.Tasks, Candidate{ID: "T-10006", Kind: "task", ParentID: "C-10001", Name: "Other task", Status: "Open"})
	decision := withWorkChoices(WorkDecision{
		Disposition: ActionResumeExisting,
		TaskID:      "T-10006",
		SprintID:    "SP-10003",
		Confidence:  0.95,
	}, "SP-10003")
	outcome := EvaluateWork(set, "C-10001", decision)
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonConflictingAnswers {
		t.Fatalf("mismatched resume owner should require review, got %#v", outcome)
	}
}

func TestEvaluateWorkRejectsExistingIDsForNewTaskProposal(t *testing.T) {
	set := CandidateSetFromTree(testTree(), DefaultLimits())
	decision := withWorkChoices(WorkDecision{
		Disposition:              ActionCreateTaskAndSprint,
		TaskID:                   "T-10002",
		Confidence:               0.95,
		ProposedName:             "New bounded task",
		EstimatedDurationSeconds: 1800,
		Confirmed:                true,
	}, "")
	outcome := EvaluateWork(set, "C-10001", decision)
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonConflictingAnswers {
		t.Fatalf("new Task/Sprint proposal with an existing Task ID should require review, got %#v", outcome)
	}
}
