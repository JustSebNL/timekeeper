// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package model

// ProjectAttention is a read-only local view of work that needs an explicit
// decision even when Pulse is clear because no Sprint is currently Active.
type ProjectAttention struct {
	ProjectID string                 `json:"project_id"`
	Items     []ProjectAttentionItem `json:"items"`
}

// ProjectAttentionItem describes durable, non-runnable or dormant Sprint work.
// It deliberately does not replace Pulse, which is reserved for over-budget
// Active Sprint execution.
type ProjectAttentionItem struct {
	Kind        string `json:"kind"`
	SprintID    string `json:"sprint_id"`
	ItemAddress string `json:"item_address"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	HoldReason  string `json:"hold_reason,omitempty"`
	Detail      string `json:"detail"`
}
