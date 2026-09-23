// Package stuck implements a stuck detector for guardian work cycles.
// It monitors progress signals and escalates when an agent appears stuck.
//
// Pattern source: experiments-autonomous-speedrunning (dual-threshold escalation).
// Unlike the original, this detector is pure Go with no external dependencies.
package stuck

import (
	"fmt"
	"sync"
	"time"
)

// SignalType classifies the kind of progress signal being tracked.
type SignalType string

const (
	// SignalWorkCycle is emitted when a work cycle completes.
	SignalWorkCycle SignalType = "work_cycle"
	// SignalBuildPass is emitted when a build succeeds.
	SignalBuildPass SignalType = "build_pass"
	// SignalTestPass is emitted when tests pass.
	SignalTestPass SignalType = "test_pass"
	// SignalHeartbeat is emitted when the agent checks in.
	SignalHeartbeat SignalType = "heartbeat"
	// SignalError is emitted on any error.
	SignalError SignalType = "error"
)

// Signal is a single progress observation.
type Signal struct {
	Type      SignalType
	Timestamp time.Time
	Metadata  map[string]string
}

// Detector tracks agent progress and detects stuck conditions.
//
// The detector uses dual-threshold escalation:
// - Level 1 (investigate): no progress for investigateAfter duration
// - Level 2 (pivot): no progress for pivotAfter duration (mandatory pivot)
//
// A noise-floor gate prevents false positives when too few signals exist.
type Detector struct {
	mu sync.Mutex
	// Config
	investigateAfter time.Duration
	pivotAfter       time.Duration
	noiseFloorMin    int    // minimum signals before escalation activates
	noiseFloorMargin float64 // margin multiplier (e.g. 2.0 = 2x expected)
	// State
	signals    []Signal
	lastSignal time.Time
	// Escalation callback
	onEscalate func(level int, reason string)
}

// Config configures the stuck detector.
type Config struct {
	// InvestigateAfter is the duration without progress before Level 1 escalation.
	InvestigateAfter time.Duration
	// PivotAfter is the duration without progress before Level 2 (mandatory pivot).
	PivotAfter time.Duration
	// NoiseFloorMin is the minimum number of signals before escalation activates.
	NoiseFloorMin int
	// NoiseFloorMargin is the multiplier for the noise floor (default 2.0).
	NoiseFloorMargin float64
	// OnEscalate is called when escalation fires (level 1 or 2).
	OnEscalate func(level int, reason string)
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		InvestigateAfter: 5 * time.Minute,
		PivotAfter:        15 * time.Minute,
		NoiseFloorMin:     3,
		NoiseFloorMargin:  2.0,
	}
}

// NewDetector creates a new stuck detector.
func NewDetector(cfg Config) *Detector {
	if cfg.InvestigateAfter <= 0 {
		cfg.InvestigateAfter = 5 * time.Minute
	}
	if cfg.PivotAfter <= 0 {
		cfg.PivotAfter = 15 * time.Minute
	}
	if cfg.NoiseFloorMin <= 0 {
		cfg.NoiseFloorMin = 3
	}
	if cfg.NoiseFloorMargin <= 0 {
		cfg.NoiseFloorMargin = 2.0
	}
	return &Detector{
		investigateAfter: cfg.InvestigateAfter,
		pivotAfter:        cfg.PivotAfter,
		noiseFloorMin:     cfg.NoiseFloorMin,
		noiseFloorMargin:  cfg.NoiseFloorMargin,
		onEscalate:       cfg.OnEscalate,
	}
}

// Record adds a signal to the detector.
func (d *Detector) Record(sig Signal) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if sig.Timestamp.IsZero() {
		sig.Timestamp = time.Now()
	}
	d.signals = append(d.signals, sig)
	d.lastSignal = sig.Timestamp
}

// RecordNow creates and records a signal with the current timestamp.
func (d *Detector) RecordNow(sigType SignalType, metadata map[string]string) {
	d.Record(Signal{Type: sigType, Timestamp: time.Now(), Metadata: metadata})
}

// Check evaluates the current stuck state and fires escalation if needed.
// Returns (level, reason) where level is 0 (ok), 1 (investigate), or 2 (pivot).
func (d *Detector) Check() (int, string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.signals) == 0 {
		return 0, "no signals yet"
	}

	// Noise floor gate: not enough signals to make a decision
	if len(d.signals) < d.noiseFloorMin {
		return 0, "insufficient signals for escalation"
	}

	// Check if we have at least noiseFloorMargin × investigateAfter worth of data
	expectedDuration := time.Duration(float64(d.investigateAfter) * d.noiseFloorMargin)
	if len(d.signals) > 0 {
		oldest := d.signals[0].Timestamp
		if time.Since(oldest) < expectedDuration {
			return 0, "within noise floor window"
		}
	}

	// Check for progress since last escalation — find the most recent non-error signal
	var lastProgress time.Time
	if len(d.signals) > 0 {
		for i := len(d.signals) - 1; i >= 0; i-- {
			if d.signals[i].Type != SignalError {
				lastProgress = d.signals[i].Timestamp
				break
			}
		}
	}

	now := time.Now()
	stuckDuration := now.Sub(lastProgress)

	// Level 2: mandatory pivot
	if stuckDuration >= d.pivotAfter {
		if d.onEscalate != nil {
			d.onEscalate(2, fmt.Sprintf("no progress for %s (threshold %s)", stuckDuration.Round(time.Second), d.pivotAfter))
		}
		return 2, "mandatory pivot required"
	}

	// Level 1: investigate
	if stuckDuration >= d.investigateAfter {
		if d.onEscalate != nil {
			d.onEscalate(1, fmt.Sprintf("no progress for %s (threshold %s)", stuckDuration.Round(time.Second), d.investigateAfter))
		}
		return 1, "investigate"
	}

	return 0, "progressing"
}

// Reset clears all signals (used after a successful intervention).
func (d *Detector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.signals = nil
	d.lastSignal = time.Time{}
}

// Stats returns current detector statistics.
type Stats struct {
	SignalCount   int
	LastSignal    time.Time
	StuckDuration time.Duration
}

// Stats returns current detector statistics.
func (d *Detector) Stats() Stats {
	d.mu.Lock()
	defer d.mu.Unlock()
	stats := Stats{SignalCount: len(d.signals)}
	if len(d.signals) > 0 {
		stats.LastSignal = d.lastSignal
		stats.StuckDuration = time.Since(d.lastSignal)
	}
	return stats
}
