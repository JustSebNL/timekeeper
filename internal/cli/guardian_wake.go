package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/storeclient"
	"github.com/JustSebNL/timekeeper/internal/stuck"
)

// wakeCoalescedExitCode is transport-neutral: shell, PowerShell, batch, and
// native adapters can all report that an already-admitted worker owns this
// wake without making Guardian treat the duplicate as a failed work cycle.
const wakeCoalescedExitCode = 75

var errWakeCoalesced = errors.New("wake coalesced: another worker owns the nudge")

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
// AgentContext is the per-agent state stored in durable file storage.
type AgentContext struct {
	SessionID     string        `json:"session_id"`
	Runtime       string        `json:"runtime"`
	AgentID       string        `json:"agent_id"`
	LastNudgeID   int64         `json:"last_nudge_id"`
	LastNudgeTime string        `json:"last_nudge_time"`
	WorkCycles    int           `json:"work_cycles"`
	LastSprintID  string        `json:"last_sprint_id"`
	LastProjectID string        `json:"last_project_id"`
	Notes         string        `json:"notes"`
	BaseURL       string        `json:"-"`
	Adapter       string        `json:"-"`
	WorkTimeout   time.Duration `json:"-"`
}

type guardianWakeOptions struct {
	agentID      string
	pollInterval time.Duration
	pollMode     bool
	continuous   bool
	untilIdle    bool
	maxCycles    int
	projectID    string
	adapter      string
	workTimeout  time.Duration
}

func parseGuardianWakeOptions(args []string) (guardianWakeOptions, error) {
	opts := guardianWakeOptions{pollInterval: 30 * time.Second, maxCycles: 1, workTimeout: 30 * time.Minute}
	explicitCycles := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project", "--adapter", "--work-timeout":
			flag := args[i]
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return opts, fmt.Errorf("%s requires a value", flag)
			}
			i++
			switch flag {
			case "--project":
				opts.projectID = args[i]
			case "--adapter":
				opts.adapter = args[i]
			case "--work-timeout":
				d, err := time.ParseDuration(args[i])
				if err != nil || d <= 0 || d > 24*time.Hour {
					return opts, errors.New("--work-timeout must be positive and at most 24h")
				}
				opts.workTimeout = d
			}
		case "--agent":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return opts, errors.New("--agent requires an id")
			}
			opts.agentID = args[i+1]
			i++
		case "--poll":
			opts.pollMode = true
		case "--continuous":
			opts.continuous = true
			if !explicitCycles {
				opts.maxCycles = 4
			}
		case "--until-idle":
			opts.untilIdle = true
		case "--max-cycles":
			if i+1 >= len(args) {
				return opts, errors.New("--max-cycles requires a positive integer")
			}
			cycles, err := strconv.Atoi(args[i+1])
			if err != nil || cycles < 1 || cycles > 32 {
				return opts, errors.New("--max-cycles must be between 1 and 32")
			}
			opts.maxCycles = cycles
			explicitCycles = true
			i++
		case "--interval":
			if i+1 >= len(args) {
				return opts, errors.New("--interval requires a duration")
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil || d <= 0 {
				return opts, errors.New("--interval must be a positive duration")
			}
			opts.pollInterval = d
			i++
		default:
			return opts, fmt.Errorf("unknown wake option %q", args[i])
		}
	}
	if opts.agentID == "" {
		return opts, errors.New("--agent requires an id")
	}
	if !opts.continuous {
		opts.maxCycles = 1
	}
	if !validWakeAgentID(opts.agentID) {
		return opts, errors.New("agent id must contain only letters, digits, dot, underscore or hyphen (1-128 characters)")
	}
	if opts.untilIdle && !opts.continuous {
		return opts, errors.New("--until-idle requires --continuous")
	}
	return opts, nil
}

// agentContextPath resolves the durable file path for agent context.
func agentContextPath(agentID string) string {
	stateDir := os.Getenv("TIMEKEEPER_STATE_DIR")
	if stateDir == "" {
		stateDir = ".timekeeper/state"
	}
	return filepath.Join(stateDir, "agent-context-"+agentID+".json")
}

