// Copyright (c) 2026 Seb. All rights reserved.

package model

import "time"

// ProjectNote is a durable, attributed human or agent observation on a project.
type ProjectNote struct {
	NoteID    int64     `json:"note_id"`
	ProjectID string    `json:"project_id"`
	Content   string    `json:"content"`
	ActorID   string    `json:"actor_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateProjectNoteInput holds client-supplied note content.
type CreateProjectNoteInput struct {
	Content string `json:"content"`
}
