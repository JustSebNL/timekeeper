// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/logging"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/ntfy"
	"github.com/JustSebNL/timekeeper/internal/version"
)

// Default endpoints, in priority order.
//
// timekeeper.local is the point of contact: it is the name the friendly-URL
// proxy was configured to serve, and it is what a human should see first.
// 127.0.0.1:1618 stays as the loopback fallback for machines without the hosts
// entry (and for tools that must not depend on name resolution).
//
// Resolution order for a request: explicit --url, then TIMEKEEPER_URL, then
// the first reachable of preferred/fallback.
const (
	// The friendly URL is the advertised/default contact point. The numeric
	// loopback listener remains the implementation and diagnostic fallback.
	preferredBaseURL = "http://timekeeper.local"
	fallbackBaseURL  = "http://127.0.0.1:1618"
)

// defaultBaseURL is the first candidate. Kept as the historical name because
// callers and tests reference the concept, not the literal.
const defaultBaseURL = preferredBaseURL

// defaultBaseURLs is the full fallback chain, preferred first.
func defaultBaseURLs() []string { return []string{preferredBaseURL, fallbackBaseURL} }

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

var jsonOutputEnabled bool

func extractJSONFlag(args []string) ([]string, bool) {
	jsonMode := false
	out := args[:0:0]
	for _, a := range args {
		if a == "--json" {
			jsonMode = true
			continue
		}
		out = append(out, a)
	}
	return out, jsonMode
}

func writeJSONResponse(out io.Writer, val any) {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(val)
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
		logPath = filepath.Join("/data/logs", "cli.log")
	}
	logging.Init(logging.Config{Path: logPath, Mode: mode, MaxSizeMiB: 5, MaxBackups: 3})
}

// parseDuration accepts standard Go duration strings ("30m", "2h") and bare
// positive integers interpreted as seconds ("3600" -> 1h). This keeps CLI
// estimates consistent across t new, sp new, and sp extend.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("duration is required")
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d > 0 {
			return d, nil
		}
		return 0, fmt.Errorf("duration must be positive")
	}

	// A unitless integer is a seconds shorthand. Keep the conversion bounded;
	// otherwise a large valid int64 can overflow time.Duration and become a
	// negative duration after multiplication.
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil || sec <= 0 {
		return 0, fmt.Errorf("invalid duration %q: use 30m, 2h, or positive seconds", s)
	}
	const maxSeconds = int64(^uint64(0)>>1) / int64(time.Second)
	if sec > maxSeconds {
		return 0, fmt.Errorf("duration %q is too large", s)
	}
	return time.Duration(sec) * time.Second, nil
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
		if logging.IsDebug() {
			logger.Error("cli command failed", append(attrs, slog.String("trace", string(debug.Stack())))...)
		} else {
			logger.Error("cli command failed", attrs...)
		}
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
//
// Precedence: an explicit --url wins, then TIMEKEEPER_URL, then the first
// reachable default endpoint (timekeeper.local, else 127.0.0.1:1618). Only the
// default path is probed; an operator-supplied address is used verbatim.
func ResolveBaseURL(args []string, environmentURL string) (string, []string, error) {
	// Parse --url before probing defaults. An explicit endpoint is an operator
	// instruction and must never incur a probe or be silently redirected.
	if len(args) > 0 && args[0] == "--url" {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return "", nil, fmt.Errorf("--url requires an API base URL")
		}
		return strings.TrimSpace(args[1]), args[2:], nil
	}
	if len(args) > 0 {
		if value, ok := strings.CutPrefix(args[0], "--url="); ok {
			if strings.TrimSpace(value) == "" {
				return "", nil, fmt.Errorf("--url requires an API base URL")
			}
			return strings.TrimSpace(value), args[1:], nil
		}
	}

	baseURL := strings.TrimSpace(environmentURL)
	if baseURL == "" {
		if isLocalHarnessCommand(args) {
			// Harness documents are bundled local files; do not make their
			// listing or display depend on the API or an endpoint probe.
			baseURL = preferredBaseURL
		} else {
			baseURL = resolvePreferredBaseURL()
		}
	}
	return baseURL, args, nil
}

func isLocalHarnessCommand(args []string) bool {
	normalized, _ := extractVerboseFlag(args)
	normalized, _ = extractJSONFlag(normalized)
	return len(normalized) > 0 && normalized[0] == "harness"
}

