// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package routing

import (
	"fmt"
	"math"
	"strings"
)

const (
	ActionExistingCategory               = "existing_category"
	ActionCreateNewCategory              = "create_new_category"
	ActionResumeExisting                 = "resume_existing"
	ActionCreateSprint                   = "create_sprint"
	ActionCreateTaskAndSprint            = "create_task_and_sprint"
	ActionHoldForReview                  = "hold_for_review"
	ActionNeedsReview                    = "needs_review"
	OutcomeProposed                      = "proposed"
	OutcomeNeedsReview                   = "needs_review"
	ReasonAmbiguousCategory              = "ambiguous_category"
	ReasonConfirmationRequired           = "confirmation_required"
	ReasonConflictingAnswers             = "conflicting_answers"
	ReasonInvalidConfidence              = "invalid_confidence"
	ReasonInvalidProbabilityDistribution = "invalid_probability_distribution"
	ReasonMissingAnswer                  = "missing_answer"
	ReasonNoCategoryFit                  = "no_category_fit"
	ReasonNoSprintTarget                 = "no_sprint_target"
	ReasonParentNotOpen                  = "parent_not_open"
	ReasonProjectNotFound                = "project_not_found"
	ReasonProviderUnavailable            = "provider_unavailable"
	ReasonProviderVersionMismatch        = "provider_version_mismatch"
	ReasonSprintCapacity                 = "sprint_capacity_exhausted"
	ReasonSprintNotRunnable              = "sprint_not_runnable"
	ReasonSprintEstimateOutOfBounds      = "sprint_estimate_out_of_bounds"
	ReasonStaleCandidate                 = "stale_candidate"
)

const (
	MinSprintEstimateSeconds         int64 = 15 * 60
	MaxSprintEstimateSeconds         int64 = 60 * 60
	probabilityDistributionTolerance       = 0.01
)

type CategoryDecision struct {
	SelectedID     string             `json:"selected_id"`
	Confidence     float64            `json:"confidence"`
	Distribution   map[string]float64 `json:"distribution,omitempty"`
	SuggestedName  string             `json:"suggested_name,omitempty"`
	SuggestedGoal  string             `json:"suggested_goal,omitempty"`
	RequiresNew    *bool              `json:"requires_new,omitempty"`
	Confirmed      bool               `json:"confirmed,omitempty"`
	IntentHash     string             `json:"intent_hash,omitempty"`
	ShadowBoxRunID string             `json:"shadowbox_run_id,omitempty"`
	PolicyVersion  string             `json:"policy_version,omitempty"`
	DecidedBy      string             `json:"decided_by,omitempty"`
	DecisionID     string             `json:"decision_id,omitempty"`
}

type WorkDecision struct {
	Disposition              string             `json:"disposition"`
	DispositionDistribution  map[string]float64 `json:"disposition_distribution,omitempty"`
	TargetDistribution       map[string]float64 `json:"target_distribution,omitempty"`
	TaskID                   string             `json:"task_id,omitempty"`
	SubtaskID                string             `json:"subtask_id,omitempty"`
	SprintID                 string             `json:"sprint_id,omitempty"`
	Confidence               float64            `json:"confidence"`
	ProposedName             string             `json:"proposed_name,omitempty"`
	EstimatedDurationSeconds int64              `json:"estimated_duration_seconds,omitempty"`
	Confirmed                bool               `json:"confirmed,omitempty"`
}

type Outcome struct {
	Version              string  `json:"version"`
	PolicyVersion        string  `json:"policy_version"`
	Outcome              string  `json:"outcome"`
	Action               string  `json:"action"`
	ProjectID            string  `json:"project_id,omitempty"`
	CategoryID           string  `json:"category_id,omitempty"`
	TaskID               string  `json:"task_id,omitempty"`
	SubtaskID            string  `json:"subtask_id,omitempty"`
	SprintID             string  `json:"sprint_id,omitempty"`
	ProposedName         string  `json:"proposed_name,omitempty"`
	ProposedGoal         string  `json:"proposed_goal,omitempty"`
	ReasonCode           string  `json:"reason_code,omitempty"`
	Reason               string  `json:"reason,omitempty"`
	RequiresConfirmation bool    `json:"requires_confirmation,omitempty"`
	Confidence           float64 `json:"confidence,omitempty"`
	CandidateStateHash   string  `json:"candidate_state_hash,omitempty"`
	DecisionID           string  `json:"decision_id,omitempty"`
}

