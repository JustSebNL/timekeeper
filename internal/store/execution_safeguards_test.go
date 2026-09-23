package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func safeguardFixture(t *testing.T) (*store.Store, string, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "safeguards.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	p, err := db.CreateProject(ctx, model.CreateProjectInput{Name: "Safeguards"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := db.CreateCategory(ctx, p.ProjectID, model.CreateCategoryInput{Name: "Execution"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := db.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{CategoryID: c.CategoryID, Name: "Bounded outcome"})
	if err != nil {
		t.Fatal(err)
	}
	sp, err := db.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Work", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	return db, p.ProjectID, sp.SprintID
}

func TestSafeguardExtensionLimits(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		db, _, id := safeguardFixture(t)
		ctx := context.Background()
		for i := 0; i < 3; i++ {
			if _, err := db.AddSprintTimeExtension(ctx, id, model.CreateSprintTimeExtensionInput{DurationSeconds: 10, Reason: "New evidence"}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.AddSprintTimeExtension(ctx, id, model.CreateSprintTimeExtensionInput{DurationSeconds: 1, Reason: "Fourth"}); err == nil {
			t.Fatal("fourth extension accepted")
		}
		xs, err := db.ListSprintTimeExtensions(ctx, id)
		if err != nil || len(xs) != 3 {
			t.Fatalf("extensions=%v err=%v", xs, err)
		}
	})
	t.Run("total", func(t *testing.T) {
		db, _, id := safeguardFixture(t)
		ctx := context.Background()
		if _, err := db.AddSprintTimeExtension(ctx, id, model.CreateSprintTimeExtensionInput{DurationSeconds: 60, Reason: "One original estimate"}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.AddSprintTimeExtension(ctx, id, model.CreateSprintTimeExtensionInput{DurationSeconds: 1, Reason: "Over budget"}); err == nil {
			t.Fatal("extension exceeding original estimate accepted")
		}
	})
	t.Run("terminal", func(t *testing.T) {
		db, _, id := safeguardFixture(t)
		ctx := context.Background()
		if _, err := db.TransitionSprint(ctx, id, "cancel", "No longer required"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.AddSprintTimeExtension(ctx, id, model.CreateSprintTimeExtensionInput{DurationSeconds: 1, Reason: "Invalid"}); err == nil {
			t.Fatal("terminal sprint extended")
		}
	})
}

func TestSafeguardUsageModelIdentity(t *testing.T) {
	db, p, _ := safeguardFixture(t)
	ctx := context.Background()
	in := model.UsageSnapshotInput{SessionID: "s", AgentID: "agent", Model: "model-a", TurnSeq: 1, InputTokens: 100}
	if _, err := db.RecordUsageSnapshot(ctx, p, in); err != nil {
		t.Fatal(err)
	}
	in.Model = "model-b"
	in.TurnSeq = 2
	in.InputTokens = 200
	if _, err := db.RecordUsageSnapshot(ctx, p, in); err == nil {
		t.Fatal("historical usage reassigned to another model")
	}
	sum, err := db.ProjectUsageSummary(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Totals.InputTokens != 100 || sum.Sessions[0].Model != "model-a" {
		t.Fatalf("summary changed: %#v", sum)
	}
	in.Model = ""
	if _, err := db.RecordUsageSnapshot(ctx, p, in); err != nil {
		t.Fatal(err)
	}
}

func TestSafeguardGuardianDeadlineIgnoresLongLease(t *testing.T) {
	db, _, id := safeguardFixture(t)
	ctx := context.Background()
	sp, err := db.TransitionSprint(ctx, id, "start", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReportAgentProgress(ctx, "agent:budget", model.AgentPulseProgressInput{ActiveSprintID: &id, LeaseDurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	ns, err := db.EvaluatePulseGuardianAt(ctx, sp.StartedAt.Add(61*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 || ns[0].Kind != "sprint_overdue" {
		t.Fatalf("deadline was not delivered: %#v", ns)
	}
	again, err := db.EvaluatePulseGuardianAt(ctx, sp.StartedAt.Add(62*time.Second))
	if err != nil || len(again) != 0 {
		t.Fatalf("duplicate: %v %v", again, err)
	}
}

func TestSafeguardGuardianRespectsExtendedAndHeldSprint(t *testing.T) {
	db, _, id := safeguardFixture(t)
	ctx := context.Background()
	sp, err := db.TransitionSprint(ctx, id, "start", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddSprintTimeExtension(ctx, id, model.CreateSprintTimeExtensionInput{DurationSeconds: 60, Reason: "Verified additional work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReportAgentProgress(ctx, "agent:extended", model.AgentPulseProgressInput{ActiveSprintID: &id, LeaseDurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	ns, err := db.EvaluatePulseGuardianAt(ctx, sp.StartedAt.Add(61*time.Second))
	if err != nil || len(ns) != 0 {
		t.Fatalf("premature deadline: %v %v", ns, err)
	}
	if _, err := db.TransitionSprint(ctx, id, "hold", "Waiting for input"); err != nil {
		t.Fatal(err)
	}
	ns, err = db.EvaluatePulseGuardianAt(ctx, sp.StartedAt.Add(2*time.Hour))
	if err != nil || len(ns) != 0 {
		t.Fatalf("held sprint nudged: %v %v", ns, err)
	}
}

func TestSafeguardGuardianDeliversDeadline(t *testing.T) {
	db, _, id := safeguardFixture(t)
	ctx := context.Background()
	var received model.PulseGuardianSignal
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.Header().Set("X-Timekeeper-Pulse-Accepted", "v1")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer callback.Close()
	sp, err := db.TransitionSprint(ctx, id, "start", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReportAgentProgress(ctx, "agent:delivery", model.AgentPulseProgressInput{ActiveSprintID: &id, LeaseDurationSeconds: 3600, GuardianURL: &callback.URL}); err != nil {
		t.Fatal(err)
	}
	result, err := guardian.Tick(ctx, db, sp.StartedAt.Add(61*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.Delivered != 1 || received.Nudge.Kind != "sprint_overdue" || received.Nudge.ActiveSprintID != id {
		t.Fatalf("result=%+v signal=%+v", result, received)
	}
	again, err := guardian.Tick(ctx, db, sp.StartedAt.Add(62*time.Second))
	if err != nil || again.DeliveryAttempts != 0 {
		t.Fatalf("duplicate delivery %+v %v", again, err)
	}
}

func TestSafeguardMigratesLegacyNudges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	ctx := context.Background()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	progress, err := db.ReportAgentProgress(ctx, "agent:legacy", model.AgentPulseProgressInput{LeaseDurationSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	ns, err := db.EvaluatePulseGuardianAt(ctx, progress.LastProgressAt.Add(2*time.Second))
	if err != nil || len(ns) != 1 {
		t.Fatalf("legacy nudge %v %v", ns, err)
	}
	db.Close()
	// Recreate only this disposable fixture with the original CHECK constraint.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := raw.QueryRow("SELECT sql FROM sqlite_master WHERE name='pulse_nudges'").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	schema = strings.Replace(schema, "pulse_nudges", "legacy_nudges", 1)
	// Revert to the original pre-deadline, pre-broadcast schema.
	// The current schema has: 'agent_unresponsive', 'sprint_overdue', 'broadcast'
	// We need to go back to just: 'agent_unresponsive'
	schema = strings.Replace(schema, "'agent_unresponsive', 'sprint_overdue', 'broadcast'", "'agent_unresponsive'", 1)
	schema = strings.Replace(schema, "'agent_unresponsive', 'sprint_overdue'", "'agent_unresponsive'", 1)
	for _, stmt := range []string{schema, "INSERT INTO legacy_nudges SELECT * FROM pulse_nudges", "DROP TABLE pulse_nudges", "ALTER TABLE legacy_nudges RENAME TO pulse_nudges"} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	history, err := db.ListPendingPulseNudges(ctx, "agent:legacy")
	if err != nil || len(history) != 1 || history[0].NudgeID != ns[0].NudgeID {
		t.Fatalf("history lost %v %v", history, err)
	}
	// The same existing store must now accept deadline nudges.
	p, _ := db.CreateProject(ctx, model.CreateProjectInput{Name: "Migrated"})
	c, _ := db.CreateCategory(ctx, p.ProjectID, model.CreateCategoryInput{Name: "C"})
	task, _ := db.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{CategoryID: c.CategoryID, Name: "T"})
	sp, _ := db.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "S", EstimatedDurationSeconds: 1})
	started, err := db.TransitionSprint(ctx, sp.SprintID, "start", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReportAgentProgress(ctx, "agent:new", model.AgentPulseProgressInput{ActiveSprintID: &sp.SprintID, LeaseDurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	got, err := db.EvaluatePulseGuardianAt(ctx, started.StartedAt.Add(2*time.Second))
	if err != nil || len(got) != 1 || got[0].Kind != "sprint_overdue" {
		t.Fatalf("migrated deadline %v %v", got, err)
	}
}