// Run executes a framework-neutral Time Keeper CLI command.
func Run(args []string, out, errOut io.Writer, baseURL string) int {
	// Pull -v/--verbose/--debug out of the front of args to flip log mode.
	args, mode := extractVerboseFlag(args)
	ensureCLILogger(mode)
	args, jsonOutputEnabled = extractJSONFlag(args)
	defer func() { jsonOutputEnabled = false }()

	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, _ = fmt.Fprint(out, "Usage: tk [--url <api-base-url>] <command>\n\nCommands:\n  list                                        List projects\n  tree <project-id> [--task <T-ID>] [--status S] [--active] [--depth N] [--limit N]  Show full or focused hierarchy\n  export <project-id> [file] [--task T-ID] [--format F]  Export full or task-focused Project snapshot\n  p schedule <project-id> start=... due=...   Set Project dates, timezone, and deadline warning window\n  p phase <project-id> <phase> [reason]        Advance or replan the Project lifecycle\n  p milestones list|add|date|status ...        Manage dated Project checkpoints\n  p deadlines                                List due-soon and overdue Project/milestone items\n  summary <project-id>                        Show a durable Sprint operational snapshot\n  usage <project-id>                          Show recorded agent token usage\n  usage record <project-id> <session-id> <agent-id> <model> <turn> <input> <output> [cache-write] [cache-read] [messages] [sprint-id]\n  usage ingest <project-id> <envelope.json>    Ingest a redaction-verified harness usage snapshot\n  supervise --project P --sprint SP --agent ID --model MODEL --state-dir DIR -- PROMPT\n                                              Run a bounded Codex CLI worker (supervise --help)\n  outcomes <project-id>                        Compare reviewed coding runs\n  outcomes record <project-id> <review.json>    Record evidence-linked review\n  pulse                                       Show local Sprint attention needing follow-up\n  agent identity                              Print the resolved agent/session identity\n  agent state [agent-id]                      Show one agent's derived work state\n  agent states                                List all derived agent work states\n  agent work <sprint-id> <lease>              Record active material work for this session\n  agent wait <duration> <reason>              Record an explicit waiting state\n  agent idle [reason]                         Mark this session idle without inventing progress\n  agent stop [reason]                         Stop this session's work tracking\n  agent progress <id> <lease> [sprint-id] [guardian-url]\n                                              Renew an agent material-progress lease and optionally register a numeric-loopback Guardian\n  agent nudges <id>                           List durable unacknowledged Guardian nudges\n  agent history <id>                          List durable Guardian delivery/recovery history\n  agent ack <id> <nudge-id>                   Acknowledge a Guardian nudge and renew the lease\n  events <project-id>                         List immutable Project activity\n  note <project-id> <content>                 Record a Project note\n  notes <project-id>                          List Project notes\n  msg <project-id> add <body>                 Post project memory (use --reply-to M-1 for a reply)\n  msg <project-id> edit M-1 <body>            Edit a message and append a revision\n  msg <project-id> thread M-1                 Read a complete message thread\n  msg <project-id> revisions M-1              Read immutable message revisions\n  msg <project-id> search <query>             Search project memory\n  msg <project-id> list [--kind K] [--limit N] List project memory as JSON\n  msg <project-id> show M-1                   Read one full message body as JSON\n  msg <project-id> context M-1                Read anchor, thread, related and revisions\n  msg <project-id> seed --yes                 Write sample messages (refused without --yes)\n  handoff <project-id>                        Print an agent session handoff snapshot\n  p new <name>                                Create a Project\n  p edit <project-id> <goal> <description>    Update Project context\n  p status <project-id> <status> [reason]        Set Project status; completion/cancellation needs evidence\n  p alias <project-id> <alias>                Set Project alias\n  p unalias <project-id>                      Clear Project alias\n  aliases                                     List project aliases\n  c new <project-id> <name> [parent-category-id] Create a Category\n  c edit <category-id> <goal> <description>    Update Category context\n  c status <category-id> <status>              Set Category status\n  t show <task-id> [--json]                   Show Task context, children and completion blockers\n  t edit <task-id> <goal> <description>        Update Task context\n  t new <project-id> <category-id> <name> <estimate> [--no-autoclose]\n                                              Create a Task\n  t status <task-id> <status>                 Set Task status\n  t batch <project-id> <file>                 Batch-create tasks from a file (JSON, CSV, TSV, XLSX, YAML, SQL)\n  st show <subtask-id> [--json]                Show Subtask context and Sprints\n  st new <task-id> <name> <estimate>          Create a Subtask\n  st status <subtask-id> <status>              Set Subtask status\n  sp new <task|subtask> <owner-id> <name> <estimate> [buffer-percent]\n                                              Create a Sprint\n  sp <start|hold|resume|complete|cancel> <sprint-id> [reason] Transition a Sprint; hold/cancel require a reason\n  sp reason <sprint-id> <reason>                Update why an already-held Sprint is blocked\n  sp next <project-id>                         Atomically claim the oldest runnable Sprint\n  sp attempts <sprint-id>                      List immutable retrieval-attempt evidence\n  sp attempt <sprint-id> <reason>              Record a failed retrieval attempt (fourth makes TimedOut)\n  sp extend <sprint-id> <duration> <reason>   Record justified additional planned time\n  sp extensions <sprint-id>                   List immutable extension history\n  sp entries <sprint-id>                      List recorded work/hold intervals\n  llm new <name> <provider> <base-url> <model> [system-prompt]\n                                              Register a loopback LLM pipeline\n  plan <generate|apply> <project-id> <pipeline-id|draft-id>\n                                              Generate or apply a reviewed planning draft\n  plan list <project-id>                       List planning drafts\n  commit [message] [-a|--all]                 Create an attributed commit and post to Project messages\n  sync                                        Sync repo with remote and log event\n  git status (or status)                      Show repo and sprint status\n  doctor                                      Check whether Time Keeper is reachable\n  api-help                                    List all available API routes\n  store status                                Show Store login state and installation id\n  store login <email> --password <password>   Authenticate against the Store (optional)\n  store logout                                Revoke this Store session\n  store catalogue                             List Store extensions\n  store claim <product-id>                    Claim a free extension\n  store install <product-id> [version]        Verify, download and install an extension\n  store list                                  Show this account's entitlements\n  service                                     Manage TimeKeeper OS service (install/uninstall/start/stop/status/logs)\n")
		_, _ = fmt.Fprintln(out, "  harness list/show <item>                   List and read bundled harness guidance")
		_, _ = fmt.Fprintln(out, "  queue list|show|flush [--limit N]               Inspect the local outbox or deliver queued registrations")
		_, _ = fmt.Fprintln(out, "  search <query...> [--projects|--tasks|--messages] [--project P-ID] [--limit N] [--kind K] [--tags T1,T2] [--after DATE] [--before DATE]  Search across projects, tasks, and messages")
		_, _ = fmt.Fprintln(out, "  tc add|attach|edit|lookup|list|show|pin|unpin|delete|restore|categories|revisions <project>  Manage project Treasure Chest gems")
		_, _ = fmt.Fprintln(out, "  t batch <project-id> <file> [--format format] [--check-only]  Smoke-test first; nested JSON/YAML applies atomically")
		return 0
	}
	setActiveCommand(args)
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
	case "queue":
		return runRegistrationQueueCommand(args[1:], out, errOut, baseURL)
	case "st":
		return subtask(args[1:], out, errOut, baseURL)
	case "sp":
		return sprint(args[1:], out, errOut, baseURL)
	case "note":
		return note(args[1:], out, errOut, baseURL)
	case "export":
		return exportProject(args[1:], out, errOut, baseURL)
	case "import":
		return importProject(args[1:], out, errOut, baseURL)
	case "summary":
		return summary(args[1:], out, errOut, baseURL)
	case "handoff", "context":
		return handoffCLI(args[1:], out, errOut, baseURL)
	case "lease":
		return runCommand("lease", func() int { return leaseCommand(args[1:], out, errOut, baseURL) })
	case "checkpoint":
		return runCommand("checkpoint", func() int { return checkpointCommand(args[1:], out, errOut, baseURL) })
	case "watch", "stream":
		return runCommand("watch", func() int { return watchCommand(args[1:], out, errOut, baseURL) })
	case "connect":
		return runCommand("connect", func() int { return connectCommand(args[1:], out, errOut, baseURL) })
	case "connector":
		if len(args) > 1 && (args[1] == "create" || args[1] == "new") {
			return runCommand("connector", func() int { return connectorCreateCommand(args[2:], out, errOut, baseURL) })
		}
		fmt.Fprintln(errOut, "unknown connector command")
		return 1
	case "supervise":
		return supervise(args[1:], out, errOut, baseURL)
	case "outcomes":
		return outcomes(args[1:], out, errOut, baseURL)
	case "usage":
		return usage(args[1:], out, errOut, baseURL)
	case "pulse":
		return pulse(args[1:], out, errOut, baseURL)
	case "agent":
		if len(args) > 1 && args[1] == "bootstrap" {
			return runCommand("agent", func() int { return agentBootstrap(out, errOut) })
		}
		return agentPulse(args[1:], out, errOut, baseURL)
	case "agents":
		return agentsCommand(args[1:], out, errOut, baseURL)
	case "init":
		if len(args) > 1 && args[1] == "--agent" {
			return runCommand("init", func() int { return agentBootstrap(out, errOut) })
		}
		return tkInit(args[1:], out, errOut, baseURL)
	case "guardian":
		if len(args) > 1 && args[1] == "wake" {
			return guardianWakeCLI(args[2:], out, errOut, baseURL)
		}
		return guardianSelfNudge(args[1:], out, errOut, baseURL)
	case "events":
		return events(args[1:], out, errOut, baseURL)
	case "notes":
		return notes(args[1:], out, errOut, baseURL)
	case "msg", "messages":
		return messages(args[1:], out, errOut, baseURL)
	case "tc":
		return treasureChest(args[1:], out, errOut, baseURL)
	case "search":
		return globalSearchCLI(args[1:], out, errOut, baseURL)
	case "list":
		return list(out, errOut, baseURL)
	case "tree":
		return tree(args[1:], out, errOut, baseURL)
	case "doctor":
		return doctor(out, errOut, baseURL)
	case "open":
		return openBrowser(args[1:], out, errOut, baseURL)
	case "uninstall":
		return uninstallTimeKeeper(out, errOut, baseURL)
	case "hosts":
		return hostsSubcommand(args[1:], out, errOut)
	case "aliases":
		return listAliases(out, errOut, baseURL)
	case "api-help":
		return apiHelp(out, errOut, baseURL)
	case "harness":
		return harnessCommand(args[1:], out, errOut)
	case "version":
		_, _ = fmt.Fprintln(out, version.String())
		return 0
	case "ntfy":
		return ntfyCommand(args[1:], out, errOut, baseURL)
	case "service":
		return serviceManager(args[1:], out, errOut, baseURL)
	case "store":
		if len(args) > 1 && (args[1] == "search" || args[1] == "install" || args[1] == "list" || args[1] == "pack" || args[1] == "publish") {
			return runCommand("store", func() int { return storeConnectors(args[1:], out, errOut, baseURL) })
		}
		return runCommand("store", func() int { return storeCommand(args[1:], out, errOut, baseURL) })
	case "wt", "worktree":
		return runCommand("worktree", func() int { return worktreeCLI(args[1:], out, errOut, baseURL) })
	case "commit":
		return runCommand("commit", func() int { return gitCommitCLI(args[1:], out, errOut, baseURL) })
	case "sync":
		return runCommand("sync", func() int { return gitSyncCLI(args[1:], out, errOut, baseURL) })
	case "git":
		return runCommand("git", func() int { return gitCLI(args[1:], out, errOut, baseURL) })
	case "status":
		return runCommand("git", func() int { return gitCLI([]string{"status"}, out, errOut, baseURL) })
	default:
		_, _ = fmt.Fprintf(errOut, "unknown command %q\n", args[0])
		if suggestion := suggestCommand(args[0]); suggestion != "" {
			_, _ = fmt.Fprintf(errOut, "did you mean %q?\n", suggestion)
		}
		_, _ = fmt.Fprint(errOut, "Run tk help for usage.\n")
		return 2
	}
}