// loadAgentContext retrieves the agent's context from durable file storage.
func loadAgentContext(agentID string) *AgentContext {
	path := agentContextPath(agentID)
	data, err := os.ReadFile(path)
	if err != nil {
		return &AgentContext{AgentID: agentID}
	}
	var ctx AgentContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return &AgentContext{AgentID: agentID}
	}
	if ctx.AgentID == "" {
		ctx.AgentID = agentID
	}
	if ctx.AgentID != agentID {
		return &AgentContext{AgentID: agentID}
	}
	return &ctx
}

// saveAgentContext stores the agent's context in durable file storage.
func saveAgentContext(ctx *AgentContext) {
	path := agentContextPath(ctx.AgentID)
	data, err := json.Marshal(ctx)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, data, 0o600)
}

func guardianWakeCLI(args []string, out, errOut io.Writer, baseURL string) int {
	opts, err := parseGuardianWakeOptions(args)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "usage: tk guardian wake --agent <id> [--project <P-ID>] [--adapter <executable>] [--work-timeout <duration>] [--poll] [--continuous [--until-idle] --max-cycles <1-32>] [--interval <duration>] (%v)\n", err)
		return 1
	}

	tk, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "client: %v\n", err)
		return 1
	}

	// Load agent context from bounded cache.
	ctx := loadAgentContext(opts.agentID)
	ctx.AgentID = opts.agentID
	ctx.BaseURL, ctx.Adapter, ctx.WorkTimeout = baseURL, opts.adapter, opts.workTimeout
	if opts.projectID != "" {
		ctx.LastProjectID = opts.projectID
	}
	if ctx.LastProjectID == "" {
		ctx.LastProjectID = guardianConfiguredProject(opts.agentID)
	}

	if opts.pollMode {
		return runPollLoop(opts.agentID, tk, ctx, opts.pollInterval, opts.continuous, opts.untilIdle, opts.maxCycles, out, errOut)
	}

	// Single-shot wake.
	return runWakeOnce(opts.agentID, tk, ctx, opts.continuous, opts.maxCycles, out, errOut, opts.untilIdle)
}

// runPollLoop continuously checks for pending nudges and processes them.
// This is the durable fallback when ntfy/HTTP are unavailable.
func runPollLoop(agentID string, tk *storeclient.Client, ctx *AgentContext, interval time.Duration, continuous, untilIdle bool, maxCycles int, out, errOut io.Writer) int {
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

		nudgeID := pending[0].NudgeID
		if err := prepareGuardianWake(tk, ctx, pending[0]); err != nil {
			_, _ = fmt.Fprintf(errOut, "bind wake context: %v\n", err)
			time.Sleep(interval)
			continue
		}
		_, _ = fmt.Fprintf(out, "waking agent %s (nudge %d, cycle %d)\n", agentID, nudgeID, ctx.WorkCycles+1)

		// Record delivered event.
		recordPulseEvent(context.Background(), tk, agentID, nudgeID, "delivered")

		// Run the work cycle with stuck detection.
		detector := newStuckDetector()
		if err := runGuardianWorkCycle(agentID, ctx, detector, continuous, 1, maxCycles); err != nil {
			if errors.Is(err, errWakeCoalesced) {
				_, _ = fmt.Fprintf(errOut, "wake coalesced: another worker owns agent %s; leaving nudge pending\n", agentID)
				time.Sleep(interval)
				continue
			}
			_, _ = fmt.Fprintf(errOut, "work cycle failed: %v\n", err)
			time.Sleep(interval)
			continue
		}

		// Acknowledge the nudge.
		if err := acknowledgeGuardianWake(tk, agentID, nudgeID); err != nil {
			_, _ = fmt.Fprintf(errOut, "acknowledge nudge: %v\n", err)
			time.Sleep(interval)
			continue
		}

		// Record acknowledged event.
		recordPulseEvent(context.Background(), tk, agentID, nudgeID, "acknowledged")

		// Update and save agent context.
		ctx.LastNudgeID = nudgeID
		ctx.LastNudgeTime = time.Now().Format(time.RFC3339)
		ctx.WorkCycles++
		saveAgentContext(ctx)

		_, _ = fmt.Fprintf(out, "agent %s completed work cycle %d\n", agentID, ctx.WorkCycles)
		if continuous {
			if untilIdle {
				runWorkUntilIdle(agentID, tk, ctx, detector, maxCycles, out, errOut)
			} else {
				runAdditionalWorkCycles(agentID, ctx, detector, maxCycles, out, errOut)
			}
		}

		// Brief pause to avoid hammering.
		time.Sleep(1 * time.Second)
	}
}

