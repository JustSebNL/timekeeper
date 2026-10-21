package ntfy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// WakeResult describes the outcome of a guardian wake cycle.
type WakeResult struct {
	NudgeID   int64  `json:"nudge_id"`
	AgentID   string `json:"agent_id"`
	WorkDone  bool   `json:"work_done"`
	SprintID  string `json:"sprint_id,omitempty"`
	Timestamp string `json:"timestamp"`
}

// Wake reads pending pulse events, runs the work cycle, and acks the nudge.
func Wake(ctx context.Context, baseURL string, agentID string) (*WakeResult, error) {
	// Read pending events for this agent.
	eventsURL := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + agentID + "/nudges?status=pending"
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(eventsURL)
	if err != nil {
		return nil, fmt.Errorf("read pending nudges: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read nudges body: %w", err)
	}
	var nudgeList struct {
		Items []struct {
			NudgeID int64  `json:"nudge_id"`
			AgentID string `json:"agent_id"`
			Kind    string `json:"kind"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &nudgeList); err != nil {
		return nil, fmt.Errorf("parse nudges: %w", err)
	}

	result := &WakeResult{
		AgentID:   agentID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if len(nudgeList.Items) == 0 {
		return result, nil
	}

	// Process each pending nudge.
	for _, nudge := range nudgeList.Items {
		result.NudgeID = nudge.NudgeID

		// Claim and run work cycle.
		workDone, sprintID := runWorkCycle(ctx, baseURL, agentID)
		result.WorkDone = workDone
		result.SprintID = sprintID

		// Acknowledge the nudge.
		if err := ackNudge(ctx, baseURL, agentID, nudge.NudgeID); err != nil {
			return result, fmt.Errorf("ack nudge %d: %w", nudge.NudgeID, err)
		}
	}

	return result, nil
}

func runWorkCycle(ctx context.Context, baseURL, agentID string) (bool, string) {
	// 1. Check for overdue sprints.
	pulseURL := strings.TrimRight(baseURL, "/") + "/api/v1/pulse"
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(pulseURL)
	if err != nil {
		return false, ""
	}
	resp.Body.Close()

	// 2. Claim next sprint.
	// (In a full implementation, this would parse pulse results and claim the oldest runnable sprint.)
	// For now, attempt to claim via the sprint queue.
	claimURL := strings.TrimRight(baseURL, "/") + "/api/v1/sprints/claim"
	claimReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, claimURL, bytes.NewReader([]byte(`{}`)))
	claimReq.Header.Set("Content-Type", "application/json")
	claimResp, err := (&http.Client{Timeout: 10 * time.Second}).Do(claimReq)
	if err != nil {
		return false, ""
	}
	claimResp.Body.Close()

	// 3. Record progress.
	progressURL := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + agentID + "/progress"
	progressBody, _ := json.Marshal(map[string]any{
		"active_sprint_id": "",
	})
	progressReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, progressURL, bytes.NewReader(progressBody))
	progressReq.Header.Set("Content-Type", "application/json")
	progressResp, err := (&http.Client{Timeout: 10 * time.Second}).Do(progressReq)
	if err != nil {
		return false, ""
	}
	progressResp.Body.Close()

	return true, ""
}

func ackNudge(ctx context.Context, baseURL, agentID string, nudgeID int64) error {
	ackURL := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + agentID + "/nudges/" + strconv.FormatInt(nudgeID, 10) + "/ack"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ackURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ack nudge: %s", resp.Status)
	}
	return nil
}
