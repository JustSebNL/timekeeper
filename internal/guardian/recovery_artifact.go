// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package guardian

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// WriteRecoveryArtifactForTest exposes the artifact writer for package tests
// without widening the production surface.
func WriteRecoveryArtifactForTest(stateDir string, nudge model.PulseNudge) error {
	return writeRecoveryArtifact(stateDir, nudge)
}

// writeRecoveryArtifact records durable local evidence that an agent's material
// progress lease expired and a recovery signal was delivered. It deliberately
// performs no process control: it does not kill, restart, or spawn the watched
// agent. The artifact is the auditable "what was in flight and when" record.
//
// path layout:
//
//	<stateDir>/recovery/<agentID>/<nudgeID>.json
//
// StateDir must already exist and be private. The receiver never creates parent
// directories; if the agentID subpath is missing the call fails closed rather
// than silently widening the writable surface.
func writeRecoveryArtifact(stateDir string, nudge model.PulseNudge) error {
	agentID := strings.TrimSpace(nudge.AgentID)
	if agentID == "" || strings.ContainsAny(agentID, "/\\") || len(agentID) > 256 {
		return fmt.Errorf("refusing recovery artifact for unsafe agent_id %q", agentID)
	}
	if nudge.NudgeID < 1 {
		return fmt.Errorf("refusing recovery artifact for invalid nudge_id %d", nudge.NudgeID)
	}
	agentDir := filepath.Join(stateDir, "recovery", agentID)
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return fmt.Errorf("prepare recovery directory: %w", err)
	}
	artifact := map[string]any{
		"format":                 "timekeeper-guardian-recovery/v1",
		"recorded_at":            time.Now().UTC().Round(0).Format(time.RFC3339Nano),
		"policy":                 "local-artifact",
		"agent_id":               agentID,
		"nudge_id":               nudge.NudgeID,
		"kind":                   nudge.Kind,
		"active_sprint_id":       nudge.ActiveSprintID,
		"detected_after_seconds": nudge.DetectedAfterSeconds,
		"nudge_created_at":       nudge.CreatedAt.UTC().Round(0).Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode recovery artifact: %w", err)
	}
	target := filepath.Join(agentDir, fmt.Sprintf("%d.json", nudge.NudgeID))
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("write recovery artifact: %w", err)
	}
	return nil
}
