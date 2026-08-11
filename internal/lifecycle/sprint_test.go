// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package lifecycle_test

import (
	"testing"

	"github.com/JustSebNL/timekeeper/internal/lifecycle"
)

func TestSprintTransitionPolicyDefinesLegalTransitions(t *testing.T) {
	cases := []struct {
		from, action, to string
		closeInterval    string
		openInterval     string
	}{
		{from: "Open", action: "start", to: "Active", openInterval: "work"},
		{from: "Active", action: "hold", to: "On Hold", closeInterval: "work", openInterval: "hold"},
		{from: "On Hold", action: "resume", to: "Active", closeInterval: "hold", openInterval: "work"},
		{from: "Active", action: "complete", to: "Completed", closeInterval: "work"},
	}

	for _, tc := range cases {
		t.Run(tc.from+"_"+tc.action, func(t *testing.T) {
			rule, err := lifecycle.SprintTransition(tc.from, tc.action)
			if err != nil {
				t.Fatalf("SprintTransition(%q, %q): %v", tc.from, tc.action, err)
			}
			if rule.To != tc.to || rule.CloseInterval != tc.closeInterval || rule.OpenInterval != tc.openInterval {
				t.Fatalf("rule = %#v, want to=%q close=%q open=%q", rule, tc.to, tc.closeInterval, tc.openInterval)
			}
		})
	}
}