// runWakeOnce processes a single nudge (one-shot mode).
func runWakeOnce(agentID string, tk *storeclient.Client, ctx *AgentContext, continuous bool, maxCycles int, out, errOut io.Writer, untilIdle ...bool) int {
	// List pending nudges for this agent.
	pending, err := listPendingNudgesForAgent(context.Background(), tk, agentID)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "list pending nudges: %v\n", err)
		return 1
	}
	if len(pending) == 0 {
		_, _ = fmt.Fprintf(out, "no pending nudges for agent %s\n", agentID)
		return 0
	}

	nudgeID := pending[0].NudgeID
	if err := prepareGuardianWake(tk, ctx, pending[0]); err != nil {
		_, _ = fmt.Fprintf(errOut, "bind wake context: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "waking agent %s (nudge %d, cycle %d)\n", agentID, nudgeID, ctx.WorkCycles+1)

	// Record delivered event.
	recordPulseEvent(context.Background(), tk, agentID, nudgeID, "delivered")

	// Run the work cycle with stuck detection.
	detector := newStuckDetector()
	if err := runGuardianWorkCycle(agentID, ctx, detector, continuous, 1, maxCycles); err != nil {
		if errors.Is(err, errWakeCoalesced) {
			_, _ = fmt.Fprintf(errOut, "wake coalesced: another worker owns agent %s; nudge remains pending\n", agentID)
			return 0
		}
		_, _ = fmt.Fprintf(errOut, "work cycle failed: %v\n", err)
		return 1
	}

	// Acknowledge the nudge.
	if err := acknowledgeGuardianWake(tk, agentID, nudgeID); err != nil {
		_, _ = fmt.Fprintf(errOut, "acknowledge nudge: %v\n", err)
		return 1
	}

	// Record acknowledged event.
	recordPulseEvent(context.Background(), tk, agentID, nudgeID, "acknowledged")

	// Update and save agent context.
	ctx.LastNudgeID = nudgeID
	ctx.LastNudgeTime = time.Now().Format(time.RFC3339)
	ctx.WorkCycles++
	saveAgentContext(ctx)

	_, _ = fmt.Fprintf(out, "agent %s completed work cycle %d\n", agentID, ctx.WorkCycles)
	if continuous {
		if len(untilIdle) > 0 && untilIdle[0] {
			runWorkUntilIdle(agentID, tk, ctx, detector, maxCycles, out, errOut)
		} else {
			runAdditionalWorkCycles(agentID, ctx, detector, maxCycles, out, errOut)
		}
	}
	return 0
}

// runAdditionalWorkCycles turns one Guardian wake into a bounded worker run.
// The first cycle owns and acknowledges the nudge; these follow-up cycles are
// continuation work owned by the same admitted worker and are independently
// persisted in the local agent context.
func runAdditionalWorkCycles(agentID string, ctx *AgentContext, detector *stuck.Detector, maxCycles int, out, errOut io.Writer) int {
	completed := 0
	for cycle := 2; cycle <= maxCycles; cycle++ {
		_, _ = fmt.Fprintf(out, "agent %s continuing work (%d/%d)\n", agentID, cycle, maxCycles)
		if err := runGuardianWorkCycle(agentID, ctx, detector, true, cycle, maxCycles); err != nil {
			if !errors.Is(err, errWakeCoalesced) {
				_, _ = fmt.Fprintf(errOut, "continuous work stopped after %d cycle(s): %v\n", cycle-1, err)
			}
			break
		}
		ctx.WorkCycles++
		saveAgentContext(ctx)
		completed++
		_, _ = fmt.Fprintf(out, "agent %s completed work cycle %d\n", agentID, ctx.WorkCycles)
	}
	return completed
}

type guardianOperationalSummary struct {
	OpenSprints   int64 `json:"open_sprints"`
	ActiveSprints int64 `json:"active_sprints"`
}

// runWorkUntilIdle re-arms bounded continuation batches without requiring a
// new chat message. The project queue is the stop signal: once it has no Open
// or Active Sprints, the worker returns to its poll loop. A hard batch cap is
// retained so a malformed project state cannot create an unbounded model loop.
func runWorkUntilIdle(agentID string, tk *storeclient.Client, ctx *AgentContext, detector *stuck.Detector, maxCycles int, out, errOut io.Writer) int {
	const maxBatches = 64
	if maxCycles < 2 {
		return 0
	}
	total := 0
	for batch := 1; batch <= maxBatches; batch++ {
		runnable, err := projectHasRunnableWork(context.Background(), tk, ctx.LastProjectID)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "continuous work stopped: cannot inspect project queue: %v\n", err)
			return total
		}
		if !runnable {
			_, _ = fmt.Fprintf(out, "agent %s reached durable project idle state\n", agentID)
			return total
		}

		for cycle := 2; cycle <= maxCycles; cycle++ {
			if cycle > 2 {
				runnable, err := projectHasRunnableWork(context.Background(), tk, ctx.LastProjectID)
				if err != nil {
					_, _ = fmt.Fprintf(errOut, "continuous work stopped: %v\n", err)
					return total
				}
				if !runnable {
					_, _ = fmt.Fprintf(out, "agent %s reached durable project idle state\n", agentID)
					return total
				}
			}
			_, _ = fmt.Fprintf(out, "agent %s continuing work (%d/%d, batch %d)\n", agentID, cycle, maxCycles, batch)
			if err := runGuardianWorkCycle(agentID, ctx, detector, true, cycle, maxCycles); err != nil {
				if !errors.Is(err, errWakeCoalesced) {
					_, _ = fmt.Fprintf(errOut, "continuous work stopped after %d cycle(s): %v\n", total+1, err)
				}
				return total
			}
			ctx.WorkCycles++
			saveAgentContext(ctx)
			total++
			_, _ = fmt.Fprintf(out, "agent %s completed work cycle %d\n", agentID, ctx.WorkCycles)
		}
	}
	_, _ = fmt.Fprintf(errOut, "continuous work stopped at safety cap: %d batches\n", maxBatches)
	return total
}

