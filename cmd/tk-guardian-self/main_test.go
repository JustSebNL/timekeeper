package main

import (
	"errors"
	"testing"
)

func TestNudgeIsNotAcknowledgedWhenWorkCycleFails(t *testing.T) {
	if shouldAcknowledgeWorkCycle(errors.New("adapter failed")) {
		t.Fatal("failed work cycle must leave the nudge pending")
	}
	if !shouldAcknowledgeWorkCycle(nil) {
		t.Fatal("successful work cycle must be acknowledged")
	}
}
