// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/logging"
	"github.com/JustSebNL/timekeeper/internal/model"
)

const defaultBaseURL = "http://127.0.0.1:1618"

// extractVerboseFlag pulls -v/--verbose/--debug from the front of args.
// Returned args are the remaining positional args. Returned mode is the
// requested log mode (ModeNormal by default).
func extractVerboseFlag(args []string) ([]string, logging.Mode) {
	mode := logging.ModeNormal
	out := args[:0:0]
	for _, a := range args {
		switch a {
		case "-v", "--verbose", "--debug":
			mode = logging.ModeDebug
			continue
		}
		out = append(out, a)
	}
	return out, mode
}

// ensureCLILogger initialises a logger for CLI invocations. The CLI doesn't
// normally write to a file (output is the user-visible stdout), but in debug
// mode we capture every step + every HTTP exchange to a file too so a CLI
// crash can be bugfixed after the fact.
func ensureCLILogger(mode logging.Mode) {
	if logging.CurrentMode() != logging.ModeNormal || mode == logging.ModeNormal {
		// Either already set, or normal mode. Default: log to stderr only.
		// If a logger hasn't been init'd yet and we're normal, just init it
		// with no file path.
		logging.Init(logging.Config{Path: "", Mode: mode})
		return
	}
	logPath := os.Getenv("TIMEKEEPER_CLI_LOG")
	if logPath == "" {
		logPath = filepath.Join(".timekeeper", "log", "cli.log")
	}
	logging.Init(logging.Config{Path: logPath, Mode: mode, MaxSizeMiB: 5, MaxBackups: 3})
}

// runCommand wraps a CLI subcommand so its start/duration/result are logged
// at the appropriate level for the current mode. Non-zero exit codes are
// reported as errors in both modes; full stack only in debug.
func runCommand(name string, fn func() int) int {
	start := time.Now()
	logger := logging.L()
	if logging.IsDebug() {
		logger.Debug("cli command start", slog.String("cmd", name))
	} else {
		logger.Info("cli", slog.String("cmd", name))
	}
	code := fn()
	dur := time.Since(start)
	attrs := []any{slog.String("cmd", name), slog.Int("exit", code), slog.Duration("duration", dur)}
	if code != 0 {
		logger.Error("cli command failed", append(attrs, slog.String("trace", string(debug.Stack())))...)
		return code
	}
	if logging.IsDebug() {
		logger.Debug("cli command ok", attrs...)
	} else {
		logger.Info("cli ok", attrs...)
	}
	return code
}

func init() {
	// Default: no file path; debug mode set per-invocation via -v.
	_ = logging.Init(logging.Config{Path: "", Mode: logging.ModeNormal})
}

// ResolveBaseURL extracts an optional explicit endpoint before dispatching a CLI command.
func ResolveBaseURL(args []string, environmentURL string) (string, []string, error) {
	baseURL := strings.TrimSpace(environmentURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if len(args) == 0 {
		return baseURL, args, nil
	}
	if args[0] == "--url" {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return "", nil, fmt.Errorf("--url requires an API base URL")
		}
		return strings.TrimSpace(args[1]), args[2:], nil
	}
	if value, ok := strings.CutPrefix(args[0], "--url="); ok {
		if strings.TrimSpace(value) == "" {
			return "", nil, fmt.Errorf("--url requires an API base URL")
		}
		return strings.TrimSpace(value), args[1:], nil
	}
	return baseURL, args, nil
}

