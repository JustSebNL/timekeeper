// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestProjectMessageVectorIndexIsRebuildableAndReturnsRelatedPosts(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)
	projectID := createMessageProject(t, database, "Vector retrieval")

	anchor, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind: model.MessageKindObservation,
		Body: "The queue checkpoint persists state before dispatch.",
		Tags: "queue,checkpoint",
	})
	if err != nil {
		t.Fatalf("create anchor: %v", err)
	}
	related, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind: model.MessageKindLesson,
		Body: "Persisted queue checkpoints keep dispatch recovery durable.",
		Tags: "queue,recovery",
	})
	if err != nil {
		t.Fatalf("create related: %v", err)
	}
	if _, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind: model.MessageKindNote,
		Body: "The dashboard uses a blue accent for category nodes.",
	}); err != nil {
		t.Fatalf("create unrelated: %v", err)
	}

	count, err := database.RebuildProjectMessageVectorIndex(ctx, projectID)
	if err != nil {
		t.Fatalf("rebuild vector index: %v", err)
	}
	if count != 3 {
		t.Fatalf("rebuild count = %d, want 3", count)
	}

	hits, err := database.SearchProjectMessageSemantic(ctx, projectID, anchor.MessageID, "durable queue recovery", 5)
	if err != nil {
		t.Fatalf("semantic search: %v", err)
	}
	if len(hits) == 0 || hits[0].MessageID != related.MessageID || hits[0].SemanticScore <= 0 {
		t.Fatalf("semantic hits = %#v", hits)
	}
	for _, hit := range hits {
		if hit.MessageID == anchor.MessageID {
			t.Fatalf("anchor leaked into semantic results: %#v", hits)
		}
	}

	if _, err := database.UpdateProjectMessage(ctx, projectID, anchor.MessageID, model.UpdateProjectMessageInput{
		Kind: model.MessageKindLesson,
		Body: "Edited vector marker for durable handoff.",
	}); err != nil {
		t.Fatalf("edit anchor: %v", err)
	}
	editedHits, err := database.SearchProjectMessageSemantic(ctx, projectID, related.MessageID, "durable handoff", 5)
	if err != nil {
		t.Fatalf("semantic search after edit: %v", err)
	}
	if len(editedHits) == 0 || editedHits[0].MessageID != anchor.MessageID {
		t.Fatalf("edited semantic hits = %#v", editedHits)
	}
}
