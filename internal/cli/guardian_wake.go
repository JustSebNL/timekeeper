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
	"time"

	"github.com/JustSebNL/timekeeper/internal/cache"
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
// agentContextCache is the shared bounded context cache for guardian wake cycles.
// It stores agent state between nudges so the agent has memory of previous work.
var agentContextCache *cache.Cache

// getAgentContextCache returns the shared agent context cache, initializing it on first use.
func getAgentContextCache() *cache.Cache {
	if agentContextCache == nil {
		cfg := cache.DefaultConfig()
		cfg.NumShards = 256
		cfg.MaxEntries = 1000
		cfg.MaxBytes = 8 * 1024 * 1024 // 8MB
		cfg.AdmissionEnabled = false    // always admit; evict on pressure
		c, err := cache.New(cfg)
		if err == nil {
			agentContextCache = c
		}
	}
	return agentContextCache
}

// AgentContext is the per-agent state stored in the bounded context cache.
type AgentContext struct {
	AgentID        string `json:"agent_id"`
	LastNudgeID    int64  `json:"last_nudge_id"`
	LastNudgeTime  string `json:"last_nudge_time"`
	WorkCycles     int    `json:"work_cycles"`
	LastSprintID   string `json:"last_sprint_id"`
	LastProjectID  string `json:"last_project_id"`
	Notes          string `json:"notes"`
}

// loadAgentContext retrieves the agent's context from the bounded cache.
func loadAgentContext(agentID string) *AgentContext {
	c := getAgentContextCache()
	if c == nil {
		return &AgentContext{AgentID: agentID}
	}
	key := []byte("agent:" + agentID)
	val, err := c.Get(key)
	if err != nil {
		return &AgentContext{AgentID: agentID}
	}
	var ctx AgentContext
	if err := json.Unmarshal(val, &ctx); err != nil {
		return &AgentContext{AgentID: agentID}
	}
	return &ctx
}

// saveAgentContext stores the agent's context in the bounded cache.
func saveAgentContext(ctx *AgentContext) {
	c := getAgentContextCache()
	if c == nil {
		return
	}
	key := []byte("agent:" + ctx.AgentID)
	val, err := json.Marshal(ctx)
	if err != nil {
		return
	}
	// Store with 24h TTL — agent context is ephemeral
	_, _ = c.Set(key, val, 24*time.Hour)
}

func guardianWakeCLI(args []string, out, errOut io.Writer, baseURL string) int {
	agentID := ""
	pollInterval := 30 * time.Second
	pollMode := false

	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--agent" {
			agentID = args[i+1]
		}
		if args[i] == "--poll" {
			pollMode = true
		}
		if args[i] == "--interval" && i+1 < len(args) {
			if d, err := time.ParseDuration(args[i+1]); err == nil {
				pollInterval = d
			}
		}
	}

	if agentID == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk guardian wake --agent <id> [--poll] [--interval <duration>]")
		return 1
	}

	tk, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "client: %v\n", err)
		return 1
	}

	// Load agent context from bounded cache.
	ctx := loadAgentContext(agentID)
	ctx.AgentID = agentID

	if pollMode {
		return runPollLoop(agentID, tk, ctx, pollInterval, out, errOut)
	}

	// Single-shot wake.
	return runWakeOnce(agentID, tk, ctx, out, errOut)
}

