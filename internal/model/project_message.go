// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

// MessageKind is the kind of a project-scoped message board entry. The set
// is closed so the dashboard can style known kinds without guessing.
type MessageKind string

const (
	MessageKindNote        MessageKind = "note"
	MessageKindDecision    MessageKind = "decision"
	MessageKindObservation MessageKind = "observation"
	MessageKindLink        MessageKind = "link"
	MessageKindLesson      MessageKind = "lesson"
	MessageKindQuestion    MessageKind = "question"
	MessageKindAnswer      MessageKind = "answer"
)

// ValidMessageKinds is the set of accepted kind values, in display order.
var ValidMessageKinds = []MessageKind{
	MessageKindNote,
	MessageKindDecision,
	MessageKindObservation,
	MessageKindLink,
	MessageKindLesson,
	MessageKindQuestion,
	MessageKindAnswer,
}

// IsValidMessageKind reports whether k is one of the accepted kinds.
func IsValidMessageKind(k MessageKind) bool {
	for _, v := range ValidMessageKinds {
		if v == k {
			return true
		}
	}
	return false
}

// ProjectMessage is one entry on a project's message board: a dated,
// attributed, kind-tagged note. It is the long-term, project-scoped memory
// surface that survives across sessions and reduces the weight agents
// have to carry in their context windows.
type ProjectMessage struct {
	MessageID int64       `json:"message_id"`
	ProjectID string      `json:"project_id"`
	Kind      MessageKind `json:"kind"`
	Author    string      `json:"author,omitempty"`
	Body      string      `json:"body"`
	Link      string      `json:"link,omitempty"`
	Tags      string      `json:"tags,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

// CreateProjectMessageInput holds client-supplied message data.
type CreateProjectMessageInput struct {
	Kind   MessageKind `json:"kind"`
	Author string      `json:"author"`
	Body   string      `json:"body"`
	Link   string      `json:"link"`
	Tags   string      `json:"tags"`
}

// ProjectMessageSearchHit is one search result, with the FTS5 snippet
// that matched so the UI can show why this entry came back.
type ProjectMessageSearchHit struct {
	ProjectMessage
	Snippet string `json:"snippet"`
}
