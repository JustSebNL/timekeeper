package stuck

import (
	"testing"
	"time"
)

func TestNewDetector(t *testing.T) {
	d := NewDetector(DefaultConfig())
	if d == nil {
		t.Fatal("NewDetector returned nil")
	}
	if d.investigateAfter != 5*time.Minute {
		t.Errorf("expected 5m investigateAfter, got %v", d.investigateAfter)
	}
	if d.pivotAfter != 15*time.Minute {
		t.Errorf("expected 15m pivotAfter, got %v", d.pivotAfter)
	}
}

func TestRecordAndCheck(t *testing.T) {
	d := NewDetector(DefaultConfig())

	// No signals yet
	level, reason := d.Check()
	if level != 0 {
		t.Errorf("expected level 0 with no signals, got %d", level)
	}

	// Record a few signals
	d.RecordNow(SignalHeartbeat, nil)
	d.RecordNow(SignalWorkCycle, nil)
	d.RecordNow(SignalBuildPass, nil)

	// Should be ok — signals are recent
	level, reason = d.Check()
	if level != 0 {
		t.Errorf("expected level 0 with recent signals, got %d (%s)", level, reason)
	}
}

func TestNoiseFloorGate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoiseFloorMin = 5
	d := NewDetector(cfg)

	// Record 4 signals (below noise floor of 5)
	for i := 0; i < 4; i++ {
		d.RecordNow(SignalHeartbeat, nil)
	}

	// Should be ok — not enough signals for escalation
	level, _ := d.Check()
	if level != 0 {
		t.Error("expected level 0 when below noise floor")
	}
}

func TestInvestigateEscalation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InvestigateAfter = 100 * time.Millisecond
	cfg.PivotAfter = 300 * time.Millisecond // higher than noise floor
	cfg.NoiseFloorMin = 2

	escalations := make(chan int, 10)
	cfg.OnEscalate = func(level int, reason string) {
		escalations <- level
	}

	d := NewDetector(cfg)

	// Record signals old enough to pass noise floor (200ms) but not trigger pivot (300ms)
	oldTime := time.Now().Add(-250 * time.Millisecond)
	d.Record(Signal{Type: SignalHeartbeat, Timestamp: oldTime})
	d.Record(Signal{Type: SignalWorkCycle, Timestamp: oldTime.Add(10 * time.Millisecond)})

	// Should trigger level 1 (investigate)
	level, reason := d.Check()
	if level != 1 {
		t.Errorf("expected level 1 (investigate), got %d (%s)", level, reason)
	}

	// Check escalation callback fired
	select {
	case escalated := <-escalations:
		if escalated != 1 {
			t.Errorf("expected escalation level 1, got %d", escalated)
		}
	default:
		t.Error("expected escalation callback to fire")
	}
}

func TestPivotEscalation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InvestigateAfter = 50 * time.Millisecond
	cfg.PivotAfter = 100 * time.Millisecond
	cfg.NoiseFloorMin = 2

	escalations := make(chan int, 10)
	cfg.OnEscalate = func(level int, reason string) {
		escalations <- level
	}

	d := NewDetector(cfg)

	// Record signals with very old timestamps
	oldTime := time.Now().Add(-5 * time.Minute)
	d.Record(Signal{Type: SignalHeartbeat, Timestamp: oldTime})
	d.Record(Signal{Type: SignalWorkCycle, Timestamp: oldTime.Add(1 * time.Second)})

	// Should trigger level 2 (pivot)
	level, reason := d.Check()
	if level != 2 {
		t.Errorf("expected level 2 (pivot), got %d (%s)", level, reason)
	}

	// Check escalation callback fired with level 2
	select {
	case escalated := <-escalations:
		if escalated != 2 {
			t.Errorf("expected escalation level 2, got %d", escalated)
		}
	default:
		t.Error("expected escalation callback to fire")
	}
}

