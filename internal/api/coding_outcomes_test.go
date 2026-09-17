package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"net/http/httptest"
	"testing"
)

func TestCodingOutcomeAPI(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/outcomes.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	p, _ := db.CreateProject(ctx, model.CreateProjectInput{Name: "Outcomes"})
	c, _ := db.CreateCategory(ctx, p.ProjectID, model.CreateCategoryInput{Name: "C"})
	task, _ := db.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{CategoryID: c.CategoryID, Name: "T"})
	sp, _ := db.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "S", EstimatedDurationSeconds: 60})
	_, err = db.RecordUsageSnapshot(ctx, p.ProjectID, model.UsageSnapshotInput{SessionID: "run", AgentID: "agent", Model: "coder", SprintID: sp.SprintID, TurnSeq: 1, InputTokens: 100, OutputTokens: 20})
	if err != nil {
		t.Fatal(err)
	}
	h := api.New(db)
	body := map[string]any{"session_id": "run", "agent_id": "agent", "category": "debugging", "difficulty": "medium", "benchmark": "fixture-v1", "configuration": "high-harness-v1", "provider": "test", "result": "accepted", "reviewer": "human:tester", "evidence": "test report abc123", "rework_count": 0}
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, req)
		return w
	}
	path := "/api/v1/projects/" + p.ProjectID + "/coding-outcomes"
	w := call("POST", path, body)
	if w.Code != 201 {
		t.Fatalf("record: %d %s", w.Code, w.Body)
	}
	w = call("POST", path, body)
	if w.Code != 200 {
		t.Fatalf("idempotent: %d %s", w.Code, w.Body)
	}
	body["result"] = "rejected"
	w = call("POST", path, body)
	if w.Code != 400 {
		t.Fatalf("immutable result: %d", w.Code)
	}
	// Review remains frozen even when a later usage turn arrives.
	_, err = db.RecordUsageSnapshot(ctx, p.ProjectID, model.UsageSnapshotInput{SessionID: "run", AgentID: "agent", Model: "coder", SprintID: sp.SprintID, TurnSeq: 2, InputTokens: 250, OutputTokens: 50})
	if err != nil {
		t.Fatal(err)
	}
	w = call("GET", path, nil)
	if w.Code != 200 {
		t.Fatalf("summary %d", w.Code)
	}
	var summary struct {
		Items []struct {
			Model       string `json:"model"`
			Accepted    int    `json:"accepted"`
			Samples     int    `json:"samples"`
			InputTokens int    `json:"input_tokens"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Items) != 1 || summary.Items[0].Accepted != 1 || summary.Items[0].Samples != 1 || summary.Items[0].InputTokens != 100 {
		t.Fatalf("summary %s", w.Body)
	}
	body["session_id"] = "missing"
	body["evidence"] = ""
	w = call("POST", path, body)
	if w.Code != 400 {
		t.Fatalf("missing evidence accepted: %d", w.Code)
	}
}