// runPollLoop continuously checks for pending nudges and processes them.
// This is the durable fallback when ntfy/HTTP are unavailable.
func runPollLoop(agentID string, tk *storeclient.Client, ctx *AgentContext, interval time.Duration, out, errOut io.Writer) int {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		pending, err := listPendingNudgesForAgent(context.Background(), tk, agentID)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "list pending nudges: %v\n", err)
			time.Sleep(interval)
			continue
		}
		if len(pending) == 0 {
			// No pending nudges — wait for next tick.
			<-ticker.C
			continue
		}

		nudgeID := pending[0]
		_, _ = fmt.Fprintf(out, "waking agent %s (nudge %d, cycle %d)\n", agentID, nudgeID, ctx.WorkCycles+1)

		// Record delivered event.
		recordPulseEvent(context.Background(), tk, agentID, nudgeID, "delivered")

		// Run the work cycle.
		if err := runGuardianWorkCycle(agentID, ctx); err != nil {
			_, _ = fmt.Fprintf(errOut, "work cycle failed: %v\n", err)
			time.Sleep(interval)
			continue
		}

		// Acknowledge the nudge.
		ackURL := strings.TrimRight(tk.BaseURL, "/") + "/api/v1/agents/" + agentID + "/nudges/" + fmt.Sprintf("%d", nudgeID) + "/ack"
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ackURL, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}

		// Record acknowledged event.
		recordPulseEvent(context.Background(), tk, agentID, nudgeID, "acknowledged")

		// Update and save agent context.
		ctx.LastNudgeID = nudgeID
		ctx.LastNudgeTime = time.Now().Format(time.RFC3339)
		ctx.WorkCycles++
		saveAgentContext(ctx)

		_, _ = fmt.Fprintf(out, "agent %s completed work cycle %d\n", agentID, ctx.WorkCycles)

		// Brief pause to avoid hammering.
		time.Sleep(1 * time.Second)
	}
}

// runWakeOnce processes a single nudge (one-shot mode).
func runWakeOnce(agentID string, tk *storeclient.Client, ctx *AgentContext, out, errOut io.Writer) int {
	// List pending nudges for this agent.
	pending, err := listPendingNudgesForAgent(context.Background(), tk, agentID)
	if err != nil || len(pending) == 0 {
		_, _ = fmt.Fprintf(out, "no pending nudges for agent %s\n", agentID)
		return 0
	}

	nudgeID := pending[0]
	_, _ = fmt.Fprintf(out, "waking agent %s (nudge %d, cycle %d)\n", agentID, nudgeID, ctx.WorkCycles+1)

	// Record delivered event.
	recordPulseEvent(context.Background(), tk, agentID, nudgeID, "delivered")

	// Run the work cycle.
	if err := runGuardianWorkCycle(agentID, ctx); err != nil {
		_, _ = fmt.Fprintf(errOut, "work cycle failed: %v\n", err)
		return 1
	}

	// Acknowledge the nudge.
	ackURL := strings.TrimRight(tk.BaseURL, "/") + "/api/v1/agents/" + agentID + "/nudges/" + fmt.Sprintf("%d", nudgeID) + "/ack"
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ackURL, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}

	// Record acknowledged event.
	recordPulseEvent(context.Background(), tk, agentID, nudgeID, "acknowledged")

	// Update and save agent context.
	ctx.LastNudgeID = nudgeID
	ctx.LastNudgeTime = time.Now().Format(time.RFC3339)
	ctx.WorkCycles++
	saveAgentContext(ctx)

	_, _ = fmt.Fprintf(out, "agent %s completed work cycle %d\n", agentID, ctx.WorkCycles)
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

func runGuardianWorkCycle(agentID string, ctx *AgentContext) error {
	// Run the work cycle script via cmd.exe (Windows).
	script := "scripts/guardian/self-nudge-work.sh"
	cmd := exec.CommandContext(context.Background(), "cmd.exe", "/c", script)
	env := os.Environ()
	env = append(env, "AGENT_ID="+agentID)
	if ctx != nil {
		env = append(env, "AGENT_WORK_CYCLES="+fmt.Sprintf("%d", ctx.WorkCycles))
		env = append(env, "AGENT_LAST_NUDGE_ID="+fmt.Sprintf("%d", ctx.LastNudgeID))
		env = append(env, "AGENT_LAST_SPRINT_ID="+ctx.LastSprintID)
		env = append(env, "AGENT_LAST_PROJECT_ID="+ctx.LastProjectID)
	}
	cmd.Env = env
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
