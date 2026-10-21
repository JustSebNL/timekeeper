// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package routing

import (
	"context"
	"errors"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const ProtocolVersion = "timekeeper-routing/v1"

var ErrProviderUnavailable = errors.New("routing provider unavailable")

type ProviderStatus struct {
	Available       bool   `json:"available"`
	Provider        string `json:"provider,omitempty"`
	ProtocolVersion string `json:"protocol_version,omitempty"`
}

type Request struct {
	Intent     string       `json:"intent"`
	AgentID    string       `json:"agent_id,omitempty"`
	SessionID  string       `json:"session_id,omitempty"`
	Candidates CandidateSet `json:"candidates"`
}

type Decision struct {
	Category CategoryDecision `json:"category"`
	Work     WorkDecision     `json:"work,omitempty"`
}

type Provider interface {
	Status(context.Context) (ProviderStatus, error)
	Route(context.Context, Request) (Decision, error)
}

type Service struct {
	Provider Provider
	Limits   Limits
}

func (service *Service) Route(ctx context.Context, tree model.ExecutionTree, request Request) (Outcome, error) {
	if service == nil || service.Provider == nil {
		return Outcome{}, ErrProviderUnavailable
	}
	set := CandidateSetFromTree(tree, service.Limits)
	if set.ProjectID == "" || set.ProjectStatus != "Open" || set.Truncated {
		return EvaluateCategory(set, CategoryDecision{SelectedID: ActionNeedsReview}), nil
	}
	limits := service.Limits
	if limits.MaxIntentRunes <= 0 {
		limits.MaxIntentRunes = DefaultLimits().MaxIntentRunes
	}
	if limits.MaxActorRunes <= 0 {
		limits.MaxActorRunes = DefaultLimits().MaxActorRunes
	}
	request.Intent = boundedText(request.Intent, limits.MaxIntentRunes)
	request.AgentID = boundedText(request.AgentID, limits.MaxActorRunes)
	request.SessionID = boundedText(request.SessionID, limits.MaxActorRunes)
	if request.Intent == "" {
		return EvaluateCategory(set, CategoryDecision{}), nil
	}
	request.Candidates = set

	status, err := service.Provider.Status(ctx)
	if err != nil {
		return Outcome{}, err
	}
	if !status.Available || status.ProtocolVersion != ProtocolVersion {
		return reviewOutcome(Outcome{
			Version:            DecisionVersion,
			PolicyVersion:      PolicyVersion,
			ProjectID:          set.ProjectID,
			CandidateStateHash: set.StateHash,
		}, ReasonProviderUnavailable, "The optional routing provider is not available for this protocol."), nil
	}

	decision, err := service.Provider.Route(ctx, request)
	if err != nil {
		return Outcome{}, err
	}
	if decision.Category.PolicyVersion != PolicyVersion {
		return reviewOutcome(Outcome{
			Version:            DecisionVersion,
			PolicyVersion:      PolicyVersion,
			ProjectID:          set.ProjectID,
			CandidateStateHash: set.StateHash,
			DecisionID:         decision.Category.DecisionID,
		}, ReasonProviderVersionMismatch, "The routing provider used an unsupported policy version."), nil
	}

	category := EvaluateCategory(set, decision.Category)
	if category.Outcome != OutcomeProposed || category.Action != ActionExistingCategory {
		return category, nil
	}
	return EvaluateWork(set, category.CategoryID, decision.Work), nil
}
