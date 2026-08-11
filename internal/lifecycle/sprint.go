// Copyright (c) 2026 Seb. All rights reserved.

// Package lifecycle defines the legal state changes for tracked work.
package lifecycle

import (
	"fmt"
	"sort"
)

// SprintTransitionRule describes the state and timer effects of one Sprint action.
type SprintTransitionRule struct {
	From          string
	Action        string
	To            string
	CloseInterval string
	OpenInterval  string
}

var sprintTransitions = map[string]SprintTransitionRule{
	"Open:start": {
		From: "Open", Action: "start", To: "Active", OpenInterval: "work",
	},
	"Active:hold": {
		From: "Active", Action: "hold", To: "On Hold", CloseInterval: "work", OpenInterval: "hold",
	},
	"On Hold:resume": {
		From: "On Hold", Action: "resume", To: "Active", CloseInterval: "hold", OpenInterval: "work",
	},
	"Active:complete": {
		From: "Active", Action: "complete", To: "Completed", CloseInterval: "work",
	},
}

// TransitionError reports an action that is invalid for the Sprint's current state.
type TransitionError struct {
	Status         string
	Action         string
	AllowedActions []string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("cannot %s sprint in status %s", e.Action, e.Status)
}

// SprintTransition returns the sole legal rule for a state/action pair.
func SprintTransition(status, action string) (SprintTransitionRule, error) {
	if rule, ok := sprintTransitions[status+":"+action]; ok {
		return rule, nil
	}
	return SprintTransitionRule{}, &TransitionError{
		Status:         status,
		Action:         action,
		AllowedActions: AllowedSprintActions(status),
	}
}

// AllowedSprintActions returns legal actions in stable lexical order.
func AllowedSprintActions(status string) []string {
	actions := make([]string, 0)
	for _, rule := range sprintTransitions {
		if rule.From == status {
			actions = append(actions, rule.Action)
		}
	}
	sort.Strings(actions)
	return actions
}
