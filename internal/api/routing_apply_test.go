package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/routing"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestRoutingApplyCreatesSprintFromConfirmedDecisionIdempotently(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "routing-apply.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Routing apply"})
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
	if _, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Existing restart", EstimatedDurationSeconds: 900}); err != nil {
		t.Fatal(err)
	}
	stateHash := routingCandidateStateHash(t, database, project.ProjectID)
	decision := routing.Decision{
		Category: routing.CategoryDecision{
			SelectedID:    category.CategoryID,
			Confidence:    0.96,
			Distribution:  map[string]float64{category.CategoryID: 0.96, "create_new": 0.04},
			PolicyVersion: routing.PolicyVersion,
			DecisionID:    "DEC-routing-apply-test",
		},
		Work: routing.WorkDecision{
			Disposition:              routing.ActionCreateSprint,
			DispositionDistribution:  map[string]float64{routing.ActionCreateSprint: 0.96, routing.ActionNeedsReview: 0.04},
			TargetDistribution:       map[string]float64{task.TaskID: 0.96, "none_of_the_above": 0.04},
			TaskID:                   task.TaskID,
			Confidence:               0.96,
			ProposedName:             "Add bounded restart retry",
			EstimatedDurationSeconds: 1800,
		},
	}
	body, err := json.Marshal(map[string]any{
		"intent":               "Add bounded restart retry",
		"decision_id":          "DEC-routing-apply-test",
		"candidate_state_hash": stateHash,
		"confirmed":            true,
		"model":                "test-fixture",
		"decision":             decision,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.New(database)
	submit := func() routingApplyResponse {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/apply", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "jev:DEC-routing-apply-test")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusCreated && response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var result routingApplyResponse
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}

	first := submit()
	if !first.Applied || first.Duplicate || first.DecisionID != "DEC-routing-apply-test" || first.CandidateStateHash != stateHash || first.QueueID == "" || first.Result == nil || len(first.Result.Sprints) != 1 {
		t.Fatalf("unexpected first Apply result: %+v", first)
	}
	second := submit()
	if !second.Applied || !second.Duplicate || second.QueueID != first.QueueID || second.Result == nil || len(second.Result.Sprints) != 1 {
		t.Fatalf("idempotent replay did not return original registration: %+v", second)
	}
	changedDecision := decision
	changedDecision.Work.ProposedName = "Different retry proposal"
	changedBody, err := json.Marshal(map[string]any{
		"intent":               "Add bounded restart retry",
		"decision_id":          "DEC-routing-apply-test",
		"candidate_state_hash": stateHash,
		"confirmed":            true,
		"model":                "test-fixture",
		"decision":             changedDecision,
	})
	if err != nil {
		t.Fatal(err)
	}
	conflictRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/apply", bytes.NewReader(changedBody))
	conflictRequest.Header.Set("Content-Type", "application/json")
	conflictRequest.Header.Set("Idempotency-Key", "jev:DEC-routing-apply-test")
	conflictResponse := httptest.NewRecorder()
	handler.ServeHTTP(conflictResponse, conflictRequest)
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("changed payload status=%d body=%s, want idempotency conflict", conflictResponse.Code, conflictResponse.Body.String())
	}

	queueItem, err := database.GetRegistrationQueueItem(ctx, project.ProjectID, first.QueueID)
	if err != nil {
		t.Fatal(err)
	}
	if queueItem.Payload == nil || queueItem.Payload.Planning == nil || queueItem.Payload.Planning.RoutingEvidence == nil {
		t.Fatalf("routing evidence not durable in registration record: %+v", queueItem)
	}
	evidence := queueItem.Payload.Planning.RoutingEvidence
	if evidence.DecisionID != first.DecisionID || evidence.CandidateStateHash != stateHash || evidence.Confirmed != true {
		t.Fatalf("routing evidence mismatch: %+v", evidence)
	}
	tree, err := database.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Categories) != 1 || len(tree.Categories[0].Tasks) != 1 || len(tree.Categories[0].Tasks[0].Sprints) != 2 {
		t.Fatalf("expected exactly one new Sprint after idempotent replay: %+v", tree)
	}
}

func TestRoutingApplyRequiresConfirmationAndFreshSnapshot(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "routing-apply-gates.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Routing gates"})
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
	stateHash := routingCandidateStateHash(t, database, project.ProjectID)
	decision := routing.Decision{
		Category: routing.CategoryDecision{
			SelectedID: category.CategoryID, Confidence: 0.96,
			Distribution:  map[string]float64{category.CategoryID: 0.96, "create_new": 0.04},
			PolicyVersion: routing.PolicyVersion, DecisionID: "DEC-routing-gate-test",
		},
		Work: routing.WorkDecision{
			Disposition:             routing.ActionCreateSprint,
			DispositionDistribution: map[string]float64{routing.ActionCreateSprint: 0.96, routing.ActionNeedsReview: 0.04},
			TargetDistribution:      map[string]float64{task.TaskID: 0.96, "none_of_the_above": 0.04},
			TaskID:                  task.TaskID, Confidence: 0.96, ProposedName: "Add retry", EstimatedDurationSeconds: 1800,
		},
	}
	handler := api.New(database)
	submit := func(key, hash string, confirmed bool) *httptest.ResponseRecorder {
		payload, err := json.Marshal(map[string]any{
			"intent": "Add retry", "decision_id": "DEC-routing-gate-test", "candidate_state_hash": hash,
			"confirmed": confirmed, "decision": decision,
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/apply", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", key)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := submit("jev:unconfirmed", stateHash, false); response.Code != http.StatusConflict {
		t.Fatalf("unconfirmed status=%d body=%s", response.Code, response.Body.String())
	}
	if response := submit("jev:stale", "sha256:stale", true); response.Code != http.StatusConflict {
		t.Fatalf("stale hash status=%d body=%s", response.Code, response.Body.String())
	}
	for _, key := range []string{"jev:unconfirmed", "jev:stale"} {
		if _, err := database.GetRegistrationQueueItemByIdempotencyKey(ctx, project.ProjectID, key); err == nil {
			t.Fatalf("gated request %q created a registration queue item", key)
		}
	}
	tree, err := database.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Categories[0].Tasks[0].Sprints) != 0 {
		t.Fatalf("gated Apply created a Sprint: %+v", tree.Categories[0].Tasks[0].Sprints)
	}
}

