package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func openRegistrationQueueFixture(t *testing.T) (*store.Store, string, string) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "registration-queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Registration queue"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Queued work"})
	if err != nil {
		t.Fatal(err)
	}
	return database, project.ProjectID, category.CategoryID
}

func registrationBatch(categoryID, taskName string) model.ExecutionBatchInput {
	return model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks: []model.ExecutionBatchTask{{
			CategoryID: categoryID,
			Name:       taskName,
			Subtasks: []model.ExecutionBatchSubtask{{
				Name:    taskName + " child",
				Sprints: []model.CreateSprintInput{{Name: taskName + " sprint", EstimatedDurationSeconds: 60}},
			}},
		}},
	}
}

func TestRegistrationQueuePagesFIFOWithoutHidingOlderOffers(t *testing.T) {
	ctx := context.Background()
	database, projectID, categoryID := openRegistrationQueueFixture(t)
	first, _, err := database.EnqueueRegistration(ctx, projectID, model.RegistrationQueueKindExecutionBatch, "page-001", registrationBatch(categoryID, "Page one"))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := database.EnqueueRegistration(ctx, projectID, model.RegistrationQueueKindExecutionBatch, "page-002", registrationBatch(categoryID, "Page two"))
	if err != nil {
		t.Fatal(err)
	}
	firstPage, hasMore, err := database.ListRegistrationQueuePage(ctx, projectID, "", 1, 0)
	if err != nil || len(firstPage) != 1 || firstPage[0].QueueID != first.QueueID || !hasMore {
		t.Fatalf("first registration page=%+v hasMore=%t err=%v", firstPage, hasMore, err)
	}
	secondPage, hasMore, err := database.ListRegistrationQueuePage(ctx, projectID, "", 1, 1)
	if err != nil || len(secondPage) != 1 || secondPage[0].QueueID != second.QueueID || hasMore {
		t.Fatalf("second registration page=%+v hasMore=%t err=%v", secondPage, hasMore, err)
	}
}

func TestRegistrationQueueStacksOffersAndProcessesFIFOAtomically(t *testing.T) {
	ctx := context.Background()
	database, projectID, categoryID := openRegistrationQueueFixture(t)
	firstInput := registrationBatch(categoryID, "First offered Task")
	secondInput := registrationBatch(categoryID, "Second offered Task")

	first, duplicate, err := database.EnqueueRegistration(ctx, projectID, "execution_batch", "offer-001", firstInput)
	if err != nil || duplicate || first.Status != model.RegistrationQueueStatusQueued {
		t.Fatalf("enqueue first=%+v duplicate=%v err=%v", first, duplicate, err)
	}
	firstRetry, duplicate, err := database.EnqueueRegistration(ctx, projectID, "execution_batch", "offer-001", firstInput)
	if err != nil || !duplicate || firstRetry.QueueID != first.QueueID {
		t.Fatalf("idempotent retry=%+v duplicate=%v err=%v", firstRetry, duplicate, err)
	}
	if _, duplicate, err := database.EnqueueRegistration(ctx, projectID, "execution_batch", "offer-001", secondInput); err == nil || duplicate {
		t.Fatalf("same idempotency key with different payload should conflict: duplicate=%v err=%v", duplicate, err)
	}
	second, duplicate, err := database.EnqueueRegistration(ctx, projectID, "execution_batch", "offer-002", secondInput)
	if err != nil || duplicate || second.QueueID == first.QueueID {
		t.Fatalf("enqueue second=%+v duplicate=%v err=%v", second, duplicate, err)
	}

	before, err := database.ListTasks(ctx, projectID)
	if err != nil || len(before) != 0 {
		t.Fatalf("queueing offers must not register Tasks yet: count=%d err=%v", len(before), err)
	}
	queued, err := database.ListRegistrationQueue(ctx, projectID, model.RegistrationQueueStatusQueued, 10)
	if err != nil || len(queued) != 2 || queued[0].QueueID != first.QueueID || queued[1].QueueID != second.QueueID {
		t.Fatalf("queued FIFO listing=%+v err=%v", queued, err)
	}

	processed, found, err := database.ProcessNextRegistrationQueueItem(ctx)
	if err != nil || !found || processed.QueueID != first.QueueID || processed.Status != model.RegistrationQueueStatusRegistered {
		t.Fatalf("process first=%+v found=%v err=%v", processed, found, err)
	}
	if processed.Result == nil || processed.Result.Counts != (model.BatchCounts{Tasks: 1, Subtasks: 1, Sprints: 1, TotalObjects: 3}) {
		t.Fatalf("first registration result=%+v", processed.Result)
	}
	processed, found, err = database.ProcessNextRegistrationQueueItem(ctx)
	if err != nil || !found || processed.QueueID != second.QueueID || processed.Status != model.RegistrationQueueStatusRegistered {
		t.Fatalf("process second=%+v found=%v err=%v", processed, found, err)
	}
	if _, found, err = database.ProcessNextRegistrationQueueItem(ctx); err != nil || found {
		t.Fatalf("empty queue should drain cleanly: found=%v err=%v", found, err)
	}

	tasks, err := database.ListTasks(ctx, projectID)
	if err != nil || len(tasks) != 2 || tasks[0].Name != "First offered Task" || tasks[1].Name != "Second offered Task" {
		t.Fatalf("registered Tasks=%+v err=%v", tasks, err)
	}
	firstAgain, err := database.ProcessRegistrationQueueItem(ctx, first.QueueID)
	if err != nil || firstAgain.Status != model.RegistrationQueueStatusRegistered {
		t.Fatalf("reprocessing completed queue item=%+v err=%v", firstAgain, err)
	}
	tasks, err = database.ListTasks(ctx, projectID)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("completed queue retry duplicated registration: tasks=%d err=%v", len(tasks), err)
	}
}

