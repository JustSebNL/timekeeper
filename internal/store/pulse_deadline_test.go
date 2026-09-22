package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// openPulseStore builds an isolated store for one test.
func openPulseStore(t *testing.T) *Store {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "pulse.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// TestNextPulseDeadlineHonoursStoredLease is the point of the change:
// the Guardian must wake on the lease stored in the database, not on a
// fixed global interval. A 30-second lease must be detected after 30
// seconds, not whenever a five-minute tick happens to land.
func TestNextPulseDeadlineHonoursStoredLease(t *testing.T) {
	database := openPulseStore(t)
	ctx := context.Background()

	// No agents registered: nothing to wait for.
	if _, ok, err := database.NextPulseDeadline(ctx, time.Now()); err != nil {
		t.Fatalf("NextPulseDeadline: %v", err)
	} else if ok {
		t.Fatalf("deadline reported with no agents registered")
	}

	// A bare lease, with no callback and no active sprint. This is the
	// case an earlier filter missed entirely, which would have left the
	// timer running out with nothing watching it.
	if _, err := database.ReportAgentProgress(ctx, "bare", model.AgentPulseProgressInput{
		LeaseDurationSeconds: 30,
	}); err != nil {
		t.Fatalf("ReportAgentProgress: %v", err)
	}

	deadline, ok, err := database.NextPulseDeadline(ctx, time.Now())
	if err != nil {
		t.Fatalf("NextPulseDeadline: %v", err)
	}
	if !ok {
		t.Fatalf("bare lease was not found; the Guardian would never notice it")
	}

	// 30s lease plus its 5%% grace, so about 31.5s.
	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.Fatalf("deadline already passed: %v", remaining)
	}
	if remaining > 32*time.Second {
		t.Fatalf("remaining = %v, want about 31.5s (lease plus grace)", remaining)
	}
}

// TestNextPulseDeadlineUsesSoonest proves a short lease is not masked by
// a longer one: the Guardian wakes on the earliest expiry.
func TestNextPulseDeadlineUsesSoonest(t *testing.T) {
	database := openPulseStore(t)
	ctx := context.Background()

	for id, lease := range map[string]int64{"long": 3600, "short": 20, "mid": 300} {
		if _, err := database.ReportAgentProgress(ctx, id, model.AgentPulseProgressInput{
			LeaseDurationSeconds: lease,
		}); err != nil {
			t.Fatalf("ReportAgentProgress %s: %v", id, err)
		}
	}

	deadline, ok, err := database.NextPulseDeadline(ctx, time.Now())
	if err != nil {
		t.Fatalf("NextPulseDeadline: %v", err)
	}
	if !ok {
		t.Fatalf("no deadline found")
	}
	if remaining := time.Until(deadline); remaining > 21*time.Second {
		t.Fatalf("remaining = %v, want about 20s (the shortest lease)", remaining)
	}
}

// TestGraceScalesWithLease proves the tolerance is proportional: a
// short lease gets a small grace, a long one a proportionally larger
// grace. A single fixed slack would be either twitchy for short leases
// or far too lax for long ones.
func TestGraceScalesWithLease(t *testing.T) {
	cases := []struct {
		lease int64
		want  time.Duration
	}{
		{30, 1500 * time.Millisecond}, // 5% of 30s
		{3600, 3 * time.Minute},       // 5% of an hour
		{1, 50 * time.Millisecond},    // 5% of a second
	}
	for _, tc := range cases {
		got := graceFor(tc.lease)
		if got < tc.want-10*time.Millisecond || got > tc.want+10*time.Millisecond {
			t.Fatalf("graceFor(%ds) = %v, want ~%v", tc.lease, got, tc.want)
		}
	}
}

// TestExpiredLeaseStillPulsesAfterGrace proves the grace period delays
// detection but does not prevent it: once the grace has also elapsed,
// the deadline is in the past.
func TestExpiredLeaseStillPulsesAfterGrace(t *testing.T) {
	database := openPulseStore(t)
	ctx := context.Background()

	if _, err := database.ReportAgentProgress(ctx, "lapsed", model.AgentPulseProgressInput{
		LeaseDurationSeconds: 1,
	}); err != nil {
		t.Fatalf("ReportAgentProgress: %v", err)
	}

	deadline, ok, err := database.NextPulseDeadline(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("NextPulseDeadline: %v", err)
	}
	if !ok {
		t.Fatalf("expired lease was not reported")
	}
	// Well past the 1s lease plus its 50ms grace.
	time.Sleep(1200 * time.Millisecond)
	if time.Until(deadline) > 0 {
		t.Fatalf("deadline still in the future after lease and grace elapsed")
	}
}

// TestParseTimestampCoversStoredFormats guards the reader against the
// formats SQLite can produce.
func TestParseTimestampCoversStoredFormats(t *testing.T) {
	cases := []string{
		"2026-09-22 21:53:13.4162432+00:00",
		"2026-09-22T21:53:13.4162432Z",
		"2026-09-22T21:53:13Z",
		"2026-09-22 21:53:13",
	}
	for _, raw := range cases {
		got, err := parseTimestamp(raw)
		if err != nil {
			t.Fatalf("parseTimestamp(%q): %v", raw, err)
		}
		if got.Year() != 2026 {
			t.Fatalf("parseTimestamp(%q) year = %d", raw, got.Year())
		}
	}
	if _, err := parseTimestamp("not-a-time"); err == nil {
		t.Fatalf("parseTimestamp accepted garbage")
	}
}