// topLevelCommands lists the verbs Run dispatches on. It is the single source
// used to suggest a correction instead of only saying "unknown command".
var topLevelCommands = []string{
	"agent", "agents", "aliases", "api-help", "c", "commit", "doctor", "events", "export",
	"git", "guardian", "handoff", "harness", "hosts", "init", "list", "llm", "msg", "messages",
	"note", "notes", "ntfy", "open", "outcomes", "p", "plan", "pulse", "service",
	"sp", "st", "status", "store", "summary", "supervise", "sync", "t", "tree", "uninstall",
	"usage", "version", "worktree", "wt",
}

// routedCommands maps words that belong to a subcommand onto the full route
// that owns them, so `tk search q` points at `msg <project-id> search` instead
// of dead-ending on "unknown command".
var routedCommands = map[string]string{
	"search": "msg <project-id> search",
	"show":   "msg <project-id> show",
	"seed":   "msg <project-id> seed --yes",
	"ls":     "list",
}

// suggestCommand returns the closest known command to input, or "" when
// nothing is close enough to be a useful suggestion.
func suggestCommand(input string) string {
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return ""
	}
	type candidate struct {
		word      string
		suggested string
	}
	options := make([]candidate, 0, len(topLevelCommands)+len(routedCommands))
	for _, word := range topLevelCommands {
		options = append(options, candidate{word: word, suggested: word})
	}
	routed := make([]string, 0, len(routedCommands))
	for word := range routedCommands {
		routed = append(routed, word)
	}
	sort.Strings(routed)
	for _, word := range routed {
		options = append(options, candidate{word: word, suggested: routedCommands[word]})
	}
	best := ""
	bestDistance := 1 << 30
	for _, option := range options {
		distance := editDistance(lower, option.word)
		if distance < bestDistance {
			bestDistance = distance
			best = option.suggested
		}
	}
	// A third of the word is the most noise worth suggesting; beyond that the
	// guess would be misleading rather than helpful.
	if bestDistance > (len(lower)+2)/3 {
		return ""
	}
	return best
}

// editDistance is the bounded Levenshtein distance between two ASCII-ish
// command words.
func editDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(current[j-1]+1, min(previous[j]+1, previous[j-1]+cost))
		}
		previous = current
	}
	return previous[len(b)]
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
	response, err := newHTTPClient(60 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "run planning request", response)
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
	response, err := newHTTPClient(10 * time.Second).Get(strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/planning-drafts")
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "list planning drafts", response)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "register llm pipeline", response)
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
	if len(args) >= 2 {
		return projectExportMulti(args, out, errOut, baseURL)
	}
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk export <project-id> [file] [--task <T-ID>] [--format F]")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/export"
	response, err := newHTTPClient(10 * time.Second).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "export project", response)
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
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, _ = fmt.Fprintln(out, "usage: tk summary <project-id>")
		return 0
	}
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk summary <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/operational-summary"
	response, err := newHTTPClient(10 * time.Second).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "read project summary", response)
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

func handoffCLI(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk handoff <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/handoff"
	if !jsonOutputEnabled {
		url += "?format=markdown"
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "failed to create request: %v\n", err)
		return 1
	}
	if !jsonOutputEnabled {
		req.Header.Set("Accept", "text/markdown")
	}
	resp, err := newHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "read agent handoff snapshot", resp)
		return 1
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "failed to read response: %v\n", err)
		return 1
	}
	_, _ = out.Write(body)
	if !strings.HasSuffix(string(body), "\n") {
		_, _ = fmt.Fprintln(out)
	}
	return 0
}

func pulse(args []string, out, errOut io.Writer, baseURL string) int {
	activeOnly := false
	staleOnly := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--active-only":
			activeOnly = true
		case "--stale":
			staleOnly = true
		}
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/pulse"
	response, err := newHTTPClient(10 * time.Second).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "TimeKeeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "read sprint pulse", response)
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
		_, _ = fmt.Fprintf(errOut, "TimeKeeper API returned invalid JSON: %v\n", err)
		return 1
	}
	if value.Format != "timekeeper-pulse/v1" {
		_, _ = fmt.Fprintf(errOut, "TimeKeeper API returned unsupported Pulse format %q\n", value.Format)
		return 1
	}

	// Filter items based on flags. Stale = overdue > 3x planned.
	type pulseItem struct {
		Kind                   string
		ProjectID              string
		SprintID               string
		Name                   string
		PlannedDurationSeconds int64
		ActiveDurationSeconds  int64
		OverdueDurationSeconds int64
	}
	filtered := make([]pulseItem, 0, len(value.Attention))
	for _, item := range value.Attention {
		isStale := item.OverdueDurationSeconds > item.PlannedDurationSeconds*3
		if activeOnly && isStale {
			continue
		}
		if staleOnly && !isStale {
			continue
		}
		filtered = append(filtered, pulseItem{
			Kind:                   item.Kind,
			ProjectID:              item.ProjectID,
			SprintID:               item.SprintID,
			Name:                   item.Name,
			PlannedDurationSeconds: item.PlannedDurationSeconds,
			ActiveDurationSeconds:  item.ActiveDurationSeconds,
			OverdueDurationSeconds: item.OverdueDurationSeconds,
		})
	}

	if len(filtered) == 0 {
		_, _ = fmt.Fprintf(out, "clear	next=%ds\n", value.RecommendedNextPulseSeconds)
		return 0
	}
	for _, item := range filtered {
		_, _ = fmt.Fprintf(out, "%s	%s	%s	active=%ds	planned=%ds	overdue=%ds	%s\n", item.Kind, item.ProjectID, item.SprintID, item.ActiveDurationSeconds, item.PlannedDurationSeconds, item.OverdueDurationSeconds, item.Name)
	}
	return 0
}

