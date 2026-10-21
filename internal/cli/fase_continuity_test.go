package cli

import (
	"encoding/json"
	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"github.com/JustSebNL/timekeeper/internal/storeclient"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFASEGuardianNativeResumeKeepsOriginalIdentity(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "wake.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server := httptest.NewServer(api.New(s))
	defer server.Close()
	t.Setenv("TIMEKEEPER_SESSION_ID", "original-worker")
	t.Setenv("TIMEKEEPER_AGENT_RUNTIME", "custom")
	t.Setenv("TIMEKEEPER_FASE_RESUME_AGENT", "")
	t.Setenv("TIMEKEEPER_FASE_RESUME_SESSION", "")
	in, e := discoverFASEIdentity()
	if e != nil {
		t.Fatal(e)
	}
	var out, errOut strings.Builder
	if code := RunTracked([]string{"list"}, &out, &errOut, server.URL); code != 0 {
		t.Fatal(errOut.String())
	}
	id, e := s.ResolveFASEIdentityAt(t.Context(), in, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	actor := model.WithFASEActor(t.Context(), id)
	p, e := s.CreateProject(actor, model.CreateProjectInput{Name: "wake"})
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.CreateCategory(actor, p.ProjectID, model.CreateCategoryInput{Name: "work"})
	if e != nil {
		t.Fatal(e)
	}
	task, e := s.CreateTask(actor, p.ProjectID, model.CreateTaskInput{Name: "task", CategoryID: c.CategoryID})
	if e != nil {
		t.Fatal(e)
	}
	sp, e := s.CreateSprint(actor, task.TaskID, model.CreateSprintInput{Name: "step", EstimatedDurationSeconds: 60})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.TransitionSprint(actor, sp.SprintID, "start", ""); e != nil {
		t.Fatal(e)
	}
	state, e := s.RecordAgentActivityAt(actor, id.AgentID, model.AgentActivityInput{SessionID: model.FASEActivitySession(in.Fingerprint), Runtime: in.Runtime, Event: "work", SprintID: sp.SprintID, LeaseDurationSeconds: 1}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	// Explicit idle creates a real current wake condition without sleeping.
	if _, e = s.RecordAgentActivityAt(actor, id.AgentID, model.AgentActivityInput{SessionID: state.SessionID, Event: "idle"}, time.Now()); e != nil {
		t.Fatal(e)
	}
	nudges, e := s.EvaluatePulseGuardianAt(t.Context(), time.Now())
	if e != nil || len(nudges) != 1 {
		t.Fatalf("%+v %v", nudges, e)
	}
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ctx := &AgentContext{AgentID: id.AgentID, LastProjectID: p.ProjectID, Adapter: executable}
	client := &storeclient.Client{BaseURL: server.URL, HTTPClient: server.Client()}
	if e = prepareGuardianWake(client, ctx, nudges[0]); e != nil {
		t.Fatal(e)
	}
	receipt := filepath.Join(t.TempDir(), "identity.json")
	t.Setenv("TIMEKEEPER_TEST_FASE_ADAPTER", "1")
	t.Setenv("TK_TEST_WAKE_RECEIPT", receipt)
	t.Setenv("TIMEKEEPER_SESSION_ID", "wrong-poller")
	t.Setenv("TIMEKEEPER_FASE_FINGERPRINT", strings.Repeat("f", 64))
	if e = runGuardianWorkCycle(id.AgentID, ctx, nil, false, 1, 1); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(receipt)
	if e != nil {
		t.Fatal(e)
	}
	var resumed model.FASEIdentity
	if e = json.Unmarshal(data, &resumed); e != nil {
		t.Fatal(e)
	}
	if resumed.AgentID != id.AgentID || resumed.IdentityScope != id.IdentityScope {
		t.Fatalf("wake split identity: original=%s resumed=%s", id.AgentID, data)
	}
	if e = acknowledgeGuardianWake(client, id.AgentID, nudges[0].NudgeID); e != nil {
		t.Fatal(e)
	}
	after, e := s.AgentWorkStateAt(t.Context(), id.AgentID, time.Now())
	if e != nil || !after.LastProgressAt.Equal(state.LastProgressAt) {
		t.Fatalf("ACK fabricated progress: %+v %v", after, e)
	}
}

func TestFASEClaimCommands(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "claims.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server := httptest.NewServer(api.New(s))
	defer server.Close()
	p, e := s.CreateProject(t.Context(), model.CreateProjectInput{Name: "claims"})
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.CreateCategory(t.Context(), p.ProjectID, model.CreateCategoryInput{Name: "work"})
	if e != nil {
		t.Fatal(e)
	}
	task, e := s.CreateTask(t.Context(), p.ProjectID, model.CreateTaskInput{Name: "task", CategoryID: c.CategoryID})
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("TIMEKEEPER_SESSION_ID", "claimant")
	t.Setenv("TIMEKEEPER_AGENT_RUNTIME", "custom")
	for _, args := range [][]string{{"agent", "claim", task.TaskID}, {"agent", "claim", task.TaskID}, {"agent", "claims", p.ProjectID}, {"agent", "release", task.TaskID}} {
		var out, errOut strings.Builder
		if code := RunTracked(args, &out, &errOut, server.URL); code != 0 {
			t.Fatalf("%v: %s", args, errOut.String())
		}
	}
}
