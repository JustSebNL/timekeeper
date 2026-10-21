// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/routing"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestRoutingDecisionAPIValidatesTypedProposalWithoutMutation(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Routing proposal"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Guardian"})
	if err != nil {
		t.Fatal(err)
	}
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Add telemetry", EstimatedDurationSeconds: 900})
	if err != nil {
		t.Fatal(err)
	}
	candidateStateHash := routingCandidateStateHash(t, database, project.ProjectID)

	beforeEvents, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	decision := routing.Decision{
		Category: routing.CategoryDecision{
			SelectedID:    category.CategoryID,
			Confidence:    0.91,
			Distribution:  map[string]float64{category.CategoryID: 0.91, "create_new": 0.09},
			PolicyVersion: routing.PolicyVersion,
			DecisionID:    "DEC-test-read-only",
		},
		Work: routing.WorkDecision{
			Disposition:             routing.ActionResumeExisting,
			DispositionDistribution: map[string]float64{routing.ActionResumeExisting: 0.94, routing.ActionNeedsReview: 0.06},
			TargetDistribution:      map[string]float64{sprint.SprintID: 0.94, "none_of_the_above": 0.06},
			TaskID:                  task.TaskID,
			SprintID:                sprint.SprintID,
			Confidence:              0.94,
		},
	}
	payload, err := json.Marshal(map[string]any{"intent": "continue Guardian telemetry", "candidate_state_hash": candidateStateHash, "decision": decision})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/decide", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var outcome routing.Outcome
	if err := json.NewDecoder(response.Body).Decode(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.Outcome != routing.OutcomeProposed || outcome.Action != routing.ActionResumeExisting || outcome.ProjectID != project.ProjectID || outcome.CategoryID != category.CategoryID || outcome.TaskID != task.TaskID || outcome.SprintID != sprint.SprintID {
		t.Fatalf("unexpected evaluated proposal: %#v", outcome)
	}
	if !strings.HasPrefix(outcome.CandidateStateHash, "sha256:") {
		t.Fatalf("missing authoritative candidate-state hash: %#v", outcome)
	}
	afterEvents, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("read-only proposal evaluation wrote events: before=%d after=%d", len(beforeEvents), len(afterEvents))
	}
	afterTree, err := database.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTree.Categories) != 1 || len(afterTree.Categories[0].Tasks) != 1 || len(afterTree.Categories[0].Tasks[0].Sprints) != 1 || afterTree.Categories[0].Tasks[0].Sprints[0].Status != "Open" {
		t.Fatalf("proposal evaluation changed the authoritative hierarchy: %#v", afterTree)
	}
}

func TestRoutingDecisionAPIRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	handler := api.New(database)
	for name, body := range map[string]string{
		"client-supplied candidates": `{"intent":"route","decision":{},"candidates":[]}`,
		"trailing object":            `{"intent":"route","decision":{}} {}`,
		"missing candidate hash":     `{"intent":"route","decision":{}}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/P-10000/routing/decide", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s status=%d body=%s", name, response.Code, response.Body.String())
		}
	}
}

func TestRoutingDecisionAPIRejectsCrossProjectCategoryCandidate(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Target project"})
	if err != nil {
		t.Fatal(err)
	}
	otherProject, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Other project"})
	if err != nil {
		t.Fatal(err)
	}
	otherCategory, err := database.CreateCategory(ctx, otherProject.ProjectID, model.CreateCategoryInput{Name: "Foreign category"})
	if err != nil {
		t.Fatal(err)
	}
	candidateStateHash := routingCandidateStateHash(t, database, project.ProjectID)
	decision := routing.Decision{Category: routing.CategoryDecision{
		SelectedID:    otherCategory.CategoryID,
		Confidence:    0.99,
		Distribution:  map[string]float64{otherCategory.CategoryID: 0.99, "create_new": 0.01},
		PolicyVersion: routing.PolicyVersion,
	}}
	payload, err := json.Marshal(map[string]any{"intent": "route", "candidate_state_hash": candidateStateHash, "decision": decision})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/decide", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var outcome routing.Outcome
	if err := json.NewDecoder(response.Body).Decode(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.Outcome != routing.OutcomeNeedsReview || outcome.ReasonCode != routing.ReasonInvalidProbabilityDistribution {
		t.Fatalf("foreign Project candidate should be rejected for review, got %#v", outcome)
	}
}

func TestRoutingCategoryAPIValidatesFirstPassWithoutWorkDecision(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Category pass"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	decision := routing.CategoryDecision{
		SelectedID:    category.CategoryID,
		Confidence:    0.95,
		Distribution:  map[string]float64{category.CategoryID: 0.95, "create_new": 0.05},
		PolicyVersion: routing.PolicyVersion,
	}
	candidateStateHash := routingCandidateStateHash(t, database, project.ProjectID)
	beforeEvents, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"intent": "repair runtime", "candidate_state_hash": candidateStateHash, "decision": decision})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/category", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var outcome routing.Outcome
	if err := json.NewDecoder(response.Body).Decode(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.Outcome != routing.OutcomeProposed || outcome.Action != routing.ActionExistingCategory || outcome.CategoryID != category.CategoryID {
		t.Fatalf("unexpected first-pass Category result: %#v", outcome)
	}
	afterEvents, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("read-only Category evaluation wrote events: before=%d after=%d", len(beforeEvents), len(afterEvents))
	}
}

func TestRoutingCategoryAPIRejectsMalformedOrStaleInput(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Category input"})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.New(database)
	for name, testCase := range map[string]struct {
		body       string
		wantStatus int
	}{
		"unknown field":          {`{"intent":"route","decision":{},"extra":true}`, http.StatusBadRequest},
		"trailing JSON":          {`{"intent":"route","decision":{}} {}`, http.StatusBadRequest},
		"missing intent":         {`{"decision":{}}`, http.StatusBadRequest},
		"missing candidate hash": {`{"intent":"route","decision":{"policy_version":"jev-routing-policy/v1"}}`, http.StatusBadRequest},
		"stale policy":           {`{"intent":"route","decision":{"policy_version":"old"}}`, http.StatusConflict},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/category", strings.NewReader(testCase.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != testCase.wantStatus {
			t.Errorf("%s status=%d want=%d body=%s", name, response.Code, testCase.wantStatus, response.Body.String())
		}
	}
}
