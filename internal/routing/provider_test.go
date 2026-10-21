// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package routing

import (
	"context"
	"errors"
	"testing"
)

type fakeProvider struct {
	status   ProviderStatus
	decision Decision
	err      error
	called   bool
	request  Request
}

func (provider *fakeProvider) Status(context.Context) (ProviderStatus, error) {
	if provider.err != nil {
		return ProviderStatus{}, provider.err
	}
	return provider.status, nil
}

func (provider *fakeProvider) Route(_ context.Context, request Request) (Decision, error) {
	provider.called = true
	provider.request = request
	return provider.decision, provider.err
}

func TestServiceRoutesProviderDecisionThroughPolicy(t *testing.T) {
	provider := &fakeProvider{
		status: ProviderStatus{Available: true, Provider: "fake.jev", ProtocolVersion: ProtocolVersion},
		decision: Decision{
			Category: CategoryDecision{SelectedID: "C-10001", Confidence: 0.91, Distribution: map[string]float64{"C-10001": 0.91, "create_new": 0.09}, DecisionID: "DEC-7", PolicyVersion: PolicyVersion},
			Work:     withWorkChoices(WorkDecision{Disposition: ActionResumeExisting, SprintID: "SP-10003", Confidence: 0.94}, "SP-10003"),
		},
	}
	service := &Service{Provider: provider, Limits: DefaultLimits()}

	outcome, err := service.Route(context.Background(), testTree(), Request{Intent: "continue Guardian telemetry", AgentID: "agent-a"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !provider.called || provider.request.Candidates.ProjectID != "P-10000" {
		t.Fatalf("provider did not receive bounded candidates: %#v", provider.request)
	}
	if outcome.Outcome != OutcomeProposed || outcome.Action != ActionResumeExisting || outcome.SprintID != "SP-10003" {
		t.Fatalf("unexpected route outcome: %#v", outcome)
	}
	if outcome.DecisionID != "" {
		t.Fatalf("work outcome unexpectedly inherited category decision id: %#v", outcome)
	}
}

func TestServiceReportsUnavailableProviderWithoutRouting(t *testing.T) {
	provider := &fakeProvider{status: ProviderStatus{Available: false, ProtocolVersion: ProtocolVersion}}
	service := &Service{Provider: provider, Limits: DefaultLimits()}

	outcome, err := service.Route(context.Background(), testTree(), Request{Intent: "continue"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if provider.called {
		t.Fatal("unavailable provider was asked for a route")
	}
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonProviderUnavailable {
		t.Fatalf("unexpected unavailable outcome: %#v", outcome)
	}
}

func TestServiceRequiresDeclaredVersions(t *testing.T) {
	unversionedProvider := &fakeProvider{status: ProviderStatus{Available: true}}
	outcome, err := (&Service{Provider: unversionedProvider, Limits: DefaultLimits()}).Route(context.Background(), testTree(), Request{Intent: "continue"})
	if err != nil {
		t.Fatalf("Route with unversioned provider: %v", err)
	}
	if unversionedProvider.called || outcome.ReasonCode != ReasonProviderUnavailable {
		t.Fatalf("unexpected unversioned-provider outcome: %#v", outcome)
	}

	unversionedDecision := &fakeProvider{
		status:   ProviderStatus{Available: true, ProtocolVersion: ProtocolVersion},
		decision: Decision{Category: CategoryDecision{SelectedID: "C-10001", Confidence: 0.95}},
	}
	outcome, err = (&Service{Provider: unversionedDecision, Limits: DefaultLimits()}).Route(context.Background(), testTree(), Request{Intent: "continue"})
	if err != nil {
		t.Fatalf("Route with unversioned decision: %v", err)
	}
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonProviderVersionMismatch {
		t.Fatalf("unexpected unversioned-decision outcome: %#v", outcome)
	}
}

func TestServiceRejectsMismatchedPolicyVersion(t *testing.T) {
	provider := &fakeProvider{
		status: ProviderStatus{Available: true, ProtocolVersion: ProtocolVersion},
		decision: Decision{Category: CategoryDecision{
			SelectedID:    "C-10001",
			Confidence:    0.95,
			PolicyVersion: "jev-routing-policy/v0",
		}},
	}
	service := &Service{Provider: provider, Limits: DefaultLimits()}

	outcome, err := service.Route(context.Background(), testTree(), Request{Intent: "continue"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonProviderVersionMismatch {
		t.Fatalf("unexpected version outcome: %#v", outcome)
	}
}

func TestServiceBoundsRequestMetadata(t *testing.T) {
	provider := &fakeProvider{
		status: ProviderStatus{Available: true, ProtocolVersion: ProtocolVersion},
		decision: Decision{
			Category: CategoryDecision{SelectedID: "C-10001", Confidence: 0.95, Distribution: map[string]float64{"C-10001": 0.95, "create_new": 0.05}, PolicyVersion: PolicyVersion},
			Work:     withWorkChoices(WorkDecision{Disposition: ActionResumeExisting, SprintID: "SP-10003", Confidence: 0.95}, "SP-10003"),
		},
	}
	limits := DefaultLimits()
	limits.MaxIntentRunes = 8
	limits.MaxActorRunes = 4

	_, err := (&Service{Provider: provider, Limits: limits}).Route(context.Background(), testTree(), Request{
		Intent:    "  123456789  ",
		AgentID:   "agent-id-too-long",
		SessionID: "session-id-too-long",
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if provider.request.Intent != "12345678" || provider.request.AgentID != "agen" || provider.request.SessionID != "sess" {
		t.Fatalf("request metadata was not bounded: %#v", provider.request)
	}
}

func TestServiceRejectsEmptyIntentWithoutProviderCall(t *testing.T) {
	provider := &fakeProvider{status: ProviderStatus{Available: true, ProtocolVersion: ProtocolVersion}}
	outcome, err := (&Service{Provider: provider, Limits: DefaultLimits()}).Route(context.Background(), testTree(), Request{Intent: "   "})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if provider.called {
		t.Fatal("provider was called with an empty intent")
	}
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonMissingAnswer {
		t.Fatalf("unexpected empty-intent outcome: %#v", outcome)
	}
}

func TestServiceDoesNotCallProviderForUnsafeCandidates(t *testing.T) {
	provider := &fakeProvider{status: ProviderStatus{Available: true, ProtocolVersion: ProtocolVersion}}
	limits := DefaultLimits()
	limits.MaxCandidates = 3

	outcome, err := (&Service{Provider: provider, Limits: limits}).Route(context.Background(), testTree(), Request{Intent: "continue"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if provider.called {
		t.Fatal("provider was called with a truncated candidate set")
	}
	if outcome.Outcome != OutcomeNeedsReview || outcome.ReasonCode != ReasonStaleCandidate {
		t.Fatalf("unexpected truncated outcome: %#v", outcome)
	}
}

func TestServiceRequiresProviderAndPropagatesFailures(t *testing.T) {
	if _, err := (&Service{}).Route(context.Background(), testTree(), Request{}); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("missing provider error = %v, want %v", err, ErrProviderUnavailable)
	}

	expected := errors.New("status failed")
	provider := &fakeProvider{err: expected}
	_, err := (&Service{Provider: provider, Limits: DefaultLimits()}).Route(context.Background(), testTree(), Request{Intent: "continue"})
	if !errors.Is(err, expected) {
		t.Fatalf("status error = %v, want %v", err, expected)
	}
}