func agentStatus(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk agent status <agent-id>")
		return 2
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + url.PathEscape(args[0]) + "/progress"
	response, err := newHTTPClient(10 * time.Second).Get(endpoint)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "TimeKeeper API unavailable at %s: %v\n", endpoint, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "read agent state", response)
		return 1
	}
	var progress struct {
		AgentID              string `json:"agent_id"`
		ActiveSprintID       string `json:"active_sprint_id"`
		LeaseDurationSeconds int64  `json:"lease_duration_seconds"`
		GuardianURL          string `json:"guardian_url"`
		LastProgressAt       string `json:"last_progress_at"`
		UpdatedAt            string `json:"updated_at"`
	}
	if err := json.NewDecoder(response.Body).Decode(&progress); err != nil {
		_, _ = fmt.Fprintf(errOut, "TimeKeeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\tlease=%ds\tsprint=%s\tguardian=%s\tlast=%s\tupdated=%s\n",
		progress.AgentID, progress.LeaseDurationSeconds, progress.ActiveSprintID,
		progress.GuardianURL, progress.LastProgressAt, progress.UpdatedAt)
	return 0
}

func agentSessions(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk agent sessions <agent-id>")
		return 2
	}
	agentID := strings.TrimSpace(args[0])
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + url.PathEscape(agentID) + "/sessions"
	response, err := newHTTPClient(10 * time.Second).Get(endpoint)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "TimeKeeper API unavailable at %s: %v\n", endpoint, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "list agent sessions", response)
		return 1
	}
	var payload struct {
		Items []struct {
			ProjectID      string `json:"project_id"`
			SessionID      string `json:"session_id"`
			AgentID        string `json:"agent_id"`
			Model          string `json:"model"`
			TurnSeq        int64  `json:"turn_seq"`
			InputTokens    int64  `json:"input_tokens"`
			OutputTokens   int64  `json:"output_tokens"`
			Messages       int64  `json:"messages"`
			LastActivityAt string `json:"last_activity_at"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "TimeKeeper API returned invalid JSON: %v\n", err)
		return 1
	}
	if len(payload.Items) == 0 {
		_, _ = fmt.Fprintf(out, "no sessions found for agent %s\n", agentID)
		return 0
	}
	for _, s := range payload.Items {
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\tturn=%d\tinput=%d\toutput=%d\tmessages=%d\tlast=%s\n",
			s.ProjectID, s.SessionID, s.Model, s.TurnSeq, s.InputTokens, s.OutputTokens, s.Messages, s.LastActivityAt)
	}
	return 0
}

func ntfyCommand(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(errOut, "usage: tk ntfy publish --server <url> --topic <name> [--token <token>] --nudge <id> --agent <id>")
		_, _ = fmt.Fprintln(errOut, "       tk ntfy wake --agent <id>")
		return 2
	}
	switch args[0] {
	case "publish":
		var server, topic, token, agentID string
		var nudgeID int64
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--server":
				if i+1 < len(args) {
					server = args[i+1]
					i++
				}
			case "--topic":
				if i+1 < len(args) {
					topic = args[i+1]
					i++
				}
			case "--token":
				if i+1 < len(args) {
					token = args[i+1]
					i++
				}
			case "--nudge":
				if i+1 < len(args) {
					nudgeID, _ = strconv.ParseInt(args[i+1], 10, 64)
					i++
				}
			case "--agent":
				if i+1 < len(args) {
					agentID = args[i+1]
					i++
				}
			}
		}
		if server == "" || topic == "" || agentID == "" {
			_, _ = fmt.Fprintln(errOut, "--server, --topic, and --agent are required")
			return 2
		}
		if err := ntfy.RunPublish(server, topic, token, nudgeID, agentID); err != nil {
			_, _ = fmt.Fprintf(errOut, "ntfy publish: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "ntfy wake published: nudge=%d agent=%s topic=%s\n", nudgeID, agentID, topic)
		return 0
	case "wake":
		var agentID string
		for i := 1; i < len(args); i++ {
			if args[i] == "--agent" && i+1 < len(args) {
				agentID = args[i+1]
				i++
			}
		}
		if agentID == "" {
			_, _ = fmt.Fprintln(errOut, "--agent is required")
			return 2
		}
		result, err := ntfy.Wake(context.Background(), baseURL, agentID)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "wake: %v\n", err)
			return 1
		}
		if result == nil || result.NudgeID == 0 {
			_, _ = fmt.Fprintln(out, "no pending nudges")
			return 0
		}
		_, _ = fmt.Fprintf(out, "wake: nudge=%d work=%v\n", result.NudgeID, result.WorkDone)
		return 0
	default:
		_, _ = fmt.Fprintf(errOut, "unknown ntfy command %q\n", args[0])
		return 2
	}
}

func agentPulse(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(errOut, "usage: tk agent <progress|status|sessions|nudges|ack> ...")
		return 2
	}
	switch args[0] {
	case "identity", "state", "states", "work", "wait", "idle", "stop":
		return agentActivityCommand(args, out, errOut, baseURL)
	case "claim", "release", "claims", "profile":
		return faseCommand(args, out, errOut, baseURL)
	case "status":
		return agentStatus(args[1:], out, errOut, baseURL)
	case "sessions":
		return agentSessions(args[1:], out, errOut, baseURL)
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
		response, err := newHTTPClient(10 * time.Second).Do(request)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
			return 1
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusCreated {
			reportHTTPStatus(errOut, "record agent progress", response)
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
		response, err := newHTTPClient(10 * time.Second).Get(endpoint)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
			return 1
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			reportHTTPStatus(errOut, "list agent nudges", response)
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
		response, err := newHTTPClient(10 * time.Second).Do(request)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
			return 1
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			reportHTTPStatus(errOut, "acknowledge agent nudge", response)
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

func agentsCommand(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(errOut, "usage: tk agents <list|broadcast> ...")
		return 2
	}
	switch args[0] {
	case "list":
		endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/agents"
		response, err := newHTTPClient(10 * time.Second).Get(endpoint)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
			return 1
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			reportHTTPStatus(errOut, "list agents", response)
			return 1
		}
		var payload struct {
			Items []struct {
				AgentID              string `json:"agent_id"`
				ActiveSprintID       string `json:"active_sprint_id"`
				LeaseDurationSeconds int64  `json:"lease_duration_seconds"`
				LastProgressAt       string `json:"last_progress_at"`
				GuardianURL          string `json:"guardian_url"`
			} `json:"items"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
			return 1
		}
		for _, a := range payload.Items {
			_, _ = fmt.Fprintf(out, "%s\tlease=%ds\tsprint=%s\tguardian=%s\tlast=%s\n", a.AgentID, a.LeaseDurationSeconds, a.ActiveSprintID, a.GuardianURL, a.LastProgressAt)
		}
		return 0
	case "broadcast":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			_, _ = fmt.Fprintln(errOut, "usage: tk agents broadcast <message>")
			return 2
		}
		message := args[1]
		// Prepend the countdown notice so agents know a restart is coming.
		fullMessage := fmt.Sprintf("%s — server will be restarted in 30 seconds", message)
		body, err := json.Marshal(map[string]string{"message": fullMessage})
		if err != nil {
			return 1
		}
		endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/agents/broadcast"
		request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return 1
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := newHTTPClient(10 * time.Second).Do(request)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
			return 1
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusCreated {
			reportHTTPStatus(errOut, "broadcast agent message", response)
			return 1
		}
		var result struct {
			AgentIDs []string `json:"agent_ids"`
			Count    int      `json:"count"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "broadcast to %d agents: %s\n", result.Count, strings.Join(result.AgentIDs, ", "))

		// Countdown from 30 on one line (\r overwrites the previous write).
		// Write directly to os.Stdout so the \r reaches the terminal even
		// when the CLI output is wrapped in a buffered writer.
		stdout := os.Stdout
		_, _ = fmt.Fprintf(stdout, "\rRestarting in %ds...          ", 30)
		for i := 30; i > 0; i-- {
			time.Sleep(time.Second)
			_, _ = fmt.Fprintf(stdout, "\rRestarting in %ds...          ", i)
		}
		_, _ = fmt.Fprintln(stdout, "\rRestarting now...                   ")

		// Execute the restart via the service manager.
		restartArgs := []string{"restart"}
		return serviceManager(restartArgs, out, errOut, baseURL)
	default:
		_, _ = fmt.Fprintf(errOut, "unknown agents command %q\n", args[0])
		return 2
	}
}

func guardianSelfNudge(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 || args[0] != "self-nudge" {
		_, _ = fmt.Fprintln(errOut, "usage: tk guardian self-nudge --agent-id <id> [--lease <duration>] [--project <id>] [--work <script>]")
		return 2
	}
	// Parse flags.
	agentID := ""
	lease := "30m"
	projectID := ""
	workScript := "scripts/guardian/self-nudge-work.sh"
	bindAddr := "127.0.0.1:1619"
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--agent-id":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(errOut, "--agent-id requires a value")
				return 2
			}
			agentID = args[i+1]
			i++
		case "--lease":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(errOut, "--lease requires a value")
				return 2
			}
			lease = args[i+1]
			i++
		case "--project":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(errOut, "--project requires a value")
				return 2
			}
			projectID = args[i+1]
			i++
		case "--work":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(errOut, "--work requires a value")
				return 2
			}
			workScript = args[i+1]
			i++
		case "--bind":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(errOut, "--bind requires a value")
				return 2
			}
			bindAddr = args[i+1]
			i++
		}
	}
	if agentID == "" {
		_, _ = fmt.Fprintln(errOut, "--agent-id is required")
		return 2
	}
	// Register the agent with the guardian.
	guardianURL := "http://" + bindAddr + "/pulse"
	leaseDur, err := time.ParseDuration(lease)
	if err != nil || leaseDur < time.Second {
		_, _ = fmt.Fprintln(errOut, "lease must be a positive duration such as 30m or 1h")
		return 2
	}
	regInput := map[string]any{"lease_duration_seconds": int64(leaseDur.Seconds()), "guardian_url": guardianURL}
	regBody, err := json.Marshal(regInput)
	if err != nil {
		return 1
	}
	regURL := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + agentID + "/progress"
	regReq, err := http.NewRequest(http.MethodPost, regURL, bytes.NewReader(regBody))
	if err != nil {
		return 1
	}
	regReq.Header.Set("Content-Type", "application/json")
	regResp, err := newHTTPClient(10 * time.Second).Do(regReq)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "register agent: %v\n", err)
		return 1
	}
	regResp.Body.Close()
	if regResp.StatusCode != http.StatusOK && regResp.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "register agent: %s\n", regResp.Status)
		return 1
	}
	_, _ = fmt.Fprintf(out, "registered agent %s with lease %s and guardian %s\n", agentID, lease, guardianURL)

	// Launch the self-nudge receiver as a subprocess.
	// Resolve the actual executable path rather than os.Args[0]. On WSL,
	// a Windows binary may be invoked as `tk` without its `.exe` suffix;
	// os.Args[0] then points at a relative command name and loses the
	// installed sibling receiver.
	selfPath, err := os.Executable()
	if err != nil || strings.TrimSpace(selfPath) == "" {
		selfPath = os.Args[0]
	}
	if absolute, absErr := filepath.Abs(selfPath); absErr == nil {
		selfPath = absolute
	}
	selfDir := filepath.Dir(selfPath)
	receiverName := "tk-guardian-self.exe"
	receiverCandidates := []string{
		filepath.Join(selfDir, receiverName),
		filepath.Join(filepath.Dir(os.Args[0]), receiverName),
		filepath.Join(".timekeeper", "app", "bin", receiverName),
	}
	receiverExe := ""
	for _, candidate := range receiverCandidates {
		if _, statErr := os.Stat(candidate); statErr == nil {
			receiverExe = candidate
			break
		}
	}
	if receiverExe == "" {
		// Fall back to running the receiver inline via the current binary.
		// This requires the receiver to be built into tk.exe — for now,
		// instruct the user to build it separately.
		_, _ = fmt.Fprintf(errOut, "tk-guardian-self.exe not found beside the installed CLI. Build it with: go build -o %s ./cmd/tk-guardian-self\n", receiverCandidates[0])
		return 1
	}

	// Start the receiver subprocess.
	cmd := exec.Command(receiverExe)
	cmd.Env = append(os.Environ(),
		"TK_AGENT_ID="+agentID,
		"TK_GUARDIAN_SELF_BIND="+bindAddr,
		"TK_GUARDIAN_SELF_WORK="+workScript,
		"TK_BASE_URL="+strings.TrimRight(baseURL, "/"),
	)
	if projectID != "" {
		cmd.Env = append(cmd.Env, "PROJECT_ID="+projectID)
	}
	cmd.Stdout = out
	cmd.Stderr = errOut
	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(errOut, "start receiver: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "self-nudge receiver started (pid %d)\n", cmd.Process.Pid)
	// Don't wait — the receiver runs until killed. The user can stop it
	// with Ctrl+C or by killing the process.
	return 0
}

func events(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(errOut, "usage: tk events <project-id> [--timestamps] [--since <duration>]")
		return 2
	}
	projectID := args[0]
	showTimestamps := false
	since := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--timestamps":
			showTimestamps = true
		case "--since":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(errOut, "--since requires a duration (e.g. 1h, 30m)")
				return 2
			}
			since = args[i+1]
			i++
		}
	}
	eventsURL := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/events"
	if since != "" {
		eventsURL += "?since=" + url.QueryEscape(since)
	}
	response, err := newHTTPClient(10 * time.Second).Get(eventsURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "TimeKeeper API unavailable at %s: %v\n", eventsURL, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "list project events", response)
		return 1
	}
	var payload struct {
		Items []struct {
			EventID   int64  `json:"event_id"`
			EventType string `json:"event_type"`
			EntityID  string `json:"entity_id"`
			Message   string `json:"message"`
			CreatedAt string `json:"created_at"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	for _, event := range payload.Items {
		if showTimestamps {
			_, _ = fmt.Fprintf(out, "%d	%s	%s	%s	%s\n", event.EventID, event.CreatedAt, event.EventType, event.EntityID, event.Message)
		} else {
			_, _ = fmt.Fprintf(out, "%d	%s	%s	%s\n", event.EventID, event.EventType, event.EntityID, event.Message)
		}
	}
	return 0
}