func TestRegistrationQueueKeepsRejectedOfferVisibleWithoutPartialRegistration(t *testing.T) {
	ctx := context.Background()
	database, projectID, _ := openRegistrationQueueFixture(t)
	input := registrationBatch("C-999999", "Invalid Category Task")
	item, duplicate, err := database.EnqueueRegistration(ctx, projectID, "execution_batch", "invalid-offer", input)
	if err != nil || duplicate {
		t.Fatalf("enqueue invalid offer=%+v duplicate=%v err=%v", item, duplicate, err)
	}
	processed, found, err := database.ProcessNextRegistrationQueueItem(ctx)
	if err != nil || !found || processed.QueueID != item.QueueID || processed.Status != model.RegistrationQueueStatusRejected {
		t.Fatalf("process invalid offer=%+v found=%v err=%v", processed, found, err)
	}
	if processed.Validation == nil || processed.Validation.Valid || len(processed.Validation.Issues) == 0 {
		t.Fatalf("rejected offer diagnostics=%+v", processed.Validation)
	}
	tasks, err := database.ListTasks(ctx, projectID)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("rejected queue item partially registered Tasks=%d err=%v", len(tasks), err)
	}
	listed, err := database.GetRegistrationQueueItem(ctx, projectID, item.QueueID)
	if err != nil || listed.Status != model.RegistrationQueueStatusRejected {
		t.Fatalf("rejected queue item disappeared: %+v err=%v", listed, err)
	}
}

func TestRegistrationQueuePersistsOffersForMissingProject(t *testing.T) {
	ctx := context.Background()
	database, _, _ := openRegistrationQueueFixture(t)
	input := registrationBatch("C-999999", "No Project Task")
	item, duplicate, err := database.EnqueueRegistration(ctx, "P-999999", "execution_batch", "missing-project", input)
	if err != nil || duplicate || item.Status != model.RegistrationQueueStatusQueued {
		t.Fatalf("invalid target offer must still be durably queued for review: item=%+v duplicate=%v err=%v", item, duplicate, err)
	}
	processed, found, err := database.ProcessNextRegistrationQueueItem(ctx)
	if err != nil || !found || processed.Status != model.RegistrationQueueStatusRejected || processed.Validation == nil || processed.Validation.Issues[0].Code != "project_not_found" {
		t.Fatalf("missing Project processing=%+v found=%v err=%v", processed, found, err)
	}
}
