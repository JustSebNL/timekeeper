// Copyright (c) 2026 Seb. All rights reserved.

package planning

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

const DraftVersion = "timekeeper-planning-draft/v1"
const maxDraftDurationSeconds int64 = 10 * 365 * 24 * 60 * 60

// Draft is a model-suggested hierarchy. It is inert until a separate explicit
// approval/apply operation materializes it in SQLite.
type Draft struct {
	Version    string          `json:"version"`
	Summary    string          `json:"summary"`
	Categories []DraftCategory `json:"categories"`
}
type DraftCategory struct {
	Name  string      `json:"name"`
	Tasks []DraftTask `json:"tasks"`
}
type DraftTask struct {
	Name                     string         `json:"name"`
	EstimatedDurationSeconds int64          `json:"estimated_duration_seconds"`
	BufferPct                float64        `json:"buffer_pct"`
	Sprints                  []DraftSprint  `json:"sprints"`
	Subtasks                 []DraftSubtask `json:"subtasks"`
}
type DraftSubtask struct {
	Name                     string        `json:"name"`
	EstimatedDurationSeconds int64         `json:"estimated_duration_seconds"`
	Sprints                  []DraftSprint `json:"sprints"`
}
type DraftSprint struct {
	Name                     string  `json:"name"`
	EstimatedDurationSeconds int64   `json:"estimated_duration_seconds"`
	BufferPct                float64 `json:"buffer_pct"`
}

// ParseDraft strictly decodes and validates a bounded proposed hierarchy.
func ParseDraft(data []byte) (Draft, error) {
	var draft Draft
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil {
		return Draft{}, fmt.Errorf("decode planning draft: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Draft{}, err
	}
	if draft.Version != DraftVersion {
		return Draft{}, fmt.Errorf("planning draft version must be %q", DraftVersion)
	}
	draft.Summary = strings.TrimSpace(draft.Summary)
	if draft.Summary == "" || len(draft.Summary) > 10000 {
		return Draft{}, errors.New("planning draft summary must contain 1 to 10000 bytes")
	}
	if len(draft.Categories) == 0 || len(draft.Categories) > 50 {
		return Draft{}, errors.New("planning draft must contain 1 to 50 categories")
	}
	items := 0
	for ci := range draft.Categories {
		category := &draft.Categories[ci]
		category.Name = strings.TrimSpace(category.Name)
		if category.Name == "" || len(category.Name) > 200 {
			return Draft{}, errors.New("planning draft category name is invalid")
		}
		if len(category.Tasks) == 0 || len(category.Tasks) > 100 {
			return Draft{}, errors.New("planning draft category must contain 1 to 100 tasks")
		}
		for ti := range category.Tasks {
			task := &category.Tasks[ti]
			if err := validateTask(task); err != nil {
				return Draft{}, err
			}
			items += 1 + len(task.Sprints) + len(task.Subtasks)
			for _, subtask := range task.Subtasks {
				items += len(subtask.Sprints)
			}
		}
	}
	if items > 500 {
		return Draft{}, errors.New("planning draft exceeds 500 hierarchy items")
	}
	return draft, nil
}
func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("planning draft must contain one JSON value")
		}
		return fmt.Errorf("decode planning draft suffix: %w", err)
	}
	return nil
}
func validateTask(task *DraftTask) error {
	task.Name = strings.TrimSpace(task.Name)
	if task.Name == "" || !validDraftDuration(task.EstimatedDurationSeconds) || !wholeBuffer(task.BufferPct) {
		return errors.New("planning draft task is invalid")
	}
	if len(task.Sprints) > 100 || len(task.Subtasks) > 100 {
		return errors.New("planning draft task has too many children")
	}
	for si := range task.Sprints {
		if err := validateSprint(&task.Sprints[si]); err != nil {
			return err
		}
	}
	for si := range task.Subtasks {
		sub := &task.Subtasks[si]
		sub.Name = strings.TrimSpace(sub.Name)
		if sub.Name == "" || !validDraftDuration(sub.EstimatedDurationSeconds) || len(sub.Sprints) > 100 {
			return errors.New("planning draft subtask is invalid")
		}
		for pi := range sub.Sprints {
			if err := validateSprint(&sub.Sprints[pi]); err != nil {
				return err
			}
		}
	}
	return nil
}
func validateSprint(sprint *DraftSprint) error {
	sprint.Name = strings.TrimSpace(sprint.Name)
	if sprint.Name == "" || !validDraftDuration(sprint.EstimatedDurationSeconds) || !wholeBuffer(sprint.BufferPct) {
		return errors.New("planning draft sprint is invalid")
	}
	return nil
}
func wholeBuffer(value float64) bool      { return value >= 0 && value <= 100 && value == math.Trunc(value) }
func validDraftDuration(value int64) bool { return value >= 0 && value <= maxDraftDurationSeconds }
