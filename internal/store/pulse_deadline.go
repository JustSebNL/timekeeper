package store

import (
	"context"
	"time"
)

// graceFraction is the tolerance applied to a lease before the Guardian
// pulses.
//
// A lease is a declaration of intent, not a hard wall: an agent that is
// mid-step should not be nudged the instant its timer hits zero. Five
// percent scales with the lease, so a 30-second lease gets 1.5s while an
// hour-long lease gets 3 minutes, instead of one fixed slack that is
// either too twitchy for short leases or too lax for long ones.
const graceFraction = 0.05

// graceFor returns the tolerance for a lease duration.
func graceFor(leaseSeconds int64) time.Duration {
	d := time.Duration(leaseSeconds) * time.Second
	return time.Duration(float64(d) * graceFraction)
}

// NextPulseDeadline reports when the next agent lease expires,
// including its grace period.
//
// The Guardian used to wake on a fixed global interval, which meant a
// short lease could not be honoured: a 30-second lease would only be
// noticed up to five minutes late. Waking on the soonest stored deadline
// makes detection follow the data rather than a separate timetable.
//
// It returns the deadline and true when at least one lease exists. When
// no agent is registered there is nothing to wait for, and the caller
// falls back to its interval.
func (s *Store) NextPulseDeadline(ctx context.Context, now time.Time) (time.Time, bool, error) {
	states, err := s.ListAgentWorkStatesAt(ctx, now)
	if err != nil {
		return time.Time{}, false, err
	}
	stateByAgent := make(map[string]*time.Time, len(states))
	for _, state := range states {
		stateByAgent[state.AgentID] = state.Deadline
	}
	// Every registered lease counts, not only those with a callback or
	// an active sprint. Filtering to those would make a bare lease
	// invisible to the Guardian, so its timer would run out and nothing
	// would ever notice.
	const query = `
		SELECT agent_id, last_progress_at, lease_duration_seconds
		FROM agent_pulse_progress`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return time.Time{}, false, err
	}
	defer rows.Close()

	var soonest time.Time
	found := false
	for rows.Next() {
		var agentID, lastProgress string
		var leaseSeconds int64
		if err := rows.Scan(&agentID, &lastProgress, &leaseSeconds); err != nil {
			return time.Time{}, false, err
		}
		at, err := parseTimestamp(lastProgress)
		if err != nil {
			continue // an unreadable row must not stall the whole loop
		}
		deadline := at.
			Add(time.Duration(leaseSeconds) * time.Second).
			Add(graceFor(leaseSeconds))
		if stateDeadline, known := stateByAgent[agentID]; known {
			if stateDeadline == nil {
				continue
			}
			deadline = *stateDeadline
		}
		if !found || deadline.Before(soonest) {
			soonest = deadline
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		return time.Time{}, false, err
	}
	if !found {
		return time.Time{}, false, nil
	}
	return soonest, true, nil
}

// parseTimestamp reads the stored timestamp format.
func parseTimestamp(raw string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05.999999999Z07:00",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	}
	var lastErr error
	for _, layout := range layouts {
		t, err := time.Parse(layout, raw)
		if err == nil {
			// Normalise to UTC. Stored timestamps are UTC while
			// time.Now() is local; comparing the two directly would
			// make an already-expired lease look like it is still in
			// the future, so the Guardian would never pulse on it.
			return t.UTC(), nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}
