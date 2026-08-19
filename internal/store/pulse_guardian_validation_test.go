// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestPulseGuardianRejectsNonLocalCallbackTargets(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	invalid := []string{
		"https://127.0.0.1:19090/pulse",
		"http://localhost:19090/pulse",
		"http://192.168.1.10:19090/pulse",
		"http://127.0.0.1/pulse",
		"http://user:pass@127.0.0.1:19090/pulse",
		"http://127.0.0.1:19090/pulse?token=nope",
		"http://127.0.0.1:19090/pulse#fragment",
	}
	for _, callbackURL := range invalid {
		t.Run(callbackURL, func(t *testing.T) {
			_, err := database.ReportAgentProgress(context.Background(), "agent:focus", model.AgentPulseProgressInput{
				LeaseDurationSeconds: 20,
				GuardianURL:          &callbackURL,
			})
			if err == nil {
				t.Fatalf("guardian_url %q was accepted", callbackURL)
			}
		})
	}

	valid := []string{
		"http://127.0.0.1:19090/pulse",
		"http://[::1]:19090/pulse",
	}
	for _, callbackURL := range valid {
		t.Run(callbackURL, func(t *testing.T) {
			_, err := database.ReportAgentProgress(context.Background(), "agent:"+callbackURL, model.AgentPulseProgressInput{
				LeaseDurationSeconds: 20,
				GuardianURL:          &callbackURL,
			})
			if err != nil {
				t.Fatalf("guardian_url %q rejected: %v", callbackURL, err)
			}
		})
	}
}
