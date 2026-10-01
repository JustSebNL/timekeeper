package store

import (
	"github.com/JustSebNL/timekeeper/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestAgentActivitySurvivesRestartAndAcknowledgementDoesNotMeanProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Minute)
	original, err := s.RecordAgentActivityAt(t.Context(), "worker", model.AgentActivityInput{SessionID: "session", Event: "work", LeaseDurationSeconds: 1}, at)
	if err != nil {
		t.Fatal(err)
	}
	nudges, err := s.EvaluatePulseGuardianAt(t.Context(), at.Add(2*time.Second))
	if err != nil || len(nudges) != 1 {
		t.Fatalf("%+v %v", nudges, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.AcknowledgePulseNudge(t.Context(), nudges[0].NudgeID, "worker"); err != nil {
		t.Fatal(err)
	}
	state, err := s.AgentWorkStateAt(t.Context(), "worker", time.Now())
	if err != nil || state.State != "idle" || !state.LastProgressAt.Equal(original.LastProgressAt) {
		t.Fatalf("receipt manufactured progress: %+v %v", state, err)
	}
}

func TestAgentActivityEnrollmentPreservesLegacyLease(t *testing.T) {
	s := openPulseStore(t)
	legacy, err := s.ReportAgentProgress(t.Context(), "legacy", model.AgentPulseProgressInput{LeaseDurationSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.RecordAgentActivityAt(t.Context(), "legacy", model.AgentActivityInput{SessionID: "session", Event: "contact"}, legacy.LastProgressAt.Add(3*time.Second))
	if err != nil || state.State != "idle" || !state.WorkExpected || !state.LastProgressAt.Equal(legacy.LastProgressAt) {
		t.Fatalf("enrollment disarmed existing work: %+v %v", state, err)
	}
}

func TestAgentActivitySeparatesContactFromProgress(t *testing.T) {
	s := openPulseStore(t)
	ctx := t.Context()
	at := time.Now().UTC()
	for _, id := range []string{"alpha", "beta"} {
		_, err := s.RecordAgentActivityAt(ctx, id, model.AgentActivityInput{SessionID: id + "-session", Runtime: "custom", Event: "work", LeaseDurationSeconds: 10}, at)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.RecordAgentActivityAt(ctx, "alpha", model.AgentActivityInput{SessionID: "alpha-session", Event: "contact"}, at.Add(11*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RecordAgentActivityAt(ctx, "beta", model.AgentActivityInput{SessionID: "beta-session", Event: "progress"}, at.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := s.AgentWorkStateAt(ctx, "alpha", at.Add(12*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	beta, err := s.AgentWorkStateAt(ctx, "beta", at.Add(12*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if alpha.State != "idle" || beta.State != "working" {
		t.Fatalf("alpha=%+v beta=%+v", alpha, beta)
	}
	if !alpha.LastContactAt.After(alpha.LastProgressAt) {
		t.Fatal("contact did not remain distinct from progress")
	}
	nudges, err := s.EvaluatePulseGuardianAt(ctx, at.Add(12*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(nudges) != 1 || nudges[0].AgentID != "alpha" {
		t.Fatalf("nudges=%+v", nudges)
	}
}

func TestAgentActivityWaitExpiresAndStopDisarms(t *testing.T) {
	s := openPulseStore(t)
	ctx := t.Context()
	at := time.Now().UTC()
	_, err := s.RecordAgentActivityAt(ctx, "worker", model.AgentActivityInput{SessionID: "session", Event: "work", LeaseDurationSeconds: 10}, at)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RecordAgentActivityAt(ctx, "worker", model.AgentActivityInput{SessionID: "session", Event: "wait", WaitDurationSeconds: 60, Reason: "awaiting approval"}, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.AgentWorkStateAt(ctx, "worker", at.Add(30*time.Second))
	if err != nil || state.State != "waiting" {
		t.Fatalf("%+v %v", state, err)
	}
	nudges, err := s.EvaluatePulseGuardianAt(ctx, at.Add(30*time.Second))
	if err != nil || len(nudges) != 0 {
		t.Fatalf("%+v %v", nudges, err)
	}
	nudges, err = s.EvaluatePulseGuardianAt(ctx, at.Add(62*time.Second))
	if err != nil || len(nudges) != 1 {
		t.Fatalf("expired wait: %+v %v", nudges, err)
	}
	_, err = s.RecordAgentActivityAt(ctx, "worker", model.AgentActivityInput{SessionID: "session", Event: "stop", Reason: "finished authorized work"}, at.Add(63*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	state, err = s.AgentWorkStateAt(ctx, "worker", at.Add(time.Hour))
	if err != nil || state.State != "stopped" {
		t.Fatalf("%+v %v", state, err)
	}
	deliveries, err := s.ListPendingPulseNudgeDeliveries(ctx)
	if err != nil || len(deliveries) != 0 {
		t.Fatalf("stopped work delivered: %+v %v", deliveries, err)
	}
}

func TestAgentActivityIdentityCollisionAndReplay(t *testing.T) {
	s := openPulseStore(t)
	ctx := t.Context()
	at := time.Now().UTC()
	input := model.AgentActivityInput{SessionID: "one", Event: "work", EventID: "event-1", LeaseDurationSeconds: 10}
	if _, err := s.RecordAgentActivityAt(ctx, "agent", input, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordAgentActivityAt(ctx, "agent", input, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	state, err := s.AgentWorkStateAt(ctx, "agent", at.Add(time.Hour))
	if err != nil || state.State != "idle" {
		t.Fatalf("replay manufactured progress: %+v %v", state, err)
	}
	if _, err := s.RecordAgentActivityAt(ctx, "agent", model.AgentActivityInput{SessionID: "two", Event: "work"}, at); err == nil {
		t.Fatal("another session overwrote agent identity")
	}
	if _, err := s.RecordAgentActivityAt(ctx, "agent", model.AgentActivityInput{SessionID: "one", Event: "wait"}, at); err == nil {
		t.Fatal("unbounded unexplained wait accepted")
	}
}

func TestAgentActivityProgressSupersedesOldNudge(t *testing.T) {
	s := openPulseStore(t)
	ctx := t.Context()
	at := time.Now().UTC()
	_, err := s.RecordAgentActivityAt(ctx, "worker", model.AgentActivityInput{SessionID: "one", Event: "work", LeaseDurationSeconds: 1}, at)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.EvaluatePulseGuardianAt(ctx, at.Add(2*time.Second))
	if err != nil || len(first) != 1 {
		t.Fatalf("%+v %v", first, err)
	}
	_, err = s.RecordAgentActivityAt(ctx, "worker", model.AgentActivityInput{SessionID: "one", Event: "progress"}, at.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	history, err := s.ListPulseNudgeHistory(ctx, "worker")
	if err != nil || len(history) != 1 || history[0].Status != "Superseded" || history[0].AcknowledgedAt != nil {
		t.Fatalf("false acknowledgement or stale nudge: %+v %v", history, err)
	}
	next, err := s.EvaluatePulseGuardianAt(ctx, at.Add(5*time.Second))
	if err != nil || len(next) != 1 || next[0].NudgeID == first[0].NudgeID {
		t.Fatalf("new inactivity blocked by old nudge: %+v %v", next, err)
	}
}
