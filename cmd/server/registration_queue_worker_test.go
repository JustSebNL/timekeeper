package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestRegistrationQueueWorkerDrainsPersistedOffers(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "queue-worker.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Queue worker"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Worker"})
	if err != nil {
		t.Fatal(err)
	}
	input := model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks: []model.ExecutionBatchTask{{
			CategoryID: category.CategoryID,
			Name:       "Worker registration",
			Sprints:    []model.CreateSprintInput{{Name: "Runnable Sprint", EstimatedDurationSeconds: 60}},
		}},
	}
	queued, _, err := database.EnqueueRegistration(ctx, project.ProjectID, model.RegistrationQueueKindExecutionBatch, "worker-offer", input)
	if err != nil {
		t.Fatal(err)
	}
	before, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil || len(before) != 0 {
		t.Fatalf("offer should remain queued before worker run: tasks=%d err=%v", len(before), err)
	}

	if err := processRegistrationQueueOnce(ctx, database, nil); err != nil {
		t.Fatal(err)
	}
	item, err := database.GetRegistrationQueueItem(ctx, project.ProjectID, queued.QueueID)
	if err != nil || item.Status != model.RegistrationQueueStatusRegistered || item.Result == nil {
		t.Fatalf("worker result=%+v err=%v", item, err)
	}
	after, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil || len(after) != 1 || after[0].Name != "Worker registration" {
		t.Fatalf("registered Task readback=%+v err=%v", after, err)
	}
}

func TestRegistrationQueueWorkerRecoversPendingOffersOnStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	database, err := store.Open(filepath.Join(t.TempDir(), "queue-worker-startup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Queue startup recovery"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(context.Background(), project.ProjectID, model.CreateCategoryInput{Name: "Worker"})
	if err != nil {
		t.Fatal(err)
	}
	queued, _, err := database.EnqueueRegistration(context.Background(), project.ProjectID, model.RegistrationQueueKindTaskBatch, "startup-offer", model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks:   []model.ExecutionBatchTask{{CategoryID: category.CategoryID, Name: "Recovered offer"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stopWorker := startRegistrationQueueWorker(ctx, database, nil)
	defer stopWorker()

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		item, err := database.GetRegistrationQueueItem(context.Background(), project.ProjectID, queued.QueueID)
		if err != nil {
			t.Fatal(err)
		}
		if item.Status == model.RegistrationQueueStatusRegistered {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("startup worker left accepted offer queued: %+v", item)
		case <-ticker.C:
		}
	}
}