func EvaluateCategory(set CandidateSet, decision CategoryDecision) Outcome {
	base := Outcome{
		Version:            DecisionVersion,
		PolicyVersion:      PolicyVersion,
		ProjectID:          set.ProjectID,
		CandidateStateHash: set.StateHash,
		DecisionID:         decision.DecisionID,
	}
	if set.ProjectID == "" {
		return reviewOutcome(base, ReasonProjectNotFound, "The decision has no authoritative Project.")
	}
	if set.ProjectStatus != "Open" {
		return reviewOutcome(base, ReasonParentNotOpen, "The selected Project is not Open.")
	}
	if set.Truncated {
		return reviewOutcome(base, ReasonStaleCandidate, "The bounded candidate set was truncated; refresh or narrow it before routing.")
	}
	selected := strings.TrimSpace(decision.SelectedID)
	if selected == "" {
		return reviewOutcome(base, ReasonMissingAnswer, "Jev did not select a Category or an explicit review outcome.")
	}
	if selected == ActionNeedsReview || selected == "hold_for_review" {
		return reviewOutcome(base, ReasonNoCategoryFit, "Jev requested human review instead of selecting a Category.")
	}
	if selected == "none_of_the_above" {
		return reviewOutcome(base, ReasonNoCategoryFit, "No existing Category was selected.")
	}
	if decision.RequiresNew != nil && *decision.RequiresNew && selected != "create_new" {
		return reviewOutcome(base, ReasonConflictingAnswers, "The answers selected an existing Category while also requiring a new Category.")
	}
	if !validConfidence(decision.Confidence) {
		return reviewOutcome(base, ReasonInvalidConfidence, "The Category confidence must be a finite value from 0 through 1.")
	}
	if !validCategoryDistribution(set, selected, decision.Distribution) {
		return reviewOutcome(base, ReasonInvalidProbabilityDistribution, "The Category probability distribution is missing, malformed, or outside the bounded candidate set.")
	}
	if selected == "create_new" {
		if strings.TrimSpace(decision.SuggestedName) == "" {
			return reviewOutcome(base, ReasonMissingAnswer, "A new Category proposal requires a suggested name.")
		}
		if decision.Confidence < 0.90 {
			return reviewOutcome(base, ReasonNoCategoryFit, "The new-Category confidence is below the review threshold.")
		}
		if strongestExistingCategory(decision.Distribution) > 0.50 {
			return reviewOutcome(base, ReasonAmbiguousCategory, "An existing Category remains plausible; the new Category requires review.")
		}
		base.Outcome = OutcomeProposed
		base.Action = ActionCreateNewCategory
		base.ProposedName = strings.TrimSpace(decision.SuggestedName)
		base.ProposedGoal = strings.TrimSpace(decision.SuggestedGoal)
		base.RequiresConfirmation = true
		base.Confidence = decision.Confidence
		return base
	}
	category, found := findCandidate(set.Categories, selected)
	if !found {
		return reviewOutcome(base, ReasonStaleCandidate, "The selected Category is not in the authoritative Project candidates.")
	}
	if category.Status != "Open" {
		base.CategoryID = category.ID
		return reviewOutcome(base, ReasonParentNotOpen, "The selected Category is not Open.")
	}
	if decision.Confidence < 0.85 {
		base.CategoryID = category.ID
		return reviewOutcome(base, ReasonAmbiguousCategory, "The existing-Category confidence is below the routing threshold.")
	}
	if tiedWithSelected(decision.Distribution, selected) {
		base.CategoryID = category.ID
		return reviewOutcome(base, ReasonAmbiguousCategory, "Another Category has an equal probability.")
	}
	base.Outcome = OutcomeProposed
	base.Action = ActionExistingCategory
	base.CategoryID = category.ID
	base.Confidence = decision.Confidence
	return base
}

