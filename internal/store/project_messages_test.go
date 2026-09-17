// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func openMessageStore(t *testing.T) *store.Store {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func createMessageProject(t *testing.T, database *store.Store, name string) string {
	t.Helper()
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: name})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return project.ProjectID
}

func TestProjectMessagesSupportRepliesEditsAndRevisionHistory(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)
	projectID := createMessageProject(t, database, "Trail of Thought")

	root, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind:   model.MessageKindObservation,
		Author: "agent-a",
		Body:   "The queue stalls after Sprint promotion; inspect the save boundary.",
		Tags:   "queue,debug",
	})
	if err != nil {
		t.Fatalf("create root message: %v", err)
	}
	if root.MessageID < 1 || root.Ref != "M-"+formatMessageID(root.MessageID) {
		t.Fatalf("root reference = %#v", root)
	}
	if root.Revision != 1 || root.ParentMessageID != nil {
		t.Fatalf("root lineage = %#v", root)
	}

	reply, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{
		Kind:            model.MessageKindDecision,
		Author:          "agent-a",
		Body:            "Confirmed: trimQueue raised before processSprint ran.",
		ParentMessageID: &root.MessageID,
	})
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if reply.ParentMessageID == nil || *reply.ParentMessageID != root.MessageID {
		t.Fatalf("reply parent = %#v", reply.ParentMessageID)
	}

	updated, err := database.UpdateProjectMessage(ctx, projectID, root.MessageID, model.UpdateProjectMessageInput{
		Kind:   model.MessageKindLesson,
		Author: "agent-b",
		Body:   "Lesson: persist state before promoting a Sprint, and make save failures visible.",
		Tags:   "queue,lesson",
	})
	if err != nil {
		t.Fatalf("update message: %v", err)
	}
	if updated.Revision != 2 || updated.Kind != model.MessageKindLesson || updated.Author != "agent-b" {
		t.Fatalf("updated message = %#v", updated)
	}
	if !updated.UpdatedAt.After(root.UpdatedAt) && updated.UpdatedAt.Equal(root.UpdatedAt) {
		t.Fatalf("updated timestamp did not change: root=%v updated=%v", root.UpdatedAt, updated.UpdatedAt)
	}

	revisions, err := database.ListProjectMessageRevisions(ctx, projectID, root.MessageID)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revisions) != 2 || revisions[0].Revision != 2 || revisions[1].Revision != 1 {
		t.Fatalf("revisions = %#v", revisions)
	}
	if revisions[1].Body != root.Body || revisions[0].Body != updated.Body {
		t.Fatalf("revision bodies = %#v", revisions)
	}

	thread, err := database.GetProjectMessageThread(ctx, projectID, root.MessageID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if len(thread) != 2 || thread[0].MessageID != root.MessageID || thread[1].MessageID != reply.MessageID {
		t.Fatalf("thread = %#v", thread)
	}
	events, err := database.ListProjectEvents(ctx, projectID)
	if err != nil {
		t.Fatalf("list message events: %v", err)
	}
	if len(events) != 3 || events[0].EventType != "message_edited" || events[1].EventType != "message_replied" || events[2].EventType != "message_created" {
		t.Fatalf("message events = %#v", events)
	}
}

func TestProjectMessageSearchIsProjectScopedAndValidatesLength(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)
	projectA := createMessageProject(t, database, "Project A")
	projectB := createMessageProject(t, database, "Project B")

	if _, err := database.CreateProjectMessage(ctx, projectA, model.CreateProjectMessageInput{Body: "SQLite FTS finds the queue invariant."}); err != nil {
		t.Fatalf("create searchable message A: %v", err)
	}
	if _, err := database.CreateProjectMessage(ctx, projectB, model.CreateProjectMessageInput{Body: "SQLite FTS finds a different project."}); err != nil {
		t.Fatalf("create searchable message B: %v", err)
	}
	tooLong := strings.Repeat("x", model.MaxProjectMessageBodyLength+1)
	if _, err := database.CreateProjectMessage(ctx, projectA, model.CreateProjectMessageInput{Body: tooLong}); err == nil {
		t.Fatal("expected long message to be rejected")
	}

	hits, err := database.SearchProjectMessages(ctx, projectA, "queue invariant", 20)
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(hits) != 1 || !strings.Contains(hits[0].Body, "queue invariant") {
		t.Fatalf("search hits = %#v", hits)
	}
	if _, err := database.GetProjectMessageThread(ctx, projectB, hits[0].MessageID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-project thread lookup error = %v", err)
	}
}

func formatMessageID(id int64) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(toDecimal(id)), "+"))
}

func toDecimal(id int64) string {
	return strings.TrimSpace(formatInt64(id))
}

func formatInt64(id int64) string {
	if id == 0 {
		return "0"
	}
	negative := id < 0
	if negative {
		id = -id
	}
	var digits [20]byte
	i := len(digits)
	for id > 0 {
		i--
		digits[i] = byte('0' + id%10)
		id /= 10
	}
	if negative {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}
