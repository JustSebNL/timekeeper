// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const sprintSelectCols = `sprint_id, project_id, category_id, task_id, subtask_id, item_address,
	sprint_name, sprint_description, sprint_goal, status, priority, estimated_duration_seconds,
	buffer_pct, buffer_duration_seconds, active_duration_seconds, hold_duration_seconds, hold_reason,
	started_at, ended_at, created_at, updated_at`

// GetHandoffSnapshot compiles a compact operational and context snapshot of a Project for agent handoffs.
func (s *Store) GetHandoffSnapshot(ctx context.Context, idOrAlias string) (model.HandoffSnapshot, error) {
	projectID, err := s.ResolveProjectID(ctx, idOrAlias)
	if err != nil {
		return model.HandoffSnapshot{}, err
	}

	row := s.db.QueryRowContext(ctx, `SELECT project_id, project_number, item_address, project_name,
		project_description, project_goal, status, priority, palette_id,
		reported_completion_pct, calculated_completion_pct, created_at, updated_at, project_alias,
		planned_start_date, target_end_date, actual_started_at, completed_at, cancelled_at,
		lifecycle_phase, schedule_timezone, deadline_warning_days
		FROM projects WHERE project_id = ?`, projectID)
	project, err := scanProject(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.HandoffSnapshot{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.HandoffSnapshot{}, fmt.Errorf("read handoff project: %w", err)
	}

	summary, err := s.ProjectOperationalSummary(ctx, projectID)
	if err != nil {
		return model.HandoffSnapshot{}, fmt.Errorf("read handoff operational summary: %w", err)
	}

	var activeSprint *model.Sprint
	var onHoldSprints []model.Sprint
	var nextRunnable *model.Sprint

	// Active sprint
	actRow := s.db.QueryRowContext(ctx, "SELECT "+sprintSelectCols+" FROM sprints WHERE project_id = ? AND status = 'Active' ORDER BY updated_at DESC LIMIT 1", projectID)
	spAct, errAct := scanSprint(actRow)
	if errAct == nil {
		activeSprint = &spAct
	}

	// On Hold sprints
	rowsHold, errHold := s.db.QueryContext(ctx, "SELECT "+sprintSelectCols+" FROM sprints WHERE project_id = ? AND status = 'On Hold' ORDER BY updated_at DESC", projectID)
	if errHold == nil {
		for rowsHold.Next() {
			if sp, errScan := scanSprint(rowsHold); errScan == nil {
				onHoldSprints = append(onHoldSprints, sp)
			}
		}
		_ = rowsHold.Close()
	}

	// Next runnable sprint
	runRow := s.db.QueryRowContext(ctx, `SELECT `+sprintSelectCols+` FROM sprints s
		JOIN projects p ON p.project_id = s.project_id
		JOIN categories c ON c.category_id = s.category_id
		JOIN tasks t ON t.task_id = s.task_id
		LEFT JOIN subtasks st ON st.subtask_id = s.subtask_id
		WHERE s.project_id = ? AND s.status = 'Open' AND p.status = 'Open' AND c.status = 'Open' AND t.status = 'Open'
		AND (st.subtask_id IS NULL OR st.status = 'Open')
		ORDER BY s.created_at, s.sprint_id LIMIT 1`, projectID)
	spRun, errRun := scanSprint(runRow)
	if errRun == nil {
		nextRunnable = &spRun
	}

	// Recent Notes (limit 5)
	allNotes, _ := s.ListProjectNotes(ctx, projectID)
	var recentNotes []model.ProjectNote
	if len(allNotes) > 5 {
		recentNotes = allNotes[:5]
	} else {
		recentNotes = allNotes
	}

	// Recent Messages (limit 5)
	allMsgs, _ := s.ListProjectMessages(ctx, projectID, "", 5)
	var recentMsgs []model.ProjectMessage
	if len(allMsgs) > 5 {
		recentMsgs = allMsgs[len(allMsgs)-5:]
	} else {
		recentMsgs = allMsgs
	}

	// Recent Events (limit 10)
	recentEvents, _ := s.ListProjectEventsLimit(ctx, projectID, 10)

	// Attention Items
	attention, _ := s.ProjectAttention(ctx, projectID)
	var attentionItems []string
	for _, item := range attention.Items {
		reason := item.HoldReason
		if reason == "" {
			reason = item.Detail
		}
		attentionItems = append(attentionItems, fmt.Sprintf("%s (%s): %s", item.SprintID, item.Status, reason))
	}

	snapshot := model.HandoffSnapshot{
		Project:            project,
		OperationalSummary: summary,
		ActiveSprint:       activeSprint,
		OnHoldSprints:      onHoldSprints,
		NextRunnableSprint: nextRunnable,
		RecentNotes:        recentNotes,
		RecentMessages:     recentMsgs,
		RecentEvents:       recentEvents,
		AttentionItems:     attentionItems,
		GeneratedAt:        time.Now().UTC().Round(0),
	}

	snapshot.Markdown = buildHandoffMarkdown(snapshot)
	return snapshot, nil
}

func buildHandoffMarkdown(s model.HandoffSnapshot) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Agent Handoff Snapshot: %s (%s)\n", s.Project.ProjectName, s.Project.ProjectID))
	sb.WriteString(fmt.Sprintf("**Status**: %s", s.Project.Status))
	if s.Project.ProjectAlias != "" {
		sb.WriteString(fmt.Sprintf(" | **Alias**: %s", s.Project.ProjectAlias))
	}
	sb.WriteString(fmt.Sprintf(" | **Generated**: %s\n", s.GeneratedAt.Format(time.RFC3339)))
	if s.Project.ProjectGoal != "" {
		sb.WriteString(fmt.Sprintf("**Goal**: %s\n", s.Project.ProjectGoal))
	}
	sb.WriteString("\n")

	sb.WriteString("## 📊 Operational Summary\n")
	sb.WriteString(fmt.Sprintf("- **Sprints**: %d Active, %d On Hold, %d Open, %d Completed, %d TimedOut\n",
		s.OperationalSummary.ActiveSprints, s.OperationalSummary.HeldSprints,
		s.OperationalSummary.OpenSprints, s.OperationalSummary.CompletedSprints,
		s.OperationalSummary.TimedOutSprints))
	sb.WriteString(fmt.Sprintf("- **Recorded Work**: %ds / Planned: %ds\n\n",
		s.OperationalSummary.RecordedWorkSeconds, s.OperationalSummary.PlannedDurationSeconds))

	sb.WriteString("## 🏃 Active Sprint\n")
	if s.ActiveSprint != nil {
		sb.WriteString(fmt.Sprintf("- **Sprint**: %s (%s)\n", s.ActiveSprint.SprintID, s.ActiveSprint.Name))
		sb.WriteString(fmt.Sprintf("  - **Task**: %s | **Address**: %s\n", s.ActiveSprint.TaskID, s.ActiveSprint.ItemAddress))
		sb.WriteString(fmt.Sprintf("  - **Status**: %s | **Active Work**: %ds\n", s.ActiveSprint.Status, s.ActiveSprint.ActiveDurationSeconds))
	} else {
		sb.WriteString("- No Sprint currently Active.\n")
	}
	sb.WriteString("\n")

	if len(s.OnHoldSprints) > 0 {
		sb.WriteString("## ⏸️ On-Hold Sprints\n")
		for _, sp := range s.OnHoldSprints {
			sb.WriteString(fmt.Sprintf("- **%s**: %s (Reason: %s)\n", sp.SprintID, sp.Name, sp.HoldReason))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## 📋 Next Runnable Sprint\n")
	if s.NextRunnableSprint != nil {
		sb.WriteString(fmt.Sprintf("- **Sprint**: %s (%s)\n", s.NextRunnableSprint.SprintID, s.NextRunnableSprint.Name))
		sb.WriteString(fmt.Sprintf("  - **Task**: %s | **Address**: %s\n", s.NextRunnableSprint.TaskID, s.NextRunnableSprint.ItemAddress))
	} else {
		sb.WriteString("- No Open runnable Sprints available.\n")
	}
	sb.WriteString("\n")

	if len(s.RecentNotes) > 0 {
		sb.WriteString("## 📝 Recent Notes\n")
		for _, n := range s.RecentNotes {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", n.CreatedAt.Format("2006-01-02 15:04"), n.Content))
		}
		sb.WriteString("\n")
	}

	if len(s.RecentMessages) > 0 {
		sb.WriteString("## 💬 Trail of Thought & Checkpoints\n")
		for _, m := range s.RecentMessages {
			sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", m.CreatedAt.Format("2006-01-02 15:04"), m.Author, m.Body))
		}
		sb.WriteString("\n")
	}

	if len(s.RecentEvents) > 0 {
		sb.WriteString("## ⚡ Recent Activity\n")
		for _, e := range s.RecentEvents {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.CreatedAt.Format("2006-01-02 15:04"), e.Message))
		}
		sb.WriteString("\n")
	}

	if len(s.AttentionItems) > 0 {
		sb.WriteString("## ⚠️ Attention Needed\n")
		for _, att := range s.AttentionItems {
			sb.WriteString(fmt.Sprintf("- %s\n", att))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