func EvaluateWork(set CandidateSet, categoryID string, decision WorkDecision) Outcome {
	base := Outcome{
		Version:            DecisionVersion,
		PolicyVersion:      PolicyVersion,
		ProjectID:          set.ProjectID,
		CategoryID:         categoryID,
		CandidateStateHash: set.StateHash,
	}
	category, found := findCandidate(set.Categories, categoryID)
	if !found {
		return reviewOutcome(base, ReasonStaleCandidate, "The routed Category is not in the authoritative Project candidates.")
	}
	if set.ProjectStatus != "Open" || category.Status != "Open" {
		return reviewOutcome(base, ReasonParentNotOpen, "The Project or selected Category is not Open.")
	}
	if set.Truncated {
		return reviewOutcome(base, ReasonStaleCandidate, "The bounded candidate set was truncated; refresh or narrow it before routing.")
	}
	disposition := strings.TrimSpace(decision.Disposition)
	if disposition == ActionNeedsReview || disposition == ActionHoldForReview {
		return reviewOutcome(base, ReasonMissingAnswer, "The work decision requested human review.")
	}
	if !validConfidence(decision.Confidence) {
		return reviewOutcome(base, ReasonInvalidConfidence, "The work confidence must be a finite value from 0 through 1.")
	}
	if !validChoiceDistribution(decision.DispositionDistribution, disposition, workDispositionOptions()) {
		return reviewOutcome(base, ReasonInvalidProbabilityDistribution, "The work-disposition probability distribution is missing or invalid.")
	}
	if decision.Confidence < 0.85 {
		return reviewOutcome(base, ReasonConflictingAnswers, "The work-routing confidence is below the review threshold.")
	}
	switch disposition {
	case ActionResumeExisting:
		if decision.SprintID == "" {
			return reviewOutcome(base, ReasonNoSprintTarget, "Resuming work requires an existing Sprint ID.")
		}
		if !validCandidateChoice(set.Sprints, decision.TargetDistribution, decision.SprintID) {
			return reviewOutcome(base, ReasonInvalidProbabilityDistribution, "The Sprint-target probability distribution is missing or invalid.")
		}
		sprint, ok := findCandidate(set.Sprints, decision.SprintID)
		if !ok || !candidateInCategory(set, sprint, categoryID) {
			base.SprintID = decision.SprintID
			return reviewOutcome(base, ReasonStaleCandidate, "The selected Sprint is stale or outside the routed Category.")
		}
		taskID, subtaskID := sprintOwners(set, sprint)
		if (decision.TaskID != "" && decision.TaskID != taskID) || (decision.SubtaskID != "" && decision.SubtaskID != subtaskID) {
			return reviewOutcome(base, ReasonConflictingAnswers, "The selected Sprint conflicts with the separately supplied Task or Subtask owner.")
		}
		if !sprint.Runnable {
			base.SprintID = sprint.ID
			return reviewOutcome(base, ReasonSprintNotRunnable, "The selected Sprint is not currently runnable.")
		}
		base.Outcome = OutcomeProposed
		base.Action = ActionResumeExisting
		base.SprintID = sprint.ID
		base.TaskID, base.SubtaskID = taskID, subtaskID
		base.Confidence = decision.Confidence
		return base
	case ActionCreateSprint:
		if decision.SprintID != "" {
			return reviewOutcome(base, ReasonConflictingAnswers, "Creating a Sprint cannot also select an existing Sprint ID.")
		}
		if strings.TrimSpace(decision.ProposedName) == "" {
			return reviewOutcome(base, ReasonMissingAnswer, "Creating a Sprint requires a non-empty proposed name.")
		}
		if !validSprintEstimate(decision.EstimatedDurationSeconds) {
			return reviewOutcome(base, ReasonSprintEstimateOutOfBounds, "A proposed Sprint estimate must be between 15 and 60 minutes.")
		}
		ownerID := firstNonEmpty(decision.SubtaskID, decision.TaskID)
		if ownerID == "" {
			return reviewOutcome(base, ReasonNoSprintTarget, "Creating a Sprint requires an existing Task or Subtask owner.")
		}
		if !validOwnerChoice(set, decision.TargetDistribution, ownerID) {
			return reviewOutcome(base, ReasonInvalidProbabilityDistribution, "The Sprint-owner probability distribution is missing or invalid.")
		}
		owner, ok := ownerCandidate(set, ownerID)
		if !ok || !candidateInCategory(set, owner, categoryID) {
			return reviewOutcome(base, ReasonStaleCandidate, "The Sprint owner is stale or outside the routed Category.")
		}
		if owner.Kind == "subtask" && decision.TaskID != "" && decision.TaskID != owner.ParentID {
			return reviewOutcome(base, ReasonConflictingAnswers, "The selected Subtask does not belong to the separately supplied Task.")
		}
		if owner.Status != "Open" {
			return reviewOutcome(base, ReasonParentNotOpen, "The Sprint owner is not Open.")
		}
		if owner.SprintCount >= 4 {
			return reviewOutcome(base, ReasonSprintCapacity, "The selected owner already has four direct Sprints.")
		}
		if decision.Confidence < 0.90 {
			return reviewOutcome(base, ReasonConflictingAnswers, "Creating a Sprint requires at least 0.90 confidence.")
		}
		if !decision.Confirmed {
			return reviewOutcome(base, ReasonConfirmationRequired, "Creating a Sprint requires human confirmation in this stage.")
		}
		base.Outcome = OutcomeProposed
		base.Action = ActionCreateSprint
		if owner.Kind == "subtask" {
			base.SubtaskID = owner.ID
			base.TaskID = owner.ParentID
		} else {
			base.TaskID = owner.ID
		}
		base.ProposedName = strings.TrimSpace(decision.ProposedName)
		base.Confidence = decision.Confidence
		return base
	case ActionCreateTaskAndSprint:
		if decision.TaskID != "" || decision.SubtaskID != "" || decision.SprintID != "" {
			return reviewOutcome(base, ReasonConflictingAnswers, "Creating a new Task and Sprint cannot also select an existing Task, Subtask, or Sprint.")
		}
		if !validCreationTargetChoice(set, decision.TargetDistribution) {
			return reviewOutcome(base, ReasonInvalidProbabilityDistribution, "The target distribution must select none_of_the_above before proposing a new Task.")
		}
		if strings.TrimSpace(decision.ProposedName) == "" {
			return reviewOutcome(base, ReasonMissingAnswer, "Creating a Task and Sprint requires a proposed Task name.")
		}
		if !validSprintEstimate(decision.EstimatedDurationSeconds) {
			return reviewOutcome(base, ReasonSprintEstimateOutOfBounds, "A proposed Sprint estimate must be between 15 and 60 minutes.")
		}
		if decision.Confidence < 0.90 {
			return reviewOutcome(base, ReasonConflictingAnswers, "Creating a Task and Sprint requires at least 0.90 confidence.")
		}
		if !decision.Confirmed {
			return reviewOutcome(base, ReasonConfirmationRequired, "Creating a Task and Sprint requires human confirmation.")
		}
		base.Outcome = OutcomeProposed
		base.Action = ActionCreateTaskAndSprint
		base.ProposedName = strings.TrimSpace(decision.ProposedName)
		base.Confidence = decision.Confidence
		return base
	default:
		return reviewOutcome(base, ReasonMissingAnswer, "The work disposition is missing or unsupported.")
	}
}

