// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestProjectMessagePinningIsDurableAndRecallPrioritized(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)
	projectID := createMessageProject(t, database, "Pinned memory")

	pinned, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind: model.MessageKindDecision,
		Body: "The durable checkpoint is the important queue decision.",
		Tags: "queue",
	})
	if err != nil {
		t.Fatalf("create pinned candidate: %v", err)
	}
	unpinned, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind: model.MessageKindObservation,
		Body: "A newer queue observation is less important than the decision.",
		Tags: "queue",
	})
	if err != nil {
		t.Fatalf("create unpinned candidate: %v", err)
	}

	updated, err := database.SetProjectMessagePinned(ctx, projectID, pinned.MessageID, true)
	if err != nil {
		t.Fatalf("pin message: %v", err)
	}
	if !updated.Pinned || updated.Revision != pinned.Revision {
		t.Fatalf("pinned message = %#v", updated)
	}
	idempotent, err := database.SetProjectMessagePinned(ctx, projectID, pinned.MessageID, true)
	if err != nil {
		t.Fatalf("repeat pin: %v", err)
	}
	if !idempotent.Pinned {
		t.Fatalf("repeat pin cleared state: %#v", idempotent)
	}

	listed, err := database.ListProjectMessages(ctx, projectID, "", 2)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(listed) != 2 || listed[0].MessageID != pinned.MessageID || listed[1].MessageID != unpinned.MessageID {
		t.Fatalf("pin-aware list = %#v", listed)
	}
	bundle, err := database.GetProjectMessageContext(ctx, projectID, unpinned.MessageID, model.ProjectMessageContextOptions{Query: "queue", RelatedLimit: 1})
	if err != nil {
		t.Fatalf("context pinned recall: %v", err)
	}
	if len(bundle.Related) != 1 || bundle.Related[0].MessageID != pinned.MessageID || !bundle.Related[0].Pinned {
		t.Fatalf("pin-aware related = %#v", bundle.Related)
	}

	if _, err := database.SetProjectMessagePinned(ctx, projectID, pinned.MessageID, false); err != nil {
		t.Fatalf("unpin message: %v", err)
	}
	reloaded, err := database.GetProjectMessage(ctx, projectID, pinned.MessageID)
	if err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if reloaded.Pinned {
		t.Fatalf("unpin did not persist: %#v", reloaded)
	}
	events, err := database.ListProjectEvents(ctx, projectID)
	if err != nil {
		t.Fatalf("list pin events: %v", err)
	}
	if len(events) != 4 || events[0].EventType != "message_unpinned" || events[1].EventType != "message_pinned" {
		t.Fatalf("pin events = %#v", events)
	}
}
