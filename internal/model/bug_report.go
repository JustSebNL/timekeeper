// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

import "time"

const (
	MaxBugReportTitleLength = 240
	MaxBugReportTextLength  = 12000
	MaxBugReportSteps       = 50
	MaxBugReportEvidence    = 50
)

type BugReportSource struct {
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
	Command   string `json:"command"`
}

type BugReportEvidence struct {
	Type string `json:"type"`
	Ref  string `json:"ref"`
}

type CreateBugReportInput struct {
	ProjectID         string              `json:"project_id"`
	CategoryID        string              `json:"category_id"`
	TaskID            string              `json:"task_id"`
	SprintID          string              `json:"sprint_id"`
	Title             string              `json:"title"`
	Severity          string              `json:"severity"`
	Expected          string              `json:"expected"`
	Actual            string              `json:"actual"`
	ReproductionSteps []string            `json:"reproduction_steps"`
	Environment       map[string]string   `json:"environment"`
	Source            BugReportSource     `json:"source"`
	Evidence          []BugReportEvidence `json:"evidence"`
	DedupeKey         string              `json:"dedupe_key"`
	IdempotencyKey    string              `json:"idempotency_key"`
	RedactionStatus   string              `json:"redaction_status"`
}

type BugReport struct {
	ReportID          string              `json:"report_id"`
	ProjectID         string              `json:"project_id"`
	CategoryID        string              `json:"category_id,omitempty"`
	TaskID            string              `json:"task_id,omitempty"`
	SprintID          string              `json:"sprint_id,omitempty"`
	Title             string              `json:"title"`
	Severity          string              `json:"severity"`
	Expected          string              `json:"expected"`
	Actual            string              `json:"actual"`
	ReproductionSteps []string            `json:"reproduction_steps"`
	Environment       map[string]string   `json:"environment,omitempty"`
	Source            BugReportSource     `json:"source"`
	Evidence          []BugReportEvidence `json:"evidence,omitempty"`
	DedupeKey         string              `json:"dedupe_key,omitempty"`
	IdempotencyKey    string              `json:"idempotency_key"`
	RedactionStatus   string              `json:"redaction_status"`
	Status            string              `json:"status"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

type BugReportResult struct {
	Report    BugReport `json:"report"`
	Duplicate bool      `json:"duplicate"`
}