// Run executes a framework-neutral Time Keeper CLI command.
func Run(args []string, out, errOut io.Writer, baseURL string) int {
	// Pull -v/--verbose/--debug out of the front of args to flip log mode.
	args, mode := extractVerboseFlag(args)
	ensureCLILogger(mode)

	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, _ = fmt.Fprint(out, "Usage: tk [--url <api-base-url>] <command>\n\nCommands:\n  list                                        List projects\n  tree <project-id>                           Show an executable hierarchy\n  export <project-id>                         Print a portable Project snapshot as JSON\n  summary <project-id>                        Show a durable Sprint operational snapshot\n  usage <project-id>                          Show recorded agent token usage\n  usage record <project-id> <session-id> <agent-id> <model> <turn> <input> <output> [cache-write] [cache-read] [messages] [sprint-id]\n  pulse                                       Show local Sprint attention needing follow-up\n  agent progress <id> <lease> [sprint-id] [guardian-url]\n                                              Renew an agent material-progress lease and optionally register a numeric-loopback Guardian\n  agent nudges <id>                           List durable unacknowledged Guardian nudges\n  agent history <id>                          List durable Guardian delivery/recovery history\n  agent ack <id> <nudge-id>                   Acknowledge a Guardian nudge and renew the lease\n  events <project-id>                         List immutable Project activity\n  note <project-id> <content>                 Record a Project note\n  notes <project-id>                          List Project notes\n  p new <name>                                Create a Project\n  p edit <project-id> <goal> <description>    Update Project context\n  p status <project-id> <status>              Set Project status\n  p alias <project-id> <alias>                Set Project alias\n  p unalias <project-id>                      Clear Project alias\n  aliases                                     List project aliases\n  c new <project-id> <name> [parent-category-id] Create a Category\n  c edit <category-id> <goal> <description>    Update Category context\n  c status <category-id> <status>              Set Category status\n  t edit <task-id> <goal> <description>        Update Task context\n  t new <project-id> <category-id> <name> <estimate>\n                                              Create a Task\n  t status <task-id> <status>                 Set Task status\n  st new <task-id> <name> <estimate>          Create a Subtask\n  st status <subtask-id> <status>              Set Subtask status\n  sp new <task|subtask> <owner-id> <name> <estimate> [buffer-percent]\n                                              Create a Sprint\n  sp <start|hold|resume|complete|cancel> <sprint-id> [reason] Transition a Sprint; hold/cancel require a reason\n  sp reason <sprint-id> <reason>                Update why an already-held Sprint is blocked\n  sp next <project-id>                         Atomically claim the oldest runnable Sprint\n  sp attempts <sprint-id>                      List immutable retrieval-attempt evidence\n  sp attempt <sprint-id> <reason>              Record a failed retrieval attempt (fourth makes TimedOut)\n  sp extend <sprint-id> <duration> <reason>   Record justified additional planned time\n  sp extensions <sprint-id>                   List immutable extension history\n  sp entries <sprint-id>                      List recorded work/hold intervals\n  llm new <name> <provider> <base-url> <model> [system-prompt]\n                                              Register a loopback LLM pipeline\n  plan <generate|apply> <project-id> <pipeline-id|draft-id>\n                                              Generate or apply a reviewed planning draft\n  plan list <project-id>                       List planning drafts\n  doctor                                      Check whether Time Keeper is reachable\n  api-help                                    List all available API routes\n  service                                     Manage TimeKeeper OS service (install/uninstall/start/stop/status/logs)\n")
		return 0
	}
	switch args[0] {
	case "plan":
		return runCommand("plan", func() int { return plan(args[1:], out, errOut, baseURL) })
	case "llm":
		return runCommand("llm", func() int { return llmPipeline(args[1:], out, errOut, baseURL) })
	case "p":
		return runCommand("project", func() int { return project(args[1:], out, errOut, baseURL) })
	case "c":
		return runCommand("category", func() int { return category(args[1:], out, errOut, baseURL) })
	case "t":
		return task(args[1:], out, errOut, baseURL)
	case "st":
		return subtask(args[1:], out, errOut, baseURL)
	case "sp":
		return sprint(args[1:], out, errOut, baseURL)
	case "note":
		return note(args[1:], out, errOut, baseURL)
	case "export":
		return exportProject(args[1:], out, errOut, baseURL)
	case "summary":
		return summary(args[1:], out, errOut, baseURL)
	case "usage":
		return usage(args[1:], out, errOut, baseURL)
	case "pulse":
		return pulse(out, errOut, baseURL)
	case "agent":
		return agentPulse(args[1:], out, errOut, baseURL)
	case "events":
		return events(args[1:], out, errOut, baseURL)
	case "notes":
		return notes(args[1:], out, errOut, baseURL)
	case "msg", "messages":
		return messages(args[1:], out, errOut, baseURL)
	case "list":
		return list(out, errOut, baseURL)
	case "tree":
		return tree(args[1:], out, errOut, baseURL)
	case "doctor":
		return doctor(out, errOut, baseURL)
	case "open":
		return openBrowser(args[1:], out, errOut)
	case "uninstall":
		return uninstallTimeKeeper(out, errOut, baseURL)
	case "hosts":
		return hostsSubcommand(args[1:], out, errOut)
	case "aliases":
		return listAliases(out, errOut, baseURL)
	case "api-help":
		return apiHelp(out, errOut, baseURL)
	case "service":
		return serviceManager(args[1:], out, errOut, baseURL)
	default:
		_, _ = fmt.Fprintf(errOut, "unknown command %q\nRun tk help for usage.\n", args[0])
		return 2
	}
}

