package store

import (
	"context"
	"errors"
	"github.com/JustSebNL/timekeeper/internal/model"
	"strings"
	"sync"
	"testing"
	"time"
)

func faseWorkFixture(t *testing.T, s *Store) (model.Project, model.Task, model.Sprint) {
	t.Helper()
	p, e := s.CreateProject(t.Context(), model.CreateProjectInput{Name: "FASE claims"})
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.CreateCategory(t.Context(), p.ProjectID, model.CreateCategoryInput{Name: "Work"})
	if e != nil {
		t.Fatal(e)
	}
	task, e := s.CreateTask(t.Context(), p.ProjectID, model.CreateTaskInput{Name: "Implement", CategoryID: c.CategoryID, EstimatedDurationSeconds: 300})
	if e != nil {
		t.Fatal(e)
	}
	sp, e := s.CreateSprint(t.Context(), task.TaskID, model.CreateSprintInput{Name: "Step", EstimatedDurationSeconds: 60})
	if e != nil {
		t.Fatal(e)
	}
	return p, task, sp
}
func faseActor(t *testing.T, s *Store, fingerprint string) (context.Context, model.FASEIdentity) {
	t.Helper()
	id, e := s.ResolveFASEIdentityAt(t.Context(), model.FASEIdentityInput{Fingerprint: strings.Repeat(fingerprint, 64), Runtime: "test", IdentityScope: "runtime-session", Metadata: model.FASEMetadata{Model: "initial-model"}}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	return model.WithFASEActor(t.Context(), id), id
}
func TestFASEClaimsProtectExecutionNotContributions(t *testing.T) {
	s := openPulseStore(t)
	p, task, sp := faseWorkFixture(t, s)
	owner, id := faseActor(t, s, "a")
	other, _ := faseActor(t, s, "b")
	if _, e := s.TransitionSprint(owner, sp.SprintID, "start", ""); e != nil {
		t.Fatal(e)
	}
	sibling, e := s.CreateSprint(other, task.TaskID, model.CreateSprintInput{Name: "Collaborative child", EstimatedDurationSeconds: 60})
	if e != nil {
		t.Fatal(e)
	}
	for _, call := range []func() error{
		func() error { _, e := s.TransitionSprint(other, sp.SprintID, "complete", ""); return e },
		func() error { _, e := s.TransitionSprint(t.Context(), sp.SprintID, "complete", ""); return e },
		func() error { _, e := s.TransitionSprint(other, sibling.SprintID, "start", ""); return e },
	} {
		var conflict *ClaimConflictError
		if e := call(); !errors.As(e, &conflict) {
			t.Fatalf("claim bypass: %v", e)
		}
	}
	if e := s.ReleaseFASEClaim(owner, sp.SprintID); e == nil {
		t.Fatal("released active work")
	}
	if _, e := s.TransitionSprint(owner, sp.SprintID, "hold", "handoff"); e != nil {
		t.Fatal(e)
	}
	if e := s.ReleaseFASEClaim(owner, task.TaskID); e == nil {
		t.Fatal("released parent with owned live child")
	}
	if e := s.ReleaseFASEClaim(owner, sp.SprintID); e != nil {
		t.Fatal(e)
	}
	if e := s.ReleaseFASEClaim(owner, task.TaskID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.TransitionSprint(other, sp.SprintID, "resume", ""); e != nil {
		t.Fatal(e)
	}
	if _, e := s.TransitionSprint(other, sp.SprintID, "complete", ""); e != nil {
		t.Fatal(e)
	}
	events, e := s.ListProjectEvents(t.Context(), p.ProjectID)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, event := range events {
		if event.EventType == "sprint_started" {
			found = true
			if event.Actor == nil || event.Actor.AgentKey != id.AgentKey || event.Actor.Metadata.Model != "initial-model" {
				t.Fatalf("missing immutable actor: %+v", event)
			}
		}
	}
	if !found {
		t.Fatal("missing start event")
	}
}
func TestFASEConcurrentClaimsHaveOneWinner(t *testing.T) {
	s := openPulseStore(t)
	_, _, sp := faseWorkFixture(t, s)
	a, _ := faseActor(t, s, "c")
	b, _ := faseActor(t, s, "d")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, ctx := range []context.Context{a, b} {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.ClaimFASEItem(ctx, sp.SprintID); results <- e }()
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for e := range results {
		if e == nil {
			wins++
		} else {
			var conflict *ClaimConflictError
			if !errors.As(e, &conflict) {
				t.Fatal(e)
			}
			conflicts++
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
}

func TestFASEProfileFiltersMonthAndPreservesEventMetadata(t *testing.T) {
	s := openPulseStore(t)
	p, _, _ := faseWorkFixture(t, s)
	september := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	in := model.FASEIdentityInput{Fingerprint: strings.Repeat("e", 64), Runtime: "test", IdentityScope: "runtime-session", Metadata: model.FASEMetadata{Model: "old-model"}}
	first, e := s.ResolveFASEIdentityAt(t.Context(), in, september)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := s.db.BeginTx(t.Context(), nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = recordProjectEvent(model.WithFASEActor(t.Context(), first), tx, p.ProjectID, "project", p.ProjectID, "test_event", "September work", september); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	in.Metadata.Model = "new-model"
	october, e := s.ResolveFASEIdentityAt(t.Context(), in, september.AddDate(0, 1, 0))
	if e != nil {
		t.Fatal(e)
	}
	tx, e = s.db.BeginTx(t.Context(), nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = recordProjectEvent(model.WithFASEActor(t.Context(), october), tx, p.ProjectID, "project", p.ProjectID, "test_event", "October work", september.AddDate(0, 1, 0)); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	profile, e := s.FASEProfile(t.Context(), first.FASEID)
	if e != nil {
		t.Fatal(e)
	}
	if profile.Identity.FASEID != first.FASEID || len(profile.Events) != 1 || profile.Events[0].Actor.Metadata.Model != "old-model" || profile.Identity.Metadata.Model != "new-model" {
		t.Fatalf("month or history corrupted: %+v", profile)
	}
}
