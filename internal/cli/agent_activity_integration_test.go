package cli

import (
	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentCLITracksSeparateSessionsAndBoundedWait(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "cli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	project, err := s.CreateProject(t.Context(), model.CreateProjectInput{Name: "agent integration"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := s.CreateCategory(t.Context(), project.ProjectID, model.CreateCategoryInput{Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(t.Context(), project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "test", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	sprint, err := s.CreateSprint(t.Context(), task.TaskID, model.CreateSprintInput{Name: "test", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.New(s))
	defer server.Close()
	t.Setenv("TIMEKEEPER_AGENT_ID", "custom-one")
	t.Setenv("TIMEKEEPER_SESSION_ID", "session-one")
	t.Setenv("TIMEKEEPER_AGENT_RUNTIME", "any-runtime")
	run := func(args ...string) {
		t.Helper()
		var out, errOut strings.Builder
		if code := Run(args, &out, &errOut, server.URL); code != 0 {
			t.Fatalf("%v: %d %s", args, code, errOut.String())
		}
	}
	run("sp", "start", sprint.SprintID)
	state, err := s.AgentWorkStateAt(t.Context(), "custom-one", time.Now())
	if err != nil || state.State != "working" {
		t.Fatalf("%+v %v", state, err)
	}
	progress := state.LastProgressAt
	run("agent", "state")
	run("pulse")
	state, err = s.AgentWorkStateAt(t.Context(), "custom-one", time.Now())
	if err != nil || !state.LastProgressAt.Equal(progress) {
		t.Fatalf("poll fabricated work: %+v %v", state, err)
	}
	run("agent", "wait", "2m", "approval pending")
	state, err = s.AgentWorkStateAt(t.Context(), "custom-one", time.Now())
	if err != nil || state.State != "waiting" || state.WaitUntil == nil {
		t.Fatalf("%+v %v", state, err)
	}
	run("agent", "idle", "unexpected pause")
	state, err = s.AgentWorkStateAt(t.Context(), "custom-one", time.Now())
	if err != nil || state.State != "idle" {
		t.Fatalf("%+v %v", state, err)
	}
	run("agent", "work", sprint.SprintID, "2m")
	run("agent", "stop", "handoff")
	t.Setenv("TIMEKEEPER_AGENT_ID", "custom-two")
	t.Setenv("TIMEKEEPER_SESSION_ID", "session-two")
	run("list")
	run("agent", "states")
	states, err := s.ListAgentWorkStatesAt(t.Context(), time.Now())
	if err != nil || len(states) != 2 {
		t.Fatalf("%+v %v", states, err)
	}
}