func plan(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 2 && args[0] == "list" {
		return listPlanningDrafts(args[1], out, errOut, baseURL)
	}
	if len(args) != 3 || (args[0] != "generate" && args[0] != "apply") {
		_, _ = fmt.Fprintln(errOut, "usage: tk plan <generate|apply> <project-id> <pipeline-id|draft-id>")
		return 2
	}
	id, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil || id < 1 {
		return 2
	}
	base := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[1] + "/planning-drafts/"
	var endpoint string
	var body []byte
	if args[0] == "generate" {
		endpoint = base + "generate"
		body, _ = json.Marshal(map[string]int64{"pipeline_id": id})
	} else {
		endpoint = base + strconv.FormatInt(id, 10) + "/apply"
		body = []byte(`{}`)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	if args[0] == "apply" {
		_, _ = fmt.Fprintln(out, "applied")
		return 0
	}
	var draft struct {
		DraftID int64  `json:"draft_id"`
		Status  string `json:"status"`
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(response.Body).Decode(&draft); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%d\t%s\t%s\n", draft.DraftID, draft.Status, draft.Summary)
	return 0
}

func listPlanningDrafts(projectID string, out, errOut io.Writer, baseURL string) int {
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/planning-drafts")
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var value struct {
		Items []struct {
			DraftID int64  `json:"draft_id"`
			Status  string `json:"status"`
			Summary string `json:"summary"`
			RawJSON string `json:"raw_json"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return 1
	}
	for _, draft := range value.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\n%s\n", draft.DraftID, draft.Status, draft.Summary, draft.RawJSON)
	}
	return 0
}

func llmPipeline(args []string, out, errOut io.Writer, baseURL string) int {
	if (len(args) != 5 && len(args) != 6) || args[0] != "new" {
		_, _ = fmt.Fprintln(errOut, "usage: tk llm new <name> <ollama|openai-compatible> <base-url> <model> [system-prompt]")
		return 2
	}
	systemPrompt := ""
	if len(args) == 6 {
		systemPrompt = args[5]
	}
	body, err := json.Marshal(map[string]string{"name": args[1], "provider": args[2], "base_url": args[3], "model": args[4], "system_prompt": systemPrompt})
	if err != nil {
		return 1
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/llm-pipelines", bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var pipeline struct {
		PipelineID int64  `json:"pipeline_id"`
		Name       string `json:"name"`
		Provider   string `json:"provider"`
		Model      string `json:"model"`
	}
	if err := json.NewDecoder(response.Body).Decode(&pipeline); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%d\t%s\t%s\t%s\n", pipeline.PipelineID, pipeline.Name, pipeline.Provider, pipeline.Model)
	return 0
}

func exportProject(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk export <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/export"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var value any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "format Project export: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(out, string(encoded))
	return 0
}

func summary(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk summary <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/operational-summary"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var value struct {
		ProjectID                string `json:"project_id"`
		TotalSprints             int64  `json:"total_sprints"`
		ActiveSprints            int64  `json:"active_sprints"`
		HeldSprints              int64  `json:"held_sprints"`
		TimedOutSprints          int64  `json:"timed_out_sprints"`
		CancelledSprints         int64  `json:"cancelled_sprints"`
		EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
		BufferDurationSeconds    int64  `json:"buffer_duration_seconds"`
		ExtensionDurationSeconds int64  `json:"extension_duration_seconds"`
		PlannedDurationSeconds   int64  `json:"planned_duration_seconds"`
		RecordedWorkSeconds      int64  `json:"recorded_work_seconds"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s	sprints=%d	active=%d	held=%d	timed-out=%d	cancelled=%d	estimated=%ds	buffer=%ds	extensions=%ds	planned=%ds	recorded-work=%ds\n", value.ProjectID, value.TotalSprints, value.ActiveSprints, value.HeldSprints, value.TimedOutSprints, value.CancelledSprints, value.EstimatedDurationSeconds, value.BufferDurationSeconds, value.ExtensionDurationSeconds, value.PlannedDurationSeconds, value.RecordedWorkSeconds)
	return 0
}

func pulse(out, errOut io.Writer, baseURL string) int {
	url := strings.TrimRight(baseURL, "/") + "/api/v1/pulse"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var value struct {
		Format                      string `json:"format"`
		RecommendedNextPulseSeconds int64  `json:"recommended_next_pulse_seconds"`
		Attention                   []struct {
			Kind                   string `json:"kind"`
			ProjectID              string `json:"project_id"`
			SprintID               string `json:"sprint_id"`
			Name                   string `json:"name"`
			PlannedDurationSeconds int64  `json:"planned_duration_seconds"`
			ActiveDurationSeconds  int64  `json:"active_duration_seconds"`
			OverdueDurationSeconds int64  `json:"overdue_duration_seconds"`
		} `json:"attention"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	if value.Format != "timekeeper-pulse/v1" {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned unsupported Pulse format %q\n", value.Format)
		return 1
	}
	if len(value.Attention) == 0 {
		_, _ = fmt.Fprintf(out, "clear\tnext=%ds\n", value.RecommendedNextPulseSeconds)
		return 0
	}
	for _, item := range value.Attention {
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\tactive=%ds\tplanned=%ds\toverdue=%ds\t%s\n", item.Kind, item.ProjectID, item.SprintID, item.ActiveDurationSeconds, item.PlannedDurationSeconds, item.OverdueDurationSeconds, item.Name)
	}
	return 0
}

func agentPulse(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(errOut, "usage: tk agent <progress|nudges|ack> ...")
		return 2
	}
	switch args[0] {
	case "progress":
		if len(args) < 3 || len(args) > 5 || strings.TrimSpace(args[1]) == "" {
			_, _ = fmt.Fprintln(errOut, "usage: tk agent progress <agent-id> <lease> [sprint-id] [guardian-url]")
			return 2
		}
		lease, err := time.ParseDuration(args[2])
		if err != nil || lease < time.Second || lease != time.Duration(int64(lease.Seconds()))*time.Second {
			_, _ = fmt.Fprintln(errOut, "lease must be a whole positive duration such as 20s or 5m")
			return 2
		}
		input := map[string]any{"lease_duration_seconds": int64(lease.Seconds())}
		if len(args) >= 4 {
			input["active_sprint_id"] = args[3]
		}
		if len(args) == 5 {
			input["guardian_url"] = args[4]
		}
		body, err := json.Marshal(input)
		if err != nil {
			return 1
		}
		endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + url.PathEscape(args[1]) + "/progress"
		request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return 1
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
			return 1
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusCreated {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
			return 1
		}
		var progress model.AgentPulseProgress
		if err := json.NewDecoder(response.Body).Decode(&progress); err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "%s\tlease=%ds\tsprint=%s\tguardian=%s\n", progress.AgentID, progress.LeaseDurationSeconds, progress.ActiveSprintID, progress.GuardianURL)
		return 0
	case "nudges", "history":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			_, _ = fmt.Fprintf(errOut, "usage: tk agent %s <agent-id>\n", args[0])
			return 2
		}
		path := "/nudges"
		if args[0] == "history" {
			path += "/history"
		}
		endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + url.PathEscape(args[1]) + path
		response, err := (&http.Client{Timeout: 10 * time.Second}).Get(endpoint)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
			return 1
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
			return 1
		}
		var payload struct {
			Items []model.PulseNudge `json:"items"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
			return 1
		}
		for _, nudge := range payload.Items {
			_, _ = fmt.Fprintf(out, "%d\t%s\t%s\t%s\tdetected=%ds\tdelivery-attempts=%d\n", nudge.NudgeID, nudge.Status, nudge.Kind, nudge.ActiveSprintID, nudge.DetectedAfterSeconds, nudge.DeliveryAttempts)
		}
		return 0
	case "ack":
		if len(args) != 3 || strings.TrimSpace(args[1]) == "" {
			_, _ = fmt.Fprintln(errOut, "usage: tk agent ack <agent-id> <nudge-id>")
			return 2
		}
		nudgeID, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil || nudgeID < 1 {
			_, _ = fmt.Fprintln(errOut, "nudge ID must be a positive integer")
			return 2
		}
		endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + url.PathEscape(args[1]) + "/nudges/" + strconv.FormatInt(nudgeID, 10) + "/ack"
		request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader([]byte(`{}`)))
		if err != nil {
			return 1
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
			return 1
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
			return 1
		}
		var nudge model.PulseNudge
		if err := json.NewDecoder(response.Body).Decode(&nudge); err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\n", nudge.NudgeID, nudge.Status, nudge.AgentID)
		return 0
	default:
		_, _ = fmt.Fprintf(errOut, "unknown agent command %q\n", args[0])
		return 2
	}
}

func events(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk events <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/events"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Items []struct {
			EventID   int64  `json:"event_id"`
			EventType string `json:"event_type"`
			EntityID  string `json:"entity_id"`
			Message   string `json:"message"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	for _, event := range payload.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\t%s\n", event.EventID, event.EventType, event.EntityID, event.Message)
	}
	return 0
}

func notes(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk notes <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/notes"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Items []struct {
			NoteID    int64  `json:"note_id"`
			ProjectID string `json:"project_id"`
			Content   string `json:"content"`
			ActorID   string `json:"actor_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	for _, note := range payload.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\t%s\n", note.NoteID, note.ProjectID, note.ActorID, note.Content)
	}
	return 0
}