func notes(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk notes <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/notes"
	response, err := newHTTPClient(10 * time.Second).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "list project notes", response)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "record project note", response)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "update sprint hold reason", response)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "transition sprint", response)
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

	if action == "start" || action == "complete" {
		performAutoGitSync(action, sprint.SprintID, out)
	}

	return 0
}

func sprintEntries(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp entries <sprint-id>")
		return 2
	}
	response, err := newHTTPClient(10 * time.Second).Get(strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + args[1] + "/time-entries")
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "list sprint entries", response)
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
	response, err := newHTTPClient(10 * time.Second).Get(strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + args[1] + "/extensions")
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "list sprint extensions", response)
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
	duration, err := parseDuration(args[2])
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "record sprint extension", response)
		return 1
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		_, _ = fmt.Fprintf(errOut, "read Time Keeper API response: %v\n", readErr)
		return 1
	}
	var extension struct {
		ExtensionID     int64  `json:"extension_id"`
		SprintID        string `json:"sprint_id"`
		DurationSeconds int64  `json:"duration_seconds"`
		Reason          string `json:"reason"`
	}
	if err := json.Unmarshal(responseBody, &extension); err != nil {
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "record retrieval attempt", response)
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
	response, err := newHTTPClient(10 * time.Second).Get(endpoint)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", endpoint, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "list retrieval attempts", response)
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

