package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/routing"
)

func TestRoutingBatchRejectsSnapshotDriftAtQueueProcessing(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "routing-snapshot-drift.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Routing drift"})
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
	tree, err := database.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	candidateHash := routing.CandidateSetFromTree(tree, routing.DefaultLimits()).StateHash
	input := model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Extensions: []model.ExecutionBatchExtension{{
			TaskID:  task.TaskID,
			Sprints: []model.CreateSprintInput{{Name: "Add retry", EstimatedDurationSeconds: 1800}},
		}},
		Planning: &model.ExecutionBatchPlanning{RoutingEvidence: &model.ExecutionBatchRoutingEvidence{
			DecisionID:         "DEC-stale-queue",
			PolicyVersion:      routing.PolicyVersion,
			CandidateStateHash: candidateHash,
			IntentHash:         "sha256:intent",
			Confirmed:          true,
			Decision:           json.RawMessage(`{"category":{"selected_id":"C"}}`),
		}},
	}
	queued, duplicate, err := database.EnqueueRegistration(ctx, project.ProjectID, model.RegistrationQueueKindExecutionBatch, "jev:DEC-stale-queue", input)
	if err != nil || duplicate {
		t.Fatalf("enqueue item=%+v duplicate=%t err=%v", queued, duplicate, err)
	}
	if _, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "New concurrent category"}); err != nil {
		t.Fatal(err)
	}
	processed, err := database.ProcessRegistrationQueueItem(ctx, queued.QueueID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Status != model.RegistrationQueueStatusRejected || processed.Validation == nil {
		t.Fatalf("stale routing batch was not rejected: %+v", processed)
	}
	found := false
	for _, issue := range processed.Validation.Issues {
		if issue.Code == "routing_candidate_state_changed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("stale-state validation issue missing: %+v", processed.Validation)
	}
	finalTree, err := database.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalTree.Categories[0].Tasks[0].Sprints) != 0 {
		t.Fatalf("stale routing batch created a Sprint: %+v", finalTree.Categories[0].Tasks[0].Sprints)
	}
}
