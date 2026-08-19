// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalLauncherCanEnablePulseGuardianWithoutLeavingTheClone(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "run-local.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, marker := range []string{"TIMEKEEPER_PULSE_GUARDIAN_INTERVAL", "-pulse-guardian-interval"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("launcher missing %q", marker)
		}
	}
}