func sprintClaimNew(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 5 && len(args) != 6 {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp claim-new <project-id> <task-id> <owner-id> <sprint-name> <estimate> [buffer-percent]")
		return 2
	}
	projectID, taskID, _, name := args[0], args[1], args[2], args[3]
	estimate, err := parseDuration(args[4])
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
	createBody, err := json.Marshal(map[string]any{"name": name, "estimated_duration_seconds": int64(estimate.Seconds()), "buffer_pct": bufferPct})
	if err != nil {
		return 1
	}
	createURL := strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + url.PathEscape(taskID) + "/sprints"
	req, err := http.NewRequest(http.MethodPost, createURL, bytes.NewReader(createBody))
	if err != nil {
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := newHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create sprint: %v\n", err)
		return 1
	}
	if resp.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "create sprint", resp)
		return 1
	}
	resp.Body.Close()
	// Now claim the next sprint for the project.
	claimURL := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + url.PathEscape(projectID) + "/sprints/claim-next"
	claimReq, err := http.NewRequest(http.MethodPost, claimURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return 1
	}
	claimReq.Header.Set("Content-Type", "application/json")
	claimResp, err := newHTTPClient(10 * time.Second).Do(claimReq)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "claim sprint: %v\n", err)
		return 1
	}
	defer claimResp.Body.Close()
	if claimResp.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "claim sprint", claimResp)
		return 1
	}
	var sprint model.Sprint
	if err := json.NewDecoder(claimResp.Body).Decode(&sprint); err != nil {
		_, _ = fmt.Fprintf(errOut, "claim sprint: invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "created and claimed %s\t%s\t%s\n", sprint.SprintID, sprint.Name, sprint.Status)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "claim next sprint", response)
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
	if len(args) > 0 && args[0] == "claim-new" {
		return sprintClaimNew(args[1:], out, errOut, baseURL)
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
	estimate, err := parseDuration(args[4])
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
	submitPath := "/api/v1/" + ownerPath + "/" + args[2] + "/sprints"
	_, state, code := submitSingleRegistrationOffer("", "sprint", submitPath, "create sprint", body, baseURL, out, errOut)
	if code != 0 || state.Status != registrationOutboxRegistered {
		return code
	}
	var sprint struct {
		SprintID    string `json:"sprint_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(state.Result, &sprint); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", sprint.SprintID, sprint.Status, sprint.Name, sprint.ItemAddress)
	return 0
}

func subtask(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) > 0 && args[0] == "show" {
		return subtaskShow(args[1:], out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "status" {
		return subtaskStatus(args[1], args[2], out, errOut, baseURL)
	}
	if len(args) != 4 || args[0] != "new" || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk st new <task-id> <subtask-name> <estimate>")
		return 2
	}
	estimate, err := parseDuration(args[3])
	if err != nil || estimate <= 0 {
		_, _ = fmt.Fprintln(errOut, "estimate must be a positive duration such as 30m or 2h")
		return 2
	}
	body, err := json.Marshal(map[string]any{"name": args[2], "estimated_duration_seconds": int64(estimate.Seconds())})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode subtask input: %v\n", err)
		return 1
	}
	submitPath := "/api/v1/tasks/" + args[1] + "/subtasks"
	_, state, code := submitSingleRegistrationOffer("", "subtask", submitPath, "create subtask", body, baseURL, out, errOut)
	if code != 0 || state.Status != registrationOutboxRegistered {
		return code
	}
	var subtask struct {
		SubtaskID   string `json:"subtask_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.Unmarshal(state.Result, &subtask); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", subtask.SubtaskID, subtask.Name, subtask.ItemAddress)
	return 0
}

func task(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) > 0 && args[0] == "show" {
		return taskShow(args[1:], out, errOut, baseURL)
	}
	if len(args) >= 2 && args[0] == "edit" && strings.TrimSpace(args[1]) != "" {
		return taskEdit(args[1:], out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "status" {
		return taskStatus(args[1], args[2], out, errOut, baseURL)
	}
	if len(args) >= 3 && args[0] == "batch" && strings.TrimSpace(args[1]) != "" && strings.TrimSpace(args[2]) != "" {
		return taskBatch(args[1], args[2:], out, errOut, baseURL)
	}
	noAutoClose := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--no-autoclose" {
			noAutoClose = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) != 5 || filtered[0] != "new" || strings.TrimSpace(filtered[1]) == "" || strings.TrimSpace(filtered[2]) == "" || strings.TrimSpace(filtered[3]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk t new <project-id> <category-id> <task-name> <estimate> [--no-autoclose]")
		return 2
	}
	estimate, err := parseDuration(filtered[4])
	if err != nil || estimate <= 0 {
		_, _ = fmt.Fprintln(errOut, "estimate must be a positive duration such as 30m or 2h")
		return 2
	}
	payload := map[string]any{
		"category_id":                filtered[2],
		"name":                       filtered[3],
		"estimated_duration_seconds": int64(estimate.Seconds()),
	}
	if noAutoClose {
		payload["auto_close"] = false
	}
	body, err := json.Marshal(payload)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode task input: %v\n", err)
		return 1
	}
	submitPath := "/api/v1/projects/" + filtered[1] + "/tasks"
	_, state, code := submitSingleRegistrationOffer(filtered[1], "task", submitPath, "create task", body, baseURL, out, errOut)
	if code != 0 || state.Status != registrationOutboxRegistered {
		return code
	}
	var task struct {
		TaskID      string `json:"task_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.Unmarshal(state.Result, &task); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	if jsonOutputEnabled {
		writeJSONResponse(out, task)
		return 0
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", task.TaskID, task.Name, task.ItemAddress)
	return 0
}

func taskBatch(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	return runTaskBatch(projectID, args, out, errOut, baseURL)
}

func subtaskStatus(subtaskID, status string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"status": status})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/subtasks/" + subtaskID + "/status"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "set subtask status", response)
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

func taskEdit(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 1 {
		_, _ = fmt.Fprintln(errOut, "usage: tk t edit <task-id> <goal> <description> OR tk t edit <task-id> [--goal G] [--description D] [--description-file F]")
		return 2
	}
	taskID := args[0]
	rest := args[1:]

	var goal *string
	var description *string
	usePatch := false

	if len(rest) == 2 && !strings.HasPrefix(rest[0], "--") {
		// Legacy positional syntax
		g := rest[0]
		d := rest[1]
		goal = &g
		description = &d
	} else {
		usePatch = true
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--goal":
				if i+1 < len(rest) {
					g := rest[i+1]
					goal = &g
					i++
				}
			case "--description":
				if i+1 < len(rest) {
					d := rest[i+1]
					description = &d
					i++
				}
			case "--description-file":
				if i+1 < len(rest) {
					filePath := rest[i+1]
					content, err := os.ReadFile(filePath)
					if err != nil {
						_, _ = fmt.Fprintf(errOut, "read description file %q: %v\n", filePath, err)
						return 1
					}
					d := string(content)
					description = &d
					i++
				}
			}
		}
	}

	if goal == nil && description == nil {
		_, _ = fmt.Fprintln(errOut, "usage: tk t edit <task-id> <goal> <description> OR tk t edit <task-id> [--goal G] [--description D] [--description-file F]")
		return 2
	}

	var method string
	var url string
	var body []byte

	if usePatch {
		method = http.MethodPatch
		url = strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + taskID + "/metadata"
		payload := map[string]*string{}
		if goal != nil {
			payload["goal"] = goal
		}
		if description != nil {
			payload["description"] = description
		}
		body, _ = json.Marshal(payload)
	} else {
		method = http.MethodPost
		url = strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + taskID + "/metadata"
		gVal, dVal := "", ""
		if goal != nil {
			gVal = *goal
		}
		if description != nil {
			dVal = *description
		}
		body, _ = json.Marshal(map[string]string{"goal": gVal, "description": dVal})
	}

	request, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "update task context", response)
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
	if jsonOutputEnabled {
		writeJSONResponse(out, task)
		return 0
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "set task status", response)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "update category context", response)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "set category status", response)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "create category", response)
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
	if len(args) > 0 && (args[0] == "schedule" || args[0] == "phase" || args[0] == "milestones" || args[0] == "deadlines") {
		return projectLifecycleCommand(args, out, errOut, baseURL)
	}
	if len(args) == 4 && args[0] == "edit" && strings.TrimSpace(args[1]) != "" {
		return projectEdit(args[1], args[2], args[3], out, errOut, baseURL)
	}
	if len(args) >= 3 && args[0] == "status" && strings.TrimSpace(args[1]) != "" && strings.TrimSpace(args[2]) != "" {
		return projectStatus(args[1], args[2], strings.TrimSpace(strings.Join(args[3:], " ")), out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "alias" && strings.TrimSpace(args[1]) != "" {
		return projectAlias(args[1:], out, errOut, baseURL)
	}
	if len(args) == 2 && args[0] == "unalias" && strings.TrimSpace(args[1]) != "" {
		return projectUnalias(args[1:], out, errOut, baseURL)
	}
	if len(args) >= 2 && args[0] == "import" && strings.TrimSpace(args[1]) != "" {
		return importProject(args[1:], out, errOut, baseURL)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "create project", response)
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

func projectStatus(projectID, status, reason string, out, errOut io.Writer, baseURL string) int {
	body, err := json.Marshal(map[string]string{"status": status, "reason": reason})
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "set project status", response)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "update project context", response)
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
	client := newHTTPClient(10 * time.Second)
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects"
	response, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("Time Keeper API unavailable at %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolve project: %s", readHTTPErrorDetail(response))
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "set project alias", response)
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
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "clear project alias", response)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s	\n", resolvedID)
	return 0
}

func listAliases(out, errOut io.Writer, baseURL string) int {
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects"
	client := newHTTPClient(10 * time.Second)
	response, err := client.Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "list project aliases", response)
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
	}
	if strings.TrimRight(baseURL, "/") == defaultBaseURL {
		for _, u := range connectionURLs(baseURL) {
			urls = append(urls, strings.TrimRight(u, "/")+"/health")
		}
	}
	client := newHTTPClient(5 * time.Second)
	allOK := true
	canonicalOK := false
	for _, u := range urls {
		ok, status, err := probeHealth(client, u)
		if err != nil {
			// A connection error means the configured endpoint is not
			// resolvable or the friendly proxy is not listening.
			_, _ = fmt.Fprintf(errOut, "[failed] API unreachable: %s (%v)\n", u, err)
			allOK = false
			continue
		}
		if !ok {
			_, _ = fmt.Fprintf(errOut, "[failed] %s: HTTP %s\n", u, status)
			allOK = false
			continue
		}
		if u == urls[0] {
			_, _ = fmt.Fprintf(out, "[ok] API reachable: %s\n", strings.TrimRight(baseURL, "/"))
			_, _ = fmt.Fprintf(out, "[ok] health status: %s\n", status)
			canonicalOK = true
		} else {
			_, _ = fmt.Fprintf(out, "[ok] %s: %s\n", u, status)
		}
	}

	// Installation sanity check: the install-commit / install-path that
	// the running binary was built from. This catches the case where
	// the user updated their local checkout but the service / launcher
	// is still running the old binary.
	if err := reportInstallInfo(out); err != nil {
		_, _ = fmt.Fprintf(out, "[skip] install info: %v\n", err)
	}

	// Capability visibility: report optional subsystems that have routes but
	// are unconfigured or unsupported, and say whether the client and server
	// builds agree. An optional capability being unavailable must not read as
	// a core failure.
	unavailableCapabilities := reportCapabilities(out, baseURL)

	if allOK {
		if unavailableCapabilities > 0 {
			noun := "capability is"
			if unavailableCapabilities != 1 {
				noun = "capabilities are"
			}
			_, _ = fmt.Fprintf(out, "\nTime Keeper is ready. Core is healthy; %d optional %s unavailable.\n",
				unavailableCapabilities, noun)
			_, _ = fmt.Fprintln(out, "Optional capabilities are not required for ordinary planning, sprint and memory work.")
			return 0
		}
		_, _ = fmt.Fprintln(out, "\nTime Keeper is ready.")
		return 0
	}
	if canonicalOK {
		_, _ = fmt.Fprintln(errOut, "\nTime Keeper is reachable on the canonical address, but the friendly URLs are not.")
		_, _ = fmt.Fprintln(errOut, "Run `tk hosts status`; add missing entries with `tk hosts add` from an Administrator shell. Friendly URLs use the running server port when port 80 is unavailable.")
		return 1
	}
	_, _ = fmt.Fprintln(errOut, "\nTime Keeper is not fully ready.")
	_, _ = fmt.Fprintln(errOut, "Start Time Keeper, then run tk doctor again.")
	return 1
}

// capability describes one probed endpoint. Required capabilities gate
// readiness; optional ones must never make a healthy core look unhealthy.
type capability struct {
	name     string
	path     string
	required bool
	hint     string
}

// optionalCapabilities are the subsystems that ship routes but may be
// unconfigured or absent on a given server build. 503 means "present but not
// configured"; 404 means "this build does not implement it". Those are
// different states and must not be reported as the same readiness level.
var optionalCapabilities = []capability{
	{name: "plugin store", path: "/api/v1/plugins", hint: "set TIMEKEEPER_STORE_URL"},
	{name: "history", path: "/api/v1/history/status", hint: "rebuild with NewWithHistory or set HISTORY_ROOT"},
}

// reportCapabilities probes optional subsystems and prints one classified line
// each, followed by the client/server version and whether the pair is skewed.
// It returns how many optional capabilities are unavailable.
func reportCapabilities(out io.Writer, baseURL string) int {
	unavailable := 0
	for _, cap := range optionalCapabilities {
		code, _ := probeStatusCode(baseURL + cap.path)
		switch {
		case code == http.StatusOK:
			_, _ = fmt.Fprintf(out, "[ok] %s: optional, available\n", cap.name)
		case code == http.StatusServiceUnavailable:
			_, _ = fmt.Fprintf(out, "[hint] %s: optional, unconfigured (%s)\n", cap.name, cap.hint)
			unavailable++
		case code == http.StatusNotFound:
			_, _ = fmt.Fprintf(out, "[hint] %s: optional, unsupported by this server build\n", cap.name)
			unavailable++
		case code == 0:
			_, _ = fmt.Fprintf(out, "[hint] %s: optional, probe failed\n", cap.name)
			unavailable++
		default:
			_, _ = fmt.Fprintf(out, "[hint] %s: optional, unexpected HTTP %d\n", cap.name, code)
			unavailable++
		}
	}
	reportVersions(out, baseURL)
	return unavailable
}

// reportVersions prints client and server build identity side by side and says
// explicitly whether a mismatch matters, because a skewed pair is the usual
// cause of "the CLI rejects a flag the source supports".
func reportVersions(out io.Writer, baseURL string) {
	client := version.Get()
	_, _ = fmt.Fprintf(out, "[info] client version: %s\n", version.String())

	code, body := probeBody(baseURL + "/api/v1/version")
	if code != http.StatusOK || body == "" {
		_, _ = fmt.Fprintln(out, "[info] server version: unknown (could not read /api/v1/version)")
		return
	}
	var v struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if json.Unmarshal([]byte(body), &v) != nil {
		_, _ = fmt.Fprintln(out, "[info] server version: unknown (unreadable response)")
		return
	}
	_, _ = fmt.Fprintf(out, "[info] server version: %s (%s)\n", v.Version, v.Commit)

	serverCommit := strings.TrimSpace(v.Commit)
	clientCommit := strings.TrimSpace(client.Commit)
	if serverCommit == "" || clientCommit == "" {
		return
	}
	if strings.HasPrefix(serverCommit, clientCommit) || strings.HasPrefix(clientCommit, serverCommit) {
		_, _ = fmt.Fprintln(out, "[info] version match: client and server are built from the same commit")
		return
	}
	_, _ = fmt.Fprintln(out, "[hint] version skew: client and server are built from different commits")
	_, _ = fmt.Fprintln(out, "       The server decides what the API accepts. Reinstall to realign the CLI with the running service.")
}

// probeStatusCode fetches a URL and returns the HTTP status code, or 0 on error.
func probeStatusCode(url string) (int, string) {
	resp, err := newHTTPClient(2 * time.Second).Get(url)
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()
	return resp.StatusCode, ""
}

// probeBody fetches a URL and returns the status code and body.
func probeBody(url string) (int, string) {
	resp, err := newHTTPClient(2 * time.Second).Get(url)
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
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
	client := newHTTPClient(10 * time.Second)
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
	usage := "usage: tk tree <project-id> [--task <T-ID>] [--status <Task-status>] [--active] [--depth N] [--limit N] [--json]"
	projectID, taskID, statusFilter := "", "", ""
	limit, maxDepth := 0, 0
	activeOnly := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			_, _ = fmt.Fprintln(out, usage)
			return 0
		case "--active":
			activeOnly = true
		case "--task", "--status", "--depth", "--limit":
			flag := args[i]
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				_, _ = fmt.Fprintf(errOut, "%s requires a value\n", flag)
				return 2
			}
			i++
			switch flag {
			case "--task":
				taskID = strings.TrimSpace(args[i])
			case "--status":
				statusFilter = strings.TrimSpace(args[i])
			case "--depth":
				parsed, err := strconv.Atoi(args[i])
				if err != nil || parsed < 1 || parsed > 10 {
					_, _ = fmt.Fprintln(errOut, "--depth must be between 1 and 10")
					return 2
				}
				maxDepth = parsed
			case "--limit":
				parsed, err := strconv.Atoi(args[i])
				if err != nil || parsed < 1 || parsed > 200 {
					_, _ = fmt.Fprintln(errOut, "--limit must be between 1 and 200")
					return 2
				}
				limit = parsed
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				_, _ = fmt.Fprintf(errOut, "unknown tree option %q\n", args[i])
				return 2
			}
			if projectID != "" {
				_, _ = fmt.Fprintln(errOut, usage)
				return 2
			}
			projectID = strings.TrimSpace(args[i])
		}
	}
	if projectID == "" {
		_, _ = fmt.Fprintln(errOut, usage)
		return 2
	}
	query := url.Values{}
	if taskID != "" {
		query.Set("task_id", taskID)
	}
	if statusFilter != "" {
		query.Set("status", statusFilter)
	}
	if activeOnly {
		query.Set("active", "true")
	}
	if maxDepth > 0 {
		query.Set("depth", strconv.Itoa(maxDepth))
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	treeURL := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/execution-tree"
	if encoded := query.Encode(); encoded != "" {
		treeURL += "?" + encoded
	}
	client := newHTTPClient(10 * time.Second)
	response, err := client.Get(treeURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", treeURL, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "read project tree", response)
		return 1
	}
	var payload model.ExecutionTree
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	if taskID != "" || statusFilter != "" {
		filtered, found := filterExecutionTree(payload, taskID, statusFilter)
		if !found {
			if taskID != "" && statusFilter == "" {
				_, _ = fmt.Fprintf(errOut, "Task %s was not found in Project %s.\n", taskID, projectID)
			} else {
				_, _ = fmt.Fprintf(errOut, "No Task matched tree filters in Project %s.\n", projectID)
			}
			return 1
		}
		payload = filtered
	}
	if activeOnly {
		filtered, found := filterExecutionTreeToActiveSprints(payload)
		if !found {
			_, _ = fmt.Fprintf(errOut, "No Active Sprints matched tree filters in Project %s.\n", projectID)
			return 1
		}
		payload = filtered
	}
	if maxDepth > 0 {
		payload = limitExecutionTreeDepth(payload, maxDepth)
	}
	totalTasks, shownTasks, truncated := 0, 0, false
	if limit > 0 {
		totalTasks = countExecutionTasks(payload)
		payload, shownTasks, truncated = limitExecutionTreeTasks(payload, limit)
		if serverTotal, serverShown, serverTruncated, ok := executionTreeLimitMetadata(response.Header, limit); ok {
			totalTasks, shownTasks, truncated = serverTotal, serverShown, serverTruncated
		}
	}
	if jsonOutputEnabled {
		if limit > 0 {
			writeJSONResponse(out, map[string]any{
				"tree": payload, "filters": map[string]any{"task_id": taskID, "status": statusFilter, "active": activeOnly, "depth": maxDepth},
				"limit": limit, "total_tasks": totalTasks, "shown_tasks": shownTasks, "truncated": truncated,
			})
		} else {
			writeJSONResponse(out, payload)
		}
		return 0
	}
	_, _ = fmt.Fprintf(out, "%s	%.1f%%	%s\n", payload.Project.ProjectID, payload.Project.CalculatedCompletionPct, payload.Project.ProjectName)
	for _, category := range payload.Categories {
		writeCategoryTree(out, category, 1)
	}
	if truncated {
		_, _ = fmt.Fprintf(out, "... truncated: showing first %d of %d Task matches (limit %d)\n", shownTasks, totalTasks, limit)
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
	client := newHTTPClient(10 * time.Second)
	response, err := client.Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "list projects", response)
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
	if jsonOutputEnabled {
		writeJSONResponse(out, payload)
		return 0
	}
	for _, project := range payload.Items {
		_, _ = fmt.Fprintf(out, "%s	%s	%.1f%%	%s	%s\n", project.ProjectID, project.Status, project.CalculatedCompletionPct, project.ProjectName, project.ItemAddress)
	}
	return 0
}
