// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardShowsGuardianRecoveryStateAndRetrievalAttempts(t *testing.T) {
	for path, markers := range map[string][]string{
		filepath.Join("..", "..", "web", "index.html"):    {`id="guardian"`, "Pulse Guardian"},
		filepath.Join("..", "..", "web", "timekeeper.js"): {"/api/v1/guardian/status", "function sprintRetrievalAttemptPanel", "/retrieval-attempts", "Open: ['start', 'hold', 'cancel']", "Attention beyond Pulse"},
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(body), marker) {
				t.Fatalf("%s missing %q", path, marker)
			}
		}
	}
}