func validSprintEstimate(seconds int64) bool {
	return seconds >= MinSprintEstimateSeconds && seconds <= MaxSprintEstimateSeconds
}

func validConfidence(confidence float64) bool {
	return !math.IsNaN(confidence) && !math.IsInf(confidence, 0) && confidence >= 0 && confidence <= 1
}

func validCategoryDistribution(set CandidateSet, selected string, distribution map[string]float64) bool {
	allowed := map[string]struct{}{
		"create_new":        {},
		"none_of_the_above": {},
		"needs_review":      {},
	}
	for _, category := range set.Categories {
		allowed[category.ID] = struct{}{}
	}
	return validChoiceDistribution(distribution, selected, allowed)
}

func validChoiceDistribution(distribution map[string]float64, selected string, allowed map[string]struct{}) bool {
	if len(distribution) == 0 {
		return false
	}
	selectedProbability, selectedPresent := distribution[selected]
	if !selectedPresent {
		return false
	}
	total := 0.0
	for id, probability := range distribution {
		if _, ok := allowed[id]; !ok || math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 || probability > 1 {
			return false
		}
		if id != selected && probability >= selectedProbability-0.000001 {
			return false
		}
		total += probability
	}
	return !math.IsNaN(total) && math.Abs(total-1) <= probabilityDistributionTolerance
}