type routingApplyResponse struct {
	DecisionID         string                       `json:"decision_id"`
	CandidateStateHash string                       `json:"candidate_state_hash"`
	QueueID            string                       `json:"queue_id"`
	RegistrationStatus string                       `json:"registration_status"`
	Applied            bool                         `json:"applied"`
	Duplicate          bool                         `json:"duplicate"`
	Outcome            routing.Outcome              `json:"outcome"`
	Result             *model.ExecutionBatchResult  `json:"result,omitempty"`
	Validation         *model.BatchValidationResult `json:"validation,omitempty"`
}

func TestRoutingApplyCreatesSprintUnderExistingSubtask(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "routing-apply-subtask.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Routing Subtask Apply"})
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
	subtask, err := database.CreateSubtask(ctx, task.TaskID, model.CreateSubtaskInput{Name: "Retry boundaries"})
	if err != nil {
		t.Fatal(err)
	}
	stateHash := routingCandidateStateHash(t, database, project.ProjectID)
	decision := routing.Decision{
		Category: routing.CategoryDecision{SelectedID: category.CategoryID, Confidence: 0.96, Distribution: map[string]float64{category.CategoryID: 0.96, "create_new": 0.04}, PolicyVersion: routing.PolicyVersion, DecisionID: "DEC-routing-subtask-apply"},
		Work: routing.WorkDecision{
			Disposition:             routing.ActionCreateSprint,
			DispositionDistribution: map[string]float64{routing.ActionCreateSprint: 0.96, routing.ActionNeedsReview: 0.04},
			TargetDistribution:      map[string]float64{subtask.SubtaskID: 0.96, "none_of_the_above": 0.04},
			TaskID:                  task.TaskID, SubtaskID: subtask.SubtaskID, Confidence: 0.96, ProposedName: "Exercise retry boundaries", EstimatedDurationSeconds: 1800,
		},
	}
	payload, err := json.Marshal(map[string]any{"intent": "Exercise retry boundaries", "decision_id": decision.Category.DecisionID, "candidate_state_hash": stateHash, "confirmed": true, "decision": decision})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/apply", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "jev:DEC-routing-subtask-apply")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusCreated && response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result routingApplyResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.QueueID == "" || result.Result == nil || len(result.Result.Sprints) != 1 || result.Result.Sprints[0].SubtaskID != subtask.SubtaskID {
		t.Fatalf("Apply did not preserve Subtask ownership: %+v", result)
	}
	tree, err := database.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Categories) != 1 || len(tree.Categories[0].Tasks) != 1 || len(tree.Categories[0].Tasks[0].Sprints) != 0 || len(tree.Categories[0].Tasks[0].Subtasks) != 1 || len(tree.Categories[0].Tasks[0].Subtasks[0].Sprints) != 1 {
		t.Fatalf("Sprint was not created exactly once under its Subtask: %+v", tree)
	}
}

func TestRoutingApplyCreatesTaskAndSprintAtomically(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "routing-apply-task-sprint.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Routing Task Sprint Apply"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	stateHash := routingCandidateStateHash(t, database, project.ProjectID)
	decision := routing.Decision{
		Category: routing.CategoryDecision{SelectedID: category.CategoryID, Confidence: 0.96, Distribution: map[string]float64{category.CategoryID: 0.96, "create_new": 0.04}, PolicyVersion: routing.PolicyVersion, DecisionID: "DEC-routing-task-sprint-apply"},
		Work: routing.WorkDecision{
			Disposition:             routing.ActionCreateTaskAndSprint,
			DispositionDistribution: map[string]float64{routing.ActionCreateTaskAndSprint: 0.96, routing.ActionNeedsReview: 0.04},
			TargetDistribution:      map[string]float64{"none_of_the_above": 0.96, "needs_review": 0.04},
			Confidence:              0.96, ProposedName: "Improve recovery handling", EstimatedDurationSeconds: 1800,
		},
	}
	payload, err := json.Marshal(map[string]any{"intent": "Improve recovery handling", "decision_id": decision.Category.DecisionID, "candidate_state_hash": stateHash, "confirmed": true, "decision": decision})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/apply", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "jev:DEC-routing-task-sprint-apply")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusCreated && response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result routingApplyResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.QueueID == "" || result.Result == nil || len(result.Result.Tasks) != 1 || len(result.Result.Sprints) != 1 {
		t.Fatalf("Apply did not create the new Task and Sprint together: %+v", result)
	}
	tree, err := database.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Categories) != 1 || len(tree.Categories[0].Tasks) != 1 || len(tree.Categories[0].Tasks[0].Sprints) != 1 || tree.Categories[0].Tasks[0].Task.Name != "Improve recovery handling" {
		t.Fatalf("expected exactly one new Task with its Sprint: %+v", tree)
	}
}