func note(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk note <project-id> <content>")
		return 2
	}
	body, err := json.Marshal(map[string]string{"content": args[1]})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode project note: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/notes"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create project note request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var note struct {
		NoteID    int64  `json:"note_id"`
		ProjectID string `json:"project_id"`
		Content   string `json:"content"`
	}
	if err := json.NewDecoder(response.Body).Decode(&note); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%d\t%s\t%s\n", note.NoteID, note.ProjectID, note.Content)
	return 0
}

func sprintHoldReason(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 3 || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(strings.Join(args[2:], " ")) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp reason <sprint-id> <reason>")
		return 2
	}
	body, err := json.Marshal(map[string]string{"reason": strings.TrimSpace(strings.Join(args[2:], " "))})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode hold reason: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + args[1] + "/hold-reason"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create hold reason request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var sprint struct {
		SprintID   string `json:"sprint_id"`
		Status     string `json:"status"`
		HoldReason string `json:"hold_reason"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sprint); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", sprint.SprintID, sprint.Status, sprint.HoldReason)
	return 0
}

func sprintAction(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp <start|hold|resume|complete|cancel> <sprint-id> [reason]")
		return 2
	}
	action := args[0]
	switch action {
	case "start", "hold", "resume", "complete", "cancel":
	default:
		_, _ = fmt.Fprintf(errOut, "unknown sprint action %q\n", action)
		return 2
	}
	reason := strings.TrimSpace(strings.Join(args[2:], " "))
	if (action == "hold" || action == "cancel") && reason == "" {
		_, _ = fmt.Fprintf(errOut, "tk sp %s requires a reason\n", action)
		return 2
	}
	body, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode sprint action reason: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + args[1] + "/" + action
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create sprint action request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var sprint struct {
		SprintID    string `json:"sprint_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sprint); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", sprint.SprintID, sprint.Status, sprint.Name, sprint.ItemAddress)
	return 0
}

