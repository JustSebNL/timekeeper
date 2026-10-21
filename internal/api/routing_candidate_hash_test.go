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

func TestRoutingCategoryAPIRejectsStaleSnapshotHash(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "stale-category.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Stale category"})
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
	payload, err := json.Marshal(map[string]any{"intent": "repair runtime", "candidate_state_hash": "sha256:stale", "decision": decision})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/category", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want stale-snapshot conflict", response.Code, response.Body.String())
	}
}

func TestRoutingDecisionAPIRejectsStaleSnapshotHash(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "stale-decision.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Stale decision"})
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
	sprint, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Restart service", EstimatedDurationSeconds: 900})
	if err != nil {
		t.Fatal(err)
	}
	decision := routing.Decision{
		Category: routing.CategoryDecision{
			SelectedID:    category.CategoryID,
			Confidence:    0.95,
			Distribution:  map[string]float64{category.CategoryID: 0.95, "create_new": 0.05},
			PolicyVersion: routing.PolicyVersion,
		},
		Work: routing.WorkDecision{
			Disposition:             routing.ActionResumeExisting,
			DispositionDistribution: map[string]float64{routing.ActionResumeExisting: 0.95, routing.ActionNeedsReview: 0.05},
			TargetDistribution:      map[string]float64{sprint.SprintID: 0.95, "none_of_the_above": 0.05},
			TaskID:                  task.TaskID,
			SprintID:                sprint.SprintID,
			Confidence:              0.95,
		},
	}
	payload, err := json.Marshal(map[string]any{"intent": "continue guardian", "candidate_state_hash": "sha256:stale", "decision": decision})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/routing/decide", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want stale-snapshot conflict", response.Code, response.Body.String())
	}
}

func routingCandidateStateHash(t *testing.T, database *store.Store, projectID string) string {
	t.Helper()
	tree, err := database.ProjectExecutionTree(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	return routing.CandidateSetFromTree(tree, routing.DefaultLimits()).StateHash
}
