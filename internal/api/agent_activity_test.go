package api_test

import (
	"bytes"
	"encoding/json"
	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestAgentActivityTracksOrdinaryRequestsWithoutCountingPollingAsWork(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	project, err := s.CreateProject(ctx, model.CreateProjectInput{Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := s.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{Name: "test", CategoryID: category.CategoryID, EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	sprint, err := s.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "test", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.New(s)
	call := func(agent, session, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Agent-ID", agent)
		r.Header.Set("X-Agent-Session-ID", session)
		r.Header.Set("X-Agent-Runtime", "custom")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := call("alpha", "session-a", "POST", "/api/v1/sprints/"+sprint.SprintID+"/start", "{}")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	state, err := s.AgentWorkStateAt(ctx, "alpha", time.Now())
	if err != nil || state.State != "working" || state.ActiveSprintID != sprint.SprintID {
		t.Fatalf("untracked Sprint start: %+v %v", state, err)
	}
	progress := state.LastProgressAt
	for i := 0; i < 3; i++ {
		call("alpha", "session-a", "GET", "/api/v1/pulse", "")
	}
	state, err = s.AgentWorkStateAt(ctx, "alpha", time.Now())
	if err != nil || !state.LastProgressAt.Equal(progress) {
		t.Fatalf("polling manufactured progress: %+v %v", state, err)
	}
	call("beta", "session-b", "GET", "/api/v1/projects", "")
	w = call("beta", "session-b", "GET", "/api/v1/agents/work-states", "")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var list struct {
		Items []model.AgentWorkState `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Items) != 2 {
		t.Fatalf("%s %v", w.Body.String(), err)
	}
	w = call("alpha", "another-session", "POST", "/api/v1/sprints/"+sprint.SprintID+"/complete", "{}")
	if w.Code != 409 {
		t.Fatalf("identity collision allowed mutation: %d %s", w.Code, w.Body.String())
	}
	w = call("alpha", "session-a", "POST", "/api/v1/agents/alpha/activity", `{"session_id":"session-a","event":"wait","wait_duration_seconds":60,"reason":"approval"}`)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	state, err = s.AgentWorkStateAt(ctx, "alpha", time.Now())
	if err != nil || state.State != "waiting" {
		t.Fatalf("%+v %v", state, err)
	}
	w = call("alpha", "session-a", "POST", "/api/v1/agents/alpha/progress", `{"lease_duration_seconds":30}`)
	if w.Code != 201 {
		t.Fatalf("legacy progress: %d %s", w.Code, w.Body.String())
	}
	state, err = s.AgentWorkStateAt(ctx, "alpha", time.Now())
	if err != nil || state.State != "working" {
		t.Fatalf("progress did not resume work: %+v %v", state, err)
	}
	w = call("alpha", "session-a", "POST", "/api/v1/sprints/"+sprint.SprintID+"/complete", `{}`)
	if w.Code != 200 {
		t.Fatalf("complete: %d %s", w.Code, w.Body.String())
	}
	w = call("alpha", "session-a", "POST", "/api/v1/projects/"+project.ProjectID+"/notes", `{"content":"verified completion"}`)
	if w.Code != 201 {
		t.Fatalf("completion evidence: %d %s", w.Code, w.Body.String())
	}
}
