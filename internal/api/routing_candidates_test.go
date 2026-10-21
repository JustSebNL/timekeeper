package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/routing"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestRoutingCandidateSnapshotMatchesCorePolicySetWithoutMutation(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "routing-candidates.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Candidate snapshot"})
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
	if _, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Restart service", EstimatedDurationSeconds: 1800}); err != nil {
		t.Fatal(err)
	}
	before, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := database.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	want := routing.CandidateSetFromTree(tree, routing.DefaultLimits())

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/routing/candidates", nil)
	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var got routing.CandidateSet
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate snapshot mismatch:\n got=%#v\nwant=%#v", got, want)
	}
	if got.StateHash == "" || got.Truncated {
		t.Fatalf("candidate snapshot missing stable hash or unexpectedly truncated: %#v", got)
	}
	after, err := database.ListProjectEvents(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("candidate snapshot wrote events: before=%d after=%d", len(before), len(after))
	}
}
