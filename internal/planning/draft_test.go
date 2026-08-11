// Copyright (c) 2026 Seb. All rights reserved.

package planning_test

import (
	"testing"

	"github.com/JustSebNL/timekeeper/internal/planning"
)

func TestParseDraftAcceptsBoundedHierarchy(t *testing.T) {
	draft, err := planning.ParseDraft([]byte(`{
		"version":"timekeeper-planning-draft/v1",
		"summary":"Deliver the first usable slice.",
		"categories":[{"name":"Delivery","tasks":[{
			"name":"Implement API","estimated_duration_seconds":3600,"buffer_pct":20,
			"sprints":[{"name":"API slice","estimated_duration_seconds":1800,"buffer_pct":10}],
			"subtasks":[{"name":"Routes","estimated_duration_seconds":900,"sprints":[]}]
		}]}]
	}`))
	if err != nil {
		t.Fatalf("parse draft: %v", err)
	}
	if draft.Categories[0].Tasks[0].Sprints[0].BufferPct != 10 {
		t.Fatalf("draft=%#v", draft)
	}
}

func TestParseDraftRejectsUnknownFieldsAndInvalidBuffers(t *testing.T) {
	_, err := planning.ParseDraft([]byte(`{"version":"timekeeper-planning-draft/v1","summary":"x","categories":[],"surprise":true}`))
	if err == nil {
		t.Fatal("expected unknown-field rejection")
	}
	_, err = planning.ParseDraft([]byte(`{"version":"timekeeper-planning-draft/v1","summary":"x","categories":[{"name":"x","tasks":[{"name":"x","estimated_duration_seconds":1,"buffer_pct":12.5,"sprints":[],"subtasks":[]}]}]}`))
	if err == nil {
		t.Fatal("expected fractional buffer rejection")
	}
}