func validCandidateChoice(candidates []Candidate, distribution map[string]float64, selected string) bool {
	allowed := map[string]struct{}{"none_of_the_above": {}, "needs_review": {}}
	for _, candidate := range candidates {
		allowed[candidate.ID] = struct{}{}
	}
	return validChoiceDistribution(distribution, selected, allowed)
}

func validOwnerChoice(set CandidateSet, distribution map[string]float64, selected string) bool {
	allowed := map[string]struct{}{"none_of_the_above": {}, "needs_review": {}}
	for _, candidate := range set.Tasks {
		allowed[candidate.ID] = struct{}{}
	}
	for _, candidate := range set.Subtasks {
		allowed[candidate.ID] = struct{}{}
	}
	return validChoiceDistribution(distribution, selected, allowed)
}

func validCreationTargetChoice(set CandidateSet, distribution map[string]float64) bool {
	allowed := map[string]struct{}{"none_of_the_above": {}, "needs_review": {}}
	for _, candidate := range set.Tasks {
		allowed[candidate.ID] = struct{}{}
	}
	for _, candidate := range set.Subtasks {
		allowed[candidate.ID] = struct{}{}
	}
	for _, candidate := range set.Sprints {
		allowed[candidate.ID] = struct{}{}
	}
	return validChoiceDistribution(distribution, "none_of_the_above", allowed)
}

func workDispositionOptions() map[string]struct{} {
	return map[string]struct{}{
		ActionResumeExisting:      {},
		ActionCreateSprint:        {},
		ActionCreateTaskAndSprint: {},
		ActionNeedsReview:         {},
		ActionHoldForReview:       {},
	}
}

func reviewOutcome(base Outcome, code, reason string) Outcome {
	base.Outcome = OutcomeNeedsReview
	base.Action = ActionNeedsReview
	base.ReasonCode = code
	base.Reason = reason
	return base
}

func findCandidate(candidates []Candidate, id string) (Candidate, bool) {
	for _, candidate := range candidates {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Candidate{}, false
}

func strongestExistingCategory(distribution map[string]float64) float64 {
	strongest := 0.0
	for id, probability := range distribution {
		if id == "create_new" || id == "none_of_the_above" || id == "needs_review" {
			continue
		}
		if probability > strongest {
			strongest = probability
		}
	}
	return strongest
}

func tiedWithSelected(distribution map[string]float64, selected string) bool {
	selectedProbability, ok := distribution[selected]
	if !ok {
		return false
	}
	for id, probability := range distribution {
		if id == selected || id == "create_new" || id == "none_of_the_above" || id == "needs_review" {
			continue
		}
		if math.Abs(probability-selectedProbability) < 0.000001 {
			return true
		}
	}
	return false
}

func ownerCandidate(set CandidateSet, id string) (Candidate, bool) {
	if task, ok := findCandidate(set.Tasks, id); ok {
		return task, true
	}
	return findCandidate(set.Subtasks, id)
}

func candidateInCategory(set CandidateSet, candidate Candidate, categoryID string) bool {
	switch candidate.Kind {
	case "task":
		return candidate.ParentID == categoryID
	case "subtask":
		task, ok := findCandidate(set.Tasks, candidate.ParentID)
		return ok && task.ParentID == categoryID
	case "sprint":
		owner, ok := ownerCandidate(set, candidate.ParentID)
		if !ok {
			return false
		}
		if owner.Kind == "task" {
			return owner.ParentID == categoryID
		}
		task, ok := findCandidate(set.Tasks, owner.ParentID)
		return ok && task.ParentID == categoryID
	default:
		return false
	}
}

func sprintOwners(set CandidateSet, sprint Candidate) (string, string) {
	owner, ok := ownerCandidate(set, sprint.ParentID)
	if !ok {
		return "", ""
	}
	if owner.Kind == "subtask" {
		return owner.ParentID, owner.ID
	}
	return owner.ID, ""
}

func (outcome Outcome) String() string {
	if outcome.ReasonCode != "" {
		return fmt.Sprintf("%s:%s", outcome.Action, outcome.ReasonCode)
	}
	return outcome.Action
}