func projectHasRunnableWork(ctx context.Context, tk *storeclient.Client, projectID string) (bool, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return false, errors.New("agent context has no project id")
	}
	endpoint := strings.TrimRight(tk.BaseURL, "/") + "/api/v1/projects/" + projectID + "/operational-summary"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	response, err := tk.HTTPClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return false, fmt.Errorf("operational summary returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var summary guardianOperationalSummary
	if err := json.NewDecoder(response.Body).Decode(&summary); err != nil {
		return false, err
	}
	return summary.OpenSprints > 0 || summary.ActiveSprints > 0, nil
}

func listPendingNudgesForAgent(ctx context.Context, tk *storeclient.Client, agentID string) ([]model.PulseNudge, error) {
	endpoint := strings.TrimRight(tk.BaseURL, "/") + "/api/v1/agents/" + url.PathEscape(agentID) + "/nudges"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := tk.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pending nudges returned %s", resp.Status)
	}
	var result struct {
		Items []model.PulseNudge `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, err
	}

	for _, item := range result.Items {
		if item.NudgeID <= 0 || item.AgentID != agentID || item.Status != "Pending" {
			return nil, errors.New("invalid pending nudge identity or status")
		}
	}
	return result.Items, nil
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

func newStuckDetector() *stuck.Detector {
	cfg := stuck.DefaultConfig()
	cfg.OnEscalate = func(level int, reason string) {
		// Escalation is logged but does not block the work cycle.
		// The guardian will deliver the nudge again if the agent is truly stuck.
		if level >= 2 {
			fmt.Fprintf(os.Stderr, "STUCK DETECTOR: level %d — %s\n", level, reason)
		}
	}
	return stuck.NewDetector(cfg)
}

func guardianWorkCommand(goos, script string) (string, []string) {
	ext := strings.ToLower(filepath.Ext(script))
	if ext == ".ps1" {
		shell := "pwsh"
		if goos == "windows" {
			shell = "powershell.exe"
		}
		return shell, []string{"-NoProfile", "-NonInteractive", "-File", script}
	}
	if goos == "windows" {
		// The installed CLI is a Windows binary, but the canonical work
		// hook is a repository .sh script. Running it through cmd.exe makes
		// cmd interpret `scripts` as a command. Route shell hooks through
		// WSL with an absolute /mnt path instead.
		if strings.HasSuffix(strings.ToLower(script), ".sh") {
			absolute, err := filepath.Abs(script)
			if err == nil {
				return "wsl.exe", []string{"--", "bash", windowsPathToWSL(absolute)}
			}
			return "wsl.exe", []string{"--", "bash", script}
		}
		if ext == ".bat" || ext == ".cmd" {
			return "cmd.exe", []string{"/c", script}
		}
	}
	if ext == ".sh" {
		return "bash", []string{script}
	}
	return script, nil
}

func windowsPathToWSL(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = filepath.ToSlash(path)
	if len(path) >= 2 && path[1] == ':' {
		return "/mnt/" + strings.ToLower(path[:1]) + path[2:]
	}
	return path
}

func runGuardianWorkCycle(agentID string, ctx *AgentContext, detector *stuck.Detector, continuous bool, cycle, budget int) error {
	// Use the native shell for the host running the wake worker. The previous
	// implementation always invoked cmd.exe, so a WSL/Linux poll worker could
	// receive a nudge but never start its work hook.
	script := os.Getenv("TK_GUARDIAN_SELF_WORK")
	adapter := os.Getenv("AGENT_WAKE_COMMAND")
	if ctx != nil && ctx.Adapter != "" {
		adapter = ctx.Adapter
	}
	if adapter != "" {
		script = adapter
	}
	if script == "" {
		return errors.New("configure --adapter, AGENT_WAKE_COMMAND, or TK_GUARDIAN_SELF_WORK; no agent runtime is selected implicitly")
	}
	shell, shellArgs := guardianWorkCommand(runtime.GOOS, script)
	if adapter != "" {
		shellArgs = append(shellArgs, "Continue the already-authorized TimeKeeper work.")
	}
	timeout := 30 * time.Minute
	if ctx != nil && ctx.WorkTimeout > 0 {
		timeout = ctx.WorkTimeout
	}
	workCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(workCtx, shell, shellArgs...)
	cmd.WaitDelay = 5 * time.Second
	env := os.Environ()
	env = append(env, "AGENT_ID="+agentID)
	env = append(env, "TIMEKEEPER_AGENT_ID="+agentID)
	// Capture the original worker identity before clearing; restore it to the
	// adapter so the resumed session preserves the same FASE identity.
	originalSession := os.Getenv("TIMEKEEPER_SESSION_ID")
	originalRuntime := os.Getenv("TIMEKEEPER_AGENT_RUNTIME")
	originalFingerprint := os.Getenv("TIMEKEEPER_FASE_FINGERPRINT")
	originalScope := os.Getenv("TIMEKEEPER_FASE_SCOPE")
	// Never let a poller's own runtime session impersonate the watched worker.
	env = append(env, "TIMEKEEPER_SESSION_ID=", "TIMEKEEPER_AGENT_RUNTIME=", "CODEX_THREAD_ID=", "CODEX_SESSION_ID=", "HERMES_SESSION_ID=", "HERMES_SESSION_THREAD_ID=", "HERMES_CRON_SESSION=")
	mode := "single"
	if continuous {
		mode = "continuous"
	}
	env = append(env, "GUARDIAN_EVENT=guardian.continuation")
	env = append(env, "GUARDIAN_PRIORITY=actionable")
	env = append(env, "GUARDIAN_OBJECTIVE=continue already-authorized TimeKeeper work")
	env = append(env, "GUARDIAN_SCOPE=bound project/task/sprint from durable wake context")
	env = append(env, "GUARDIAN_CONTINUATION_MODE="+mode)
	env = append(env, "GUARDIAN_CONTINUATION_CYCLE="+strconv.Itoa(cycle))
	env = append(env, "GUARDIAN_CONTINUATION_BUDGET="+strconv.Itoa(budget))
	if ctx != nil {
		if ctx.SessionID != "" {
			env = append(env, "TIMEKEEPER_SESSION_ID="+ctx.SessionID, "TIMEKEEPER_AGENT_RUNTIME="+ctx.Runtime)
		}
		if ctx.BaseURL != "" {
			env = append(env, "TK_BASE_URL="+ctx.BaseURL)
		}
		env = append(env, "AGENT_WORK_CYCLES="+fmt.Sprintf("%d", ctx.WorkCycles))
		env = append(env, "AGENT_LAST_NUDGE_ID="+fmt.Sprintf("%d", ctx.LastNudgeID))
		env = append(env, "AGENT_LAST_SPRINT_ID="+ctx.LastSprintID)
		env = append(env, "AGENT_LAST_PROJECT_ID="+ctx.LastProjectID)
		if ctx.LastProjectID != "" {
			env = append(env, "PROJECT_ID="+ctx.LastProjectID)
		}
	}
	// Restore the original worker identity so the adapter resumes with the same
	// FASE session, rather than creating a new process-scoped fingerprint.
	if ctx != nil && ctx.SessionID != "" {
		env = append(env, "TIMEKEEPER_SESSION_ID="+ctx.SessionID)
	} else if originalSession != "" {
		env = append(env, "TIMEKEEPER_SESSION_ID="+originalSession)
	}
	if ctx != nil && ctx.Runtime != "" {
		env = append(env, "TIMEKEEPER_AGENT_RUNTIME="+ctx.Runtime)
	} else if originalRuntime != "" {
		env = append(env, "TIMEKEEPER_AGENT_RUNTIME="+originalRuntime)
	}
	if originalFingerprint != "" {
		env = append(env, "TIMEKEEPER_FASE_FINGERPRINT="+originalFingerprint)
	}
	if originalScope != "" {
		env = append(env, "TIMEKEEPER_FASE_SCOPE="+originalScope)
	}
	// Environment keys are unique: duplicate entries let the poller's old
	// session win on Windows and split the resumed FASE identity.
	cmd.Env = uniqueEnv(env)
	if runtime.GOOS == "windows" && shell == "wsl.exe" {
		// WSL does not inherit arbitrary Windows environment variables. Forward
		// only this adapter contract; translate filesystem paths explicitly.
		forward := "TIMEKEEPER_AGENT_ID:TIMEKEEPER_SESSION_ID:TIMEKEEPER_AGENT_RUNTIME:TIMEKEEPER_FASE_FINGERPRINT:TIMEKEEPER_FASE_SCOPE:AGENT_ID:AGENT_LAST_NUDGE_ID:AGENT_LAST_PROJECT_ID:AGENT_LAST_SPRINT_ID:AGENT_WORK_CYCLES:PROJECT_ID:TK_BASE_URL:GUARDIAN_EVENT:GUARDIAN_PRIORITY:GUARDIAN_OBJECTIVE:GUARDIAN_SCOPE:GUARDIAN_CONTINUATION_MODE:GUARDIAN_CONTINUATION_CYCLE:GUARDIAN_CONTINUATION_BUDGET:TIMEKEEPER_STATE_DIR/p:AGENT_WAKE_COMMAND/p"
		if existing := os.Getenv("WSLENV"); existing != "" {
			forward = existing + ":" + forward
		}
		cmd.Env = append(cmd.Env, "WSLENV="+forward)
	}

	// Wire stuck detector to capture signals from work cycle output.
	if detector != nil {
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("create stdout pipe: %w", err)
		}
		cmd.Stderr = cmd.Stdout // merge stderr into stdout

		// Start the command
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start work cycle: %w", err)
		}

		// Scan output for signals.
		go func() {
			scanner := bufio.NewScanner(stdoutPipe)
			for scanner.Scan() {
				line := scanner.Text()
				// Record signals based on output patterns.
				if strings.Contains(line, "Build OK") || strings.Contains(line, "build pass") {
					detector.RecordNow(stuck.SignalBuildPass, nil)
				} else if strings.Contains(line, "Tests passed") || strings.Contains(line, "test pass") {
					detector.RecordNow(stuck.SignalTestPass, nil)
				} else if strings.Contains(line, "Sprint") && strings.Contains(line, "completed") {
					detector.RecordNow(stuck.SignalWorkCycle, nil)
				} else if strings.Contains(line, "failed") || strings.Contains(line, "error") {
					detector.RecordNow(stuck.SignalError, map[string]string{"reason": line})
				}
				// Echo the line to stdout for visibility.
				fmt.Println(line)
			}
		}()

		if err := cmd.Wait(); err != nil {
			if isWakeCoalesced(err) {
				return errWakeCoalesced
			}
			return fmt.Errorf("work cycle: %w", err)
		}
		return nil
	}

	// No detector — run without signal capture.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start work cycle: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		if isWakeCoalesced(err) {
			return errWakeCoalesced
		}
		return fmt.Errorf("work cycle: %w", err)
	}
	return nil
}

func uniqueEnv(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for i := len(values) - 1; i >= 0; i-- {
		key := values[i]
		if n := strings.IndexByte(key, '='); n >= 0 {
			key = key[:n]
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, values[i])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func isWakeCoalesced(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == wakeCoalescedExitCode
}
