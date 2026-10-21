// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

const MaxProjectMessageBodyLength = 1000
const MaxProjectMessageAuthorLength = 120
const MaxProjectMessageTagsLength = 240

const (
	DefaultProjectMessageContextThreadLimit   = 12
	DefaultProjectMessageContextRelatedLimit  = 8
	DefaultProjectMessageContextRevisionLimit = 4
	MaxProjectMessageContextThreadLimit       = 50
	MaxProjectMessageContextRelatedLimit      = 25
	MaxProjectMessageContextRevisionLimit     = 12
	MaxProjectMessageCheckpointEntries        = 25
)

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
	MessageID       int64       `json:"message_id"`
	Ref             string      `json:"ref"`
	ProjectID       string      `json:"project_id"`
	ParentMessageID *int64      `json:"parent_message_id,omitempty"`
	Kind            MessageKind `json:"kind"`
	Author          string      `json:"author,omitempty"`
	Body            string      `json:"body"`
	Link            string      `json:"link,omitempty"`
	Tags            string      `json:"tags,omitempty"`
	Revision        int         `json:"revision"`
	Pinned          bool        `json:"pinned"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// ProjectMessageRevision is an immutable prior/current version of a message.
type ProjectMessageRevision struct {
	RevisionID int64       `json:"revision_id"`
	MessageID  int64       `json:"message_id"`
	ProjectID  string      `json:"project_id"`
	Revision   int         `json:"revision"`
	Kind       MessageKind `json:"kind"`
	Author     string      `json:"author,omitempty"`
	Body       string      `json:"body"`
	Link       string      `json:"link,omitempty"`
	Tags       string      `json:"tags,omitempty"`
	EditedBy   string      `json:"edited_by,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

// CreateProjectMessageInput holds client-supplied message data.
type CreateProjectMessageInput struct {
	Kind            MessageKind `json:"kind"`
	Author          string      `json:"author"`
	Body            string      `json:"body"`
	Link            string      `json:"link"`
	Tags            string      `json:"tags"`
	ParentMessageID *int64      `json:"parent_message_id"`
}

type ProjectMessageCheckpointInput struct {
	Author  string                      `json:"author"`
	Entries []CreateProjectMessageInput `json:"entries"`
}

type ProjectMessageCheckpoint struct {
	Root  ProjectMessage   `json:"root"`
	Items []ProjectMessage `json:"items"`
}

// UpdateProjectMessageInput holds editable message fields. Thread lineage is
// immutable after creation so edits cannot accidentally re-parent a discussion.
type UpdateProjectMessageInput struct {
	Kind   MessageKind `json:"kind"`
	Author string      `json:"author"`
	Body   string      `json:"body"`
	Link   string      `json:"link"`
	Tags   string      `json:"tags"`
}

type ProjectMessageSearchFilter struct {
	Limit  int
	Kind   string
	Tags   []string
	After  *time.Time
	Before *time.Time
}

// ProjectMessageSearchHit is one search result, with the FTS5 snippet
// that matched so the UI can show why this entry came back.
type ProjectMessageSearchHit struct {
	ProjectMessage
	Snippet       string  `json:"snippet"`
	SemanticScore float64 `json:"semantic_score,omitempty"`
}

// ProjectMessageContextOptions controls one bounded pointer-first hydration.
// Zero values use the documented defaults; larger values are capped by store.
type ProjectMessageContextOptions struct {
	Query         string
	ThreadLimit   int
	RelatedLimit  int
	RevisionLimit int
}

// ProjectMessageContext is the compact recovery bundle for an M-<id> pointer.
type ProjectMessageContext struct {
	Anchor             ProjectMessage            `json:"anchor"`
	Thread             []ProjectMessage          `json:"thread"`
	Related            []ProjectMessageSearchHit `json:"related"`
	Revisions          []ProjectMessageRevision  `json:"revisions"`
	RelatedQuery       string                    `json:"related_query,omitempty"`
	RelatedMode        string                    `json:"related_mode"`
	ThreadTruncated    bool                      `json:"thread_truncated"`
	RelatedTruncated   bool                      `json:"related_truncated"`
	RevisionsTruncated bool                      `json:"revisions_truncated"`
}
