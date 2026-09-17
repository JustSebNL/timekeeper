// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestProjectMessageCheckpointCreatesOneAtomicGroupedHandoff(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)
	projectID := createMessageProject(t, database, "Checkpoint bundle")

	bundle, err := database.CreateProjectMessageCheckpoint(ctx, projectID, model.ProjectMessageCheckpointInput{
		Author: "agent-a",
		Entries: []model.CreateProjectMessageInput{
			{Kind: model.MessageKindDecision, Body: "Persist the queue checkpoint before dispatch.", Tags: "handoff,queue"},
			{Kind: model.MessageKindObservation, Body: "The save boundary is the failure seam."},
			{Kind: model.MessageKindLesson, Body: "Keep only the M-ref in active context."},
		},
	})
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if bundle.Root.Ref == "" || len(bundle.Items) != 3 || bundle.Items[0].MessageID != bundle.Root.MessageID {
		t.Fatalf("checkpoint bundle = %#v", bundle)
	}
	for _, item := range bundle.Items[1:] {
		if item.ParentMessageID == nil || *item.ParentMessageID != bundle.Root.MessageID {
			t.Fatalf("checkpoint grouping = %#v", bundle.Items)
		}
		if item.Author != "agent-a" {
			t.Fatalf("checkpoint author = %#v", item)
		}
	}
	thread, err := database.GetProjectMessageThread(ctx, projectID, bundle.Root.MessageID)
	if err != nil {
		t.Fatalf("read checkpoint thread: %v", err)
	}
	if len(thread) != 3 {
		t.Fatalf("checkpoint thread = %#v", thread)
	}

	_, err = database.CreateProjectMessageCheckpoint(ctx, projectID, model.ProjectMessageCheckpointInput{
		Entries: []model.CreateProjectMessageInput{
			{Kind: model.MessageKindObservation, Body: "valid first"},
			{Kind: model.MessageKindObservation, Body: ""},
		},
	})
	if err == nil {
		t.Fatal("expected invalid checkpoint to fail")
	}
	messages, err := database.ListProjectMessages(ctx, projectID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("invalid checkpoint partially committed: %#v", messages)
	}
}