func TestReset(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InvestigateAfter = 50 * time.Millisecond
	cfg.PivotAfter = 100 * time.Millisecond
	cfg.NoiseFloorMin = 2

	d := NewDetector(cfg)

	oldTime := time.Now().Add(-5 * time.Minute)
	d.Record(Signal{Type: SignalHeartbeat, Timestamp: oldTime})
	d.Record(Signal{Type: SignalWorkCycle, Timestamp: oldTime.Add(1 * time.Second)})

	// Confirm escalation fires
	level, _ := d.Check()
	if level != 2 {
		t.Fatalf("expected level 2 before reset, got %d", level)
	}

	// Reset
	d.Reset()

	// Should be ok now
	level, _ = d.Check()
	if level != 0 {
		t.Error("expected level 0 after reset")
	}

	stats := d.Stats()
	if stats.SignalCount != 0 {
		t.Errorf("expected 0 signals after reset, got %d", stats.SignalCount)
	}
}

func TestErrorSignalsDoNotCountAsProgress(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InvestigateAfter = 100 * time.Millisecond
	cfg.PivotAfter = 200 * time.Millisecond
	cfg.NoiseFloorMin = 2

	d := NewDetector(cfg)

	// Record old progress signal
	oldTime := time.Now().Add(-1 * time.Minute)
	d.Record(Signal{Type: SignalWorkCycle, Timestamp: oldTime})

	// Record recent error signals — these should NOT count as progress
	d.RecordNow(SignalError, map[string]string{"reason": "build failed"})
	d.RecordNow(SignalError, map[string]string{"reason": "test failed"})

	// Should still escalate because last non-error signal was old
	level, _ := d.Check()
	if level == 0 {
		t.Error("expected escalation when only errors since last progress")
	}
}

func TestRecentProgressPreventsEscalation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InvestigateAfter = 100 * time.Millisecond
	cfg.PivotAfter = 200 * time.Millisecond
	cfg.NoiseFloorMin = 2

	d := NewDetector(cfg)

	// Record old progress signal
	oldTime := time.Now().Add(-1 * time.Minute)
	d.Record(Signal{Type: SignalWorkCycle, Timestamp: oldTime})

	// Record recent progress — should prevent escalation
	d.RecordNow(SignalBuildPass, nil)

	// Should be ok because of recent progress
	level, _ := d.Check()
	if level != 0 {
		t.Error("expected level 0 with recent progress signal")
	}
}

func TestStats(t *testing.T) {
	d := NewDetector(DefaultConfig())

	d.RecordNow(SignalHeartbeat, nil)
	d.RecordNow(SignalWorkCycle, nil)

	stats := d.Stats()
	if stats.SignalCount != 2 {
		t.Errorf("expected 2 signals, got %d", stats.SignalCount)
	}
	if stats.LastSignal.IsZero() {
		t.Error("expected non-zero LastSignal")
	}
}

func TestConcurrency(t *testing.T) {
	d := NewDetector(DefaultConfig())

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				d.RecordNow(SignalHeartbeat, nil)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	stats := d.Stats()
	if stats.SignalCount != 500 {
		t.Errorf("expected 500 signals, got %d", stats.SignalCount)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			d.Check()
			d.Stats()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.InvestigateAfter != 5*time.Minute {
		t.Errorf("expected 5m, got %v", cfg.InvestigateAfter)
	}
	if cfg.PivotAfter != 15*time.Minute {
		t.Errorf("expected 15m, got %v", cfg.PivotAfter)
	}
	if cfg.NoiseFloorMin != 3 {
		t.Errorf("expected 3, got %d", cfg.NoiseFloorMin)
	}
	if cfg.NoiseFloorMargin != 2.0 {
		t.Errorf("expected 2.0, got %f", cfg.NoiseFloorMargin)
	}
}

func BenchmarkRecord(b *testing.B) {
	d := NewDetector(DefaultConfig())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.RecordNow(SignalHeartbeat, nil)
	}
}

func BenchmarkCheck(b *testing.B) {
	d := NewDetector(DefaultConfig())
	for i := 0; i < 100; i++ {
		d.RecordNow(SignalHeartbeat, nil)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Check()
	}
}
