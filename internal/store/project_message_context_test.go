// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestProjectMessageContextHydrationIsBoundedAndPointerFriendly(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)
	projectID := createMessageProject(t, database, "Context hydration")

	anchor, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind:   model.MessageKindObservation,
		Author: "agent-a",
		Body:   "The alpha queue boundary needs a durable checkpoint.",
		Tags:   "alpha,queue",
	})
	if err != nil {
		t.Fatalf("create anchor: %v", err)
	}
	if _, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind:            model.MessageKindAnswer,
		Body:            "First alpha reply.",
		ParentMessageID: &anchor.MessageID,
	}); err != nil {
		t.Fatalf("create first reply: %v", err)
	}
	if _, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind:            model.MessageKindAnswer,
		Body:            "Second alpha reply.",
		ParentMessageID: &anchor.MessageID,
	}); err != nil {
		t.Fatalf("create second reply: %v", err)
	}
	for _, body := range []string{"Related alpha finding one.", "Related alpha finding two.", "Related alpha finding three."} {
		if _, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
			Kind: model.MessageKindLesson,
			Body: body,
			Tags: "alpha",
		}); err != nil {
			t.Fatalf("create related message: %v", err)
		}
	}
	if _, err := database.UpdateProjectMessage(ctx, projectID, anchor.MessageID, model.UpdateProjectMessageInput{
		Kind:   model.MessageKindDecision,
		Author: "agent-a",
		Body:   "The alpha queue checkpoint is mandatory.",
		Tags:   "alpha,queue",
	}); err != nil {
		t.Fatalf("edit anchor: %v", err)
	}

	bundle, err := database.GetProjectMessageContext(ctx, projectID, anchor.MessageID, model.ProjectMessageContextOptions{
		ThreadLimit:   1,
		RelatedLimit:  2,
		RevisionLimit: 1,
	})
	if err != nil {
		t.Fatalf("hydrate context: %v", err)
	}
	if bundle.Anchor.MessageID != anchor.MessageID || bundle.Anchor.Revision != 2 {
		t.Fatalf("anchor = %#v", bundle.Anchor)
	}
	if len(bundle.Thread) != 1 || bundle.Thread[0].MessageID != anchor.MessageID || !bundle.ThreadTruncated {
		t.Fatalf("thread = %#v truncated=%t", bundle.Thread, bundle.ThreadTruncated)
	}
	if len(bundle.Related) != 2 || !bundle.RelatedTruncated {
		t.Fatalf("related = %#v truncated=%t", bundle.Related, bundle.RelatedTruncated)
	}
	for _, hit := range bundle.Related {
		if hit.MessageID == anchor.MessageID {
			t.Fatalf("anchor leaked into related results: %#v", bundle.Related)
		}
	}
	if len(bundle.Revisions) != 1 || bundle.Revisions[0].Revision != 2 || !bundle.RevisionsTruncated {
		t.Fatalf("revisions = %#v truncated=%t", bundle.Revisions, bundle.RevisionsTruncated)
	}
	if bundle.RelatedQuery == "" || bundle.RelatedMode != "vector" {
		t.Fatalf("related retrieval = query=%q mode=%q", bundle.RelatedQuery, bundle.RelatedMode)
	}
}
