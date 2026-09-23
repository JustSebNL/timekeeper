package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/storeclient"
)

// guardianWakeCLI implements `tk guardian wake --agent <id>` — the agent-side
// entry point invoked by the ntfy subscriber (or the HTTP receiver) when the
// guardian delivers a wake-up nudge.
//
// The command:
// 1. Reads the pending nudge from the TimeKeeper server
// 2. Records a PulseEventDelivered
// 3. Runs the work cycle (same as self-nudge-work.sh)
// 4. Acknowledges the nudge via AcknowledgePulseNudge
// 5. Records a PulseEventAcknowledged
func guardianWakeCLI(args []string, out, errOut io.Writer, baseURL string) int {
	agentID := ""
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--agent" {
			agentID = args[i+1]
		}
	}
	if agentID == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk guardian wake --agent <id>")
		return 1
	}

	tk, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "client: %v\n", err)
		return 1
	}

	// List pending nudges for this agent.
	pending, err := listPendingNudgesForAgent(context.Background(), tk, agentID)
	if err != nil || len(pending) == 0 {
		_, _ = fmt.Fprintf(out, "no pending nudges for agent %s\n", agentID)
		return 0
	}

	nudgeID := pending[0]
	_, _ = fmt.Fprintf(out, "waking agent %s (nudge %d)\n", agentID, nudgeID)

	// Record delivered event.
	recordPulseEvent(context.Background(), tk, agentID, nudgeID, "delivered")

	// Run the work cycle.
	if err := runGuardianWorkCycle(agentID); err != nil {
		_, _ = fmt.Fprintf(errOut, "work cycle failed: %v\n", err)
		return 1
	}

	// Acknowledge the nudge.
	ackURL := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + agentID + "/nudges/" + fmt.Sprintf("%d", nudgeID) + "/ack"
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ackURL, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}

	// Record acknowledged event.
	recordPulseEvent(context.Background(), tk, agentID, nudgeID, "acknowledged")

	_, _ = fmt.Fprintf(out, "agent %s completed work cycle\n", agentID)
	return 0
}

func listPendingNudgesForAgent(ctx context.Context, tk *storeclient.Client, agentID string) ([]int64, error) {
	url := strings.TrimRight(tk.BaseURL, "/") + "/api/v1/agents/" + agentID + "/nudges"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := tk.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Items []struct {
			NudgeID int64 `json:"nudge_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(result.Items))
	for _, item := range result.Items {
		ids = append(ids, item.NudgeID)
	}
	return ids, nil
}

func recordPulseEvent(ctx context.Context, tk *storeclient.Client, agentID string, nudgeID int64, eventType string) {
	// Pulse event recording is best-effort; the wake flow continues regardless.
	url := strings.TrimRight(tk.BaseURL, "/") + "/api/v1/agents/" + agentID + "/nudges/" + fmt.Sprintf("%d", nudgeID) + "/events"
	payload := map[string]string{
		"event_type": eventType,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := tk.HTTPClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func runGuardianWorkCycle(agentID string) error {
	// Run the work cycle script via cmd.exe (Windows).
	script := "scripts/guardian/self-nudge-work.sh"
	cmd := exec.CommandContext(context.Background(), "cmd.exe", "/c", script)
	cmd.Env = append(os.Environ(), "AGENT_ID="+agentID)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start work cycle: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("work cycle: %w", err)
	}
	return nil
}