func sprintEntries(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp entries <sprint-id>")
		return 2
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + args[1] + "/time-entries")
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var value struct {
		Items []struct {
			TimeEntryID     int64  `json:"time_entry_id"`
			SprintID        string `json:"sprint_id"`
			EntryType       string `json:"entry_type"`
			DurationSeconds int64  `json:"duration_seconds"`
			Reason          string `json:"reason"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return 1
	}
	for _, entry := range value.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\t%d\t%s\n", entry.TimeEntryID, entry.SprintID, entry.EntryType, entry.DurationSeconds, entry.Reason)
	}
	return 0
}

func sprintExtensions(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp extensions <sprint-id>")
		return 2
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + args[1] + "/extensions")
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var value struct {
		Items []struct {
			ExtensionID     int64  `json:"extension_id"`
			SprintID        string `json:"sprint_id"`
			DurationSeconds int64  `json:"duration_seconds"`
			Reason          string `json:"reason"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return 1
	}
	for _, extension := range value.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%d\t%s\n", extension.ExtensionID, extension.SprintID, extension.DurationSeconds, extension.Reason)
	}
	return 0
}

func sprintExtend(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 4 || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[3]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp extend <sprint-id> <duration> <reason>")
		return 2
	}
	duration, err := time.ParseDuration(args[2])
	if err != nil || duration <= 0 {
		_, _ = fmt.Fprintln(errOut, "extension duration must be positive, such as 10m")
		return 2
	}
	body, err := json.Marshal(map[string]any{"duration_seconds": int64(duration.Seconds()), "reason": args[3]})
	if err != nil {
		return 1
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/sprints/"+args[1]+"/extensions", bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var extension struct {
		ExtensionID     int64  `json:"extension_id"`
		SprintID        string `json:"sprint_id"`
		DurationSeconds int64  `json:"duration_seconds"`
		Reason          string `json:"reason"`
	}
	if err := json.NewDecoder(response.Body).Decode(&extension); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%d\t%s\t%d\t%s\n", extension.ExtensionID, extension.SprintID, extension.DurationSeconds, extension.Reason)
	return 0
}

func sprintAttempt(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 3 || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp attempt <sprint-id> <reason>")
		return 2
	}
	body, err := json.Marshal(map[string]string{"reason": strings.TrimSpace(args[2])})
	if err != nil {
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + url.PathEscape(args[1]) + "/retrieval-attempts"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var attempt struct {
		AttemptNumber int  `json:"attempt_number"`
		TimedOut      bool `json:"timed_out"`
	}
	if err := json.NewDecoder(response.Body).Decode(&attempt); err != nil {
		return 1
	}
	status := "recorded"
	if attempt.TimedOut {
		status = "TimedOut"
	}
	_, _ = fmt.Fprintf(out, "%s\tattempt=%d\t%s\n", args[1], attempt.AttemptNumber, status)
	return 0
}

func sprintAttempts(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp attempts <sprint-id>")
		return 2
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + url.PathEscape(args[1]) + "/retrieval-attempts"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(endpoint)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Items []struct {
			AttemptNumber int    `json:"attempt_number"`
			Reason        string `json:"reason"`
			TimedOut      bool   `json:"timed_out"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return 1
	}
	for _, item := range payload.Items {
		status := "recorded"
		if item.TimedOut {
			status = "TimedOut"
		}
		_, _ = fmt.Fprintf(out, "%s\tattempt=%d\t%s\t%s\n", args[1], item.AttemptNumber, status, item.Reason)
	}
	return 0
}

func sprintNext(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp next <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[1] + "/sprints/claim-next"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var sprint struct {
		SprintID    string `json:"sprint_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sprint); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", sprint.SprintID, sprint.Status, sprint.Name, sprint.ItemAddress)
	return 0
}

