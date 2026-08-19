// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectOperationalSummaryReportsDurableSprintStateAndEstimate(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Time Keeper"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Execution"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Visibility"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	active, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Active", EstimatedDurationSeconds: 900, BufferPct: 10})
	if err != nil {
		t.Fatalf("create active sprint: %v", err)
	}
	held, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Held", EstimatedDurationSeconds: 600, BufferPct: 50})
	if err != nil {
		t.Fatalf("create held sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, active.SprintID, "start", ""); err != nil {
		t.Fatalf("start active sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, held.SprintID, "start", ""); err != nil {
		t.Fatalf("start held sprint: %v", err)
	}
	if _, err := database.TransitionSprint(ctx, held.SprintID, "hold", "waiting for external input"); err != nil {
		t.Fatalf("hold sprint: %v", err)
	}

	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/operational-summary", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", response.Code, response.Body.String())
	}
	var summary struct {
		ProjectID                string `json:"project_id"`
		TotalSprints             int64  `json:"total_sprints"`
		ActiveSprints            int64  `json:"active_sprints"`
		HeldSprints              int64  `json:"held_sprints"`
		OpenSprints              int64  `json:"open_sprints"`
		CompletedSprints         int64  `json:"completed_sprints"`
		EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
		BufferDurationSeconds    int64  `json:"buffer_duration_seconds"`
		PlannedDurationSeconds   int64  `json:"planned_duration_seconds"`
		RecordedWorkSeconds      int64  `json:"recorded_work_seconds"`
	}
	if err := json.NewDecoder(response.Body).Decode(&summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.ProjectID != project.ProjectID || summary.TotalSprints != 2 || summary.ActiveSprints != 1 || summary.HeldSprints != 1 || summary.OpenSprints != 0 || summary.CompletedSprints != 0 || summary.EstimatedDurationSeconds != 1500 || summary.BufferDurationSeconds != 390 || summary.PlannedDurationSeconds != 1890 || summary.RecordedWorkSeconds < 0 {
		t.Fatalf("summary = %#v", summary)
	}
}