func sprint(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) > 0 && args[0] == "reason" {
		return sprintHoldReason(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "attempts" {
		return sprintAttempts(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "attempt" {
		return sprintAttempt(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "next" {
		return sprintNext(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "entries" {
		return sprintEntries(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "extensions" {
		return sprintExtensions(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "extend" {
		return sprintExtend(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] != "new" {
		return sprintAction(args, out, errOut, baseURL)
	}
	if (len(args) != 5 && len(args) != 6) || args[0] != "new" || (args[1] != "task" && args[1] != "subtask") || strings.TrimSpace(args[2]) == "" || strings.TrimSpace(args[3]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp new <task|subtask> <owner-id> <sprint-name> <estimate> [buffer-percent]")
		return 2
	}
	estimate, err := time.ParseDuration(args[4])
	if err != nil || estimate <= 0 {
		_, _ = fmt.Fprintln(errOut, "estimate must be a positive duration such as 30m or 2h")
		return 2
	}
	bufferPct := int64(0)
	if len(args) == 6 {
		bufferPct, err = strconv.ParseInt(args[5], 10, 64)
		if err != nil || bufferPct < 0 || bufferPct > 100 {
			_, _ = fmt.Fprintln(errOut, "buffer percent must be a whole number from 0 to 100")
			return 2
		}
	}
	body, err := json.Marshal(map[string]any{"name": args[3], "estimated_duration_seconds": int64(estimate.Seconds()), "buffer_pct": bufferPct})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode sprint input: %v\n", err)
		return 1
	}
	ownerPath := "tasks"
	if args[1] == "subtask" {
		ownerPath = "subtasks"
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/" + ownerPath + "/" + args[2] + "/sprints"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create sprint request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var sprint struct {
		SprintID    string `json:"sprint_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sprint); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", sprint.SprintID, sprint.Status, sprint.Name, sprint.ItemAddress)
	return 0
}

func subtask(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 3 && args[0] == "status" {
		return subtaskStatus(args[1], args[2], out, errOut, baseURL)
	}
	if len(args) != 4 || args[0] != "new" || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk st new <task-id> <subtask-name> <estimate>")
		return 2
	}
	estimate, err := time.ParseDuration(args[3])
	if err != nil || estimate <= 0 {
		_, _ = fmt.Fprintln(errOut, "estimate must be a positive duration such as 30m or 2h")
		return 2
	}
	body, err := json.Marshal(map[string]any{"name": args[2], "estimated_duration_seconds": int64(estimate.Seconds())})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode subtask input: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + args[1] + "/subtasks"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create subtask request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var subtask struct {
		SubtaskID   string `json:"subtask_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.NewDecoder(response.Body).Decode(&subtask); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", subtask.SubtaskID, subtask.Name, subtask.ItemAddress)
	return 0
}

func task(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 4 && args[0] == "edit" && strings.TrimSpace(args[1]) != "" {
		return taskEdit(args[1], args[2], args[3], out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "status" {
		return taskStatus(args[1], args[2], out, errOut, baseURL)
	}
	if len(args) != 5 || args[0] != "new" || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" || strings.TrimSpace(args[3]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk t new <project-id> <category-id> <task-name> <estimate>")
		return 2
	}
	estimate, err := time.ParseDuration(args[4])
	if err != nil || estimate <= 0 {
		_, _ = fmt.Fprintln(errOut, "estimate must be a positive duration such as 30m or 2h")
		return 2
	}
	body, err := json.Marshal(map[string]any{"category_id": args[2], "name": args[3], "estimated_duration_seconds": int64(estimate.Seconds())})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode task input: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[1] + "/tasks"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create task request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var task struct {
		TaskID      string `json:"task_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", task.TaskID, task.Name, task.ItemAddress)
	return 0
}

func subtaskStatus(subtaskID, status string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"status": status})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/subtasks/" + subtaskID + "/status"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var subtask struct {
		SubtaskID string `json:"subtask_id"`
		Status    string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&subtask); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\n", subtask.SubtaskID, subtask.Status)
	return 0
}

func taskEdit(taskID, goal, description string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"goal": goal, "description": description})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + taskID + "/metadata"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var task struct {
		TaskID      string `json:"task_id"`
		Goal        string `json:"goal"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", task.TaskID, task.Goal, task.Description)
	return 0
}

func taskStatus(taskID, status string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"status": status})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + taskID + "/status"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var task struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\n", task.TaskID, task.Status)
	return 0
}

func categoryEdit(categoryID, goal, description string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"goal": goal, "description": description})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/categories/" + categoryID + "/metadata"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var category struct {
		CategoryID string `json:"category_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&category); err != nil {
		return 1
	}
	_, _ = fmt.Fprintln(out, category.CategoryID)
	return 0
}

func categoryStatus(categoryID, status string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"status": status})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/categories/" + categoryID + "/status"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create category status request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var category struct {
		CategoryID string `json:"category_id"`
		Status     string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&category); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\n", category.CategoryID, category.Status)
	return 0
}

func category(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 4 && args[0] == "edit" && strings.TrimSpace(args[1]) != "" {
		return categoryEdit(args[1], args[2], args[3], out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "status" && strings.TrimSpace(args[1]) != "" && strings.TrimSpace(args[2]) != "" {
		return categoryStatus(args[1], args[2], out, errOut, baseURL)
	}
	if (len(args) != 3 && len(args) != 4) || args[0] != "new" || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk c new <project-id> <category-name> [parent-category-id]")
		return 2
	}
	input := map[string]string{"name": args[2]}
	if len(args) == 4 && strings.TrimSpace(args[3]) != "" {
		input["parent_category_id"] = args[3]
	}
	body, err := json.Marshal(input)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode category input: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[1] + "/categories"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create category request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var category struct {
		CategoryID  string `json:"category_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.NewDecoder(response.Body).Decode(&category); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", category.CategoryID, category.Name, category.ItemAddress)
	return 0
}

func project(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 4 && args[0] == "edit" && strings.TrimSpace(args[1]) != "" {
		return projectEdit(args[1], args[2], args[3], out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "status" && strings.TrimSpace(args[1]) != "" && strings.TrimSpace(args[2]) != "" {
		return projectStatus(args[1], args[2], out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "alias" && strings.TrimSpace(args[1]) != "" {
		return projectAlias(args[1:], out, errOut, baseURL)
	}
	if len(args) == 2 && args[0] == "unalias" && strings.TrimSpace(args[1]) != "" {
		return projectUnalias(args[1:], out, errOut, baseURL)
	}
	if len(args) != 2 || args[0] != "new" || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk p new <project-name>")
		return 2
	}
	body, err := json.Marshal(map[string]string{"name": args[1]})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode project input: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create project request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var project struct {
		ProjectID   string `json:"project_id"`
		ProjectName string `json:"project_name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", project.ProjectID, project.ProjectName, project.ItemAddress)
	return 0
}

func projectStatus(projectID, status string, out, errOut io.Writer, baseURL string) int {
	body, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/status"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "update Project status request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var project struct {
		ProjectID string `json:"project_id"`
		Status    string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\n", project.ProjectID, project.Status)
	return 0
}

func projectEdit(projectID, goal, description string, out, errOut io.Writer, baseURL string) int {
	body, err := json.Marshal(map[string]string{"goal": goal, "description": description})
	if err != nil {
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/metadata"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "update Project metadata request: ", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var project struct {
		ProjectID   string `json:"project_id"`
		Goal        string `json:"project_goal"`
		Description string `json:"project_description"`
	}
	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s	%s	%s\n", project.ProjectID, project.Goal, project.Description)
	return 0
}

func resolveProjectID(baseURL, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("project id or alias is required")
	}
	upper := strings.ToUpper(value)
	if strings.HasPrefix(upper, "P-") || value == "pulse" {
		return value, nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects"
	response, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("Time Keeper API unavailable at %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Time Keeper API returned %s", response.Status)
	}
	var payload struct {
		Items []struct {
			ProjectID    string `json:"project_id"`
			ProjectName  string `json:"project_name"`
			ProjectAlias string `json:"project_alias"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("Time Keeper API returned invalid JSON: %w", err)
	}
	needle := strings.TrimSpace(value)
	for _, item := range payload.Items {
		if item.ProjectAlias != "" && item.ProjectAlias == needle {
			return item.ProjectID, nil
		}
	}
	return value, nil
}

func projectAlias(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk p alias <project-id> <alias>")
		return 2
	}
	resolvedID, err := resolveProjectID(baseURL, args[0])
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	body, err := json.Marshal(map[string]string{"alias": args[1]})
	if err != nil {
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + resolvedID + "/alias"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "create alias request: ", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var project struct {
		ProjectID    string `json:"project_id"`
		ProjectAlias string `json:"project_alias"`
	}
	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s	%s\n", project.ProjectID, project.ProjectAlias)
	return 0
}

func projectUnalias(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk p unalias <project-id>")
		return 2
	}
	resolvedID, err := resolveProjectID(baseURL, args[0])
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	body, err := json.Marshal(map[string]string{"alias": ""})
	if err != nil {
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + resolvedID + "/alias"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "clear alias request: ", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s	\n", resolvedID)
	return 0
}

func listAliases(out, errOut io.Writer, baseURL string) int {
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects"
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Items []model.Project `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	for _, project := range payload.Items {
		if strings.TrimSpace(project.ProjectAlias) != "" {
			_, _ = fmt.Fprintf(out, "%s	%s	%s\n", project.ProjectID, project.ProjectAlias, project.ProjectName)
		}
	}
	return 0
}

func doctor(out, errOut io.Writer, baseURL string) int {
	urls := []string{
		strings.TrimRight(baseURL, "/") + "/health",
		"http://timekeeper.local/health",
		"http://api.timekeeper.local/health",
	}
	client := &http.Client{Timeout: 5 * time.Second}
	allOK := true
	canonicalOK := false
	for _, u := range urls {
		ok, status, err := probeHealth(client, u)
		if err != nil {
			// A connection error means the friendly URL is not
			// resolvable, the proxy is not listening, or the host
			// is unreachable. Treat as a failure: the user is
			// asking "is TimeKeeper usable?" and the answer is
			// "partially".
			_, _ = fmt.Fprintf(out, "[failed] %s: %v\n", u, err)
			allOK = false
			continue
		}
		if !ok {
			_, _ = fmt.Fprintf(out, "[failed] %s: HTTP %s\n", u, status)
			allOK = false
			continue
		}
		_, _ = fmt.Fprintf(out, "[ok] %s: %s\n", u, status)
		if u == urls[0] {
			canonicalOK = true
		}
	}

	// Installation sanity check: the install-commit / install-path that
	// the running binary was built from. This catches the case where
	// the user updated their local checkout but the service / launcher
	// is still running the old binary.
	if err := reportInstallInfo(out); err != nil {
		_, _ = fmt.Fprintf(out, "[skip] install info: %v\n", err)
	}

	if allOK {
		_, _ = fmt.Fprintln(out, "\nTime Keeper is ready.")
		return 0
	}
	if canonicalOK {
		_, _ = fmt.Fprintln(out, "\nTime Keeper is reachable on the canonical address, but the friendly URLs are not.")
		_, _ = fmt.Fprintln(out, "Run the installer, or set TIMEKEEPER_PROXY_ADDR to a free port, then restart.")
		return 1
	}
	_, _ = fmt.Fprintln(out, "\nTime Keeper is not fully ready.")
	_, _ = fmt.Fprintln(out, "Start the service, then run tk doctor again.")
	return 1
}

// reportInstallInfo prints a one-line summary of the install that the
// current binary was built from. The installer writes INSTALLATION.env
// next to the binary; if it is missing (e.g. running from a manually
// copied binary) the call returns an error and the doctor prints
// "[skip] install info: ..." rather than failing the whole check.
func reportInstallInfo(out io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// Look for INSTALLATION.env in the binary's directory and one
	// level up (the install layout puts the file at .timekeeper/app/
	// while the binary is at .timekeeper/app/bin/).
	dir := filepath.Dir(exe)
	for _, rel := range []string{"INSTALLATION.env", filepath.Join("..", "INSTALLATION.env")} {
		candidate := filepath.Join(dir, rel)
		body, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		commit := readEnvLine(string(body), "SOURCE_COMMIT")
		target := readEnvLine(string(body), "TARGET_GOOS")
		proxyAddr := readEnvLine(string(body), "TIMEKEEPER_PROXY_ADDR")
		proxyDisabled := readEnvLine(string(body), "TIMEKEEPER_PROXY_DISABLED")
		short := commit
		if len(short) > 12 {
			short = short[:12]
		}
		_, _ = fmt.Fprintf(out, "[ok] install: target=%s commit=%s", target, short)
		if proxyAddr != "" {
			_, _ = fmt.Fprintf(out, " proxy=%s", proxyAddr)
		}
		if proxyDisabled == "1" {
			_, _ = fmt.Fprint(out, " proxy=disabled")
		}
		_, _ = fmt.Fprintln(out)
		return nil
	}
	return fmt.Errorf("INSTALLATION.env not found next to the binary")
}

// readEnvLine is a tiny KEY=VALUE reader that tolerates shell-style
// quoting (single or double quotes) so the installer can store the
// values in a human-readable form.
func readEnvLine(body, key string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, key+"=") {
			continue
		}
		value := strings.TrimPrefix(line, key+"=")
		value = strings.Trim(value, "\"'")
		return value
	}
	return ""
}

// probeHealth does a single GET against the health endpoint. It
// returns (ok, statusString, err). A connection error is reported
// as err (and status is empty). An HTTP error is reported as !ok
// with the status string set.
func probeHealth(client *http.Client, u string) (bool, string, error) {
	resp, err := client.Get(u)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, resp.Status, nil
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, resp.Status, fmt.Errorf("invalid JSON: %w", err)
	}
	if payload.Status != "ok" {
		return false, resp.Status, nil
	}
	return true, "ok", nil
}

// apiHelp fetches and displays all available API routes from the server.
func apiHelp(out, errOut io.Writer, baseURL string) int {
	url := strings.TrimRight(baseURL, "/") + "/api/help"
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "API unreachable: %s (%v)\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "API returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Service string              `json:"service"`
		Version string              `json:"version"`
		Routes  []map[string]string `json:"routes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Failed to decode API help: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "Time Keeper API — %s v%s\n\n", payload.Service, payload.Version)
	for _, r := range payload.Routes {
		_, _ = fmt.Fprintf(out, "  %-6s %-55s %s\n", r["method"], r["path"], r["desc"])
	}
	_, _ = fmt.Fprintf(out, "\n%d routes\n", len(payload.Routes))
	return 0
}

func tree(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk tree <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/execution-tree"
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload model.ExecutionTree
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s	%.1f%%	%s\n", payload.Project.ProjectID, payload.Project.CalculatedCompletionPct, payload.Project.ProjectName)
	for _, category := range payload.Categories {
		writeCategoryTree(out, category, 1)
	}
	return 0
}

func writeCategoryTree(out io.Writer, category model.ExecutionCategory, depth int) {
	indent := strings.Repeat("  ", depth)
	_, _ = fmt.Fprintf(out, "%s%s	%s\n", indent, category.Category.CategoryID, category.Category.Name)
	for _, child := range category.Categories {
		writeCategoryTree(out, child, depth+1)
	}
	for _, task := range category.Tasks {
		_, _ = fmt.Fprintf(out, "%s  %s	%s	%s\n", indent, task.Task.TaskID, task.Task.Status, task.Task.Name)
		for _, sprint := range task.Sprints {
			_, _ = fmt.Fprintf(out, "%s    %s	%s	%s\n", indent, sprint.SprintID, sprint.Status, sprint.Name)
		}
		for _, subtask := range task.Subtasks {
			_, _ = fmt.Fprintf(out, "%s    %s	%s	%s\n", indent, subtask.Subtask.SubtaskID, subtask.Subtask.Status, subtask.Subtask.Name)
			for _, sprint := range subtask.Sprints {
				_, _ = fmt.Fprintf(out, "%s      %s	%s	%s\n", indent, sprint.SprintID, sprint.Status, sprint.Name)
			}
		}
	}
}

func list(out, errOut io.Writer, baseURL string) int {
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects"
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Items []struct {
			ProjectID               string  `json:"project_id"`
			ProjectName             string  `json:"project_name"`
			Status                  string  `json:"status"`
			CalculatedCompletionPct float64 `json:"calculated_completion_pct"`
			ItemAddress             string  `json:"item_address"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	for _, project := range payload.Items {
		_, _ = fmt.Fprintf(out, "%s	%s	%.1f%%	%s	%s\n", project.ProjectID, project.Status, project.CalculatedCompletionPct, project.ProjectName, project.ItemAddress)
	}
	return 0
}
