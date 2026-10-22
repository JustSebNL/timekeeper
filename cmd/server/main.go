// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/harness"
	"github.com/JustSebNL/timekeeper/internal/history"
	"github.com/JustSebNL/timekeeper/internal/instanceguard"
	"github.com/JustSebNL/timekeeper/internal/knowledge"
	"github.com/JustSebNL/timekeeper/internal/logging"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/observer"
	"github.com/JustSebNL/timekeeper/internal/plugin"
	edgeplugin "github.com/JustSebNL/timekeeper/internal/plugin/edgeplugin"
	obsidianplugin "github.com/JustSebNL/timekeeper/internal/plugin/knowledgeobsidian"
	"github.com/JustSebNL/timekeeper/internal/shadowcopy"
	"github.com/JustSebNL/timekeeper/internal/store"
	"github.com/JustSebNL/timekeeper/internal/version"
)

// modeFlag is a tiny adapter so we can use logging.Mode by value with flag.Var.
type modeFlag struct{ v *logging.Mode }

func (f modeFlag) String() string {
	if f.v == nil {
		return ""
	}
	return f.v.String()
}
func (f modeFlag) Set(s string) error { return f.v.Set(s) }

func recoveryPolicy() string {
	// The repo-local recovery receiver performs exactly one allowlisted action:
	// write a durable local recovery artifact and acknowledge the nudge. It
	// performs no process control. This string is status evidence, not a grant.
	return "local-artifact: durable recovery marker only; no process control"
}

func newGuardianRecoveryHandler(database *store.Store, stateDir string) (http.Handler, error) {
	stateDir = strings.TrimSpace(stateDir)
	if stateDir == "" {
		return nil, nil
	}
	absolute, err := filepath.Abs(stateDir)
	if err != nil {
		return nil, fmt.Errorf("resolve Guardian recovery state directory: %w", err)
	}
	return guardian.NewRecoveryHandler(guardian.RecoveryHandlerConfig{
		StateDir:    absolute,
		Action:      guardian.RecoveryActionLocalArtifact,
		PolicyLabel: recoveryPolicy(),
		Acknowledge: func(ctx context.Context, nudge model.PulseNudge) error {
			_, err := database.AcknowledgePulseNudge(ctx, nudge.NudgeID, nudge.AgentID)
			return err
		},
	})
}

func validateServerConfig(addr, uiPath string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("listen address %q must include host and port: %w", addr, err)
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber > 65535 {
		return fmt.Errorf("listen address %q has an invalid port", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen address %q must use a numeric loopback host; Time Keeper is local-first", addr)
	}
	absolute, err := filepath.Abs(strings.TrimSpace(uiPath))
	if err != nil || strings.TrimSpace(uiPath) == "" {
		return fmt.Errorf("dashboard path must be a valid file path")
	}
	for _, candidate := range []string{absolute, filepath.Join(filepath.Dir(absolute), "timekeeper.css"), filepath.Join(filepath.Dir(absolute), "timekeeper.js")} {
		info, err := os.Stat(candidate)
		if err != nil {
			return fmt.Errorf("dashboard asset unavailable: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("dashboard asset %q is not a regular file", candidate)
		}
		if info.Mode().Perm()&0o444 == 0 {
			return fmt.Errorf("dashboard asset %q is not readable", candidate)
		}
		file, err := os.Open(candidate)
		if err != nil {
			return fmt.Errorf("dashboard asset unavailable: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close dashboard validation file: %w", err)
		}
	}
	return nil
}

func showHelp() {
	_, _ = fmt.Print(`usage: timekeeper [-v | --version] [-h | --help]
                  <command> [<args>]

Time Keeper is a local project execution memory. These commands manage
projects, categories, tasks, sprints, and planning drafts.

View state (read-only)
  list          List projects
  tree <id>     Show an executable hierarchy
  export <id>   Print a portable project snapshot as JSON
  summary <id>  Show a durable sprint operational snapshot
  pulse         Show local sprint attention needing follow-up
  events <id>   List immutable project activity
  notes <id>    List project notes
  aliases       List project aliases
  doctor        Check whether Time Keeper is reachable
  api-help      List all available API routes

Projects
  p new <name>                             Create a project
  p edit <id> <goal> <description>         Update project context
  p status <id> <status>                   Set project status (Open/Completed/Cancelled)
  p alias <id> <alias>                     Set project alias
  p unalias <id>                           Clear project alias

Categories
  c new <proj> <name> [parent-id]          Create a category
  c edit <id> <goal> <description>         Update category context
  c status <id> <status>                   Set category status

Tasks
  t new <proj> <cat> <name> <estimate>     Create a task (estimate: 30m, 2h)
  t edit <id> <goal> <description>         Update task context
  t status <id> <status>                   Set task status (Open/Completed/Cancelled)

Subtasks
  st new <task> <name> <estimate>          Create a subtask
  st status <id> <status>                  Set subtask status

Sprints
  sp new <task|subtask> <owner> <name> <estimate> [buffer]
                                           Create a sprint
  sp start <id>                            Start a sprint
  sp hold <id> <reason>                    Hold a sprint
  sp resume <id>                           Resume a sprint
  sp complete <id>                         Complete a sprint
  sp cancel <id> <reason>                  Cancel a sprint
  sp reason <id> <reason>                  Update hold reason
  sp next <proj>                           Claim the oldest runnable sprint
  sp attempts <id>                         List retrieval-attempt evidence
  sp attempt <id> <reason>                 Record a failed retrieval attempt
  sp extend <id> <duration> <reason>       Add justified planned time
  sp extensions <id>                       List extension history
  sp entries <id>                          List work/hold intervals

Notes
  note <id> <content>                      Record a project note

Planning drafts (LLM)
  plan generate <proj> <pipeline-id>       Generate a planning draft via LLM
  plan apply <proj> <draft-id>             Apply a reviewed planning draft
  plan list <proj>                         List planning drafts
  llm new <name> <provider> <url> <model>  Register a loopback LLM pipeline

Agent / Guardian
  agent progress <id> <lease> [sprint] [url]  Renew agent progress lease
  agent nudges <id>                           List pending Guardian nudges
  agent history <id>                          List Guardian delivery/recovery history
  agent ack <id> <nudge-id>                   Acknowledge a Guardian nudge

Server (start the HTTP service)
  timekeeper -addr 127.0.0.1:1618 -db .timekeeper/timekeeper.db -ui web/index.html -keep-alive-interval 5m
  -shadow-copy-dir .timekeeper/recovery/sqlite-shadow     A/B verified SQLite shadow location
  -shadow-copy-interval 5m                             Local cadence; 0 disables (minimum 1m)
  -restore-shadow-slot shadow-a.db -confirm-shadow-restore
                                                       Offline restore; requires Core stopped, preserves DB/WAL/SHM
  -db <standalone.db> -offline-batch-file <batch> -offline-batch-project <P-ID> -offline-batch-copy-to <new.db>
                                                       Offline Core smoke; default is check-only, no HTTP listener
  -offline-batch-apply                                 Apply only to the new copy, after successful Core preflight

Offline batch refuses an active Core lock, SQLite sidecars, corrupt sources, and existing output files.
It never writes the source database; output-copy promotion is a separate explicit operation.
  -obsidian-descriptor <path>                          Optional authenticated Galaxy endpoint descriptor
  -history-root <path>                                  Optional tracked-file history root

Each shadow run verifies the primary before copying and checks the candidate before A/B promotion.

Run 'tk <command> --help' for details on a specific command.

First use for agents
  1. Check health: tk doctor
  2. If unavailable, follow the failure-only procedure in harness/SKILL.md.
  3. Discover work: tk list, then tk tree <project-id> and tk summary <project-id>
  4. Work in the hierarchy Project -> Category -> Task -> optional Subtask -> Sprint.
  5. Use bounded 15–45 minute Sprints; record decisions, blockers, and handoffs.
  6. Verify with a test, build, reproduction, or other explicit evidence.
  7. Complete only after reading the result back. Resume from tree, summary,
     events, and Trail of Thought checkpoint M-<id> after interruption.

Full agent guide: docs/HARNESS.md
Usage harness contract: harness/SKILL.md
The harness performs the failure-only health gate and points agents to the
project-local Skills under harness/skills/. Never store credentials, private
prompts, or unbounded logs in Time Keeper.
`)
}

// envOr returns the value of the named environment variable, or fallback
// if it is empty.
func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

// friendlyURLs returns the human-facing URLs the server should advertise
// when the proxy listener is enabled. They are sugar for the loopback
// address; the canonical port stays at *addr.
func friendlyURLs(addr string) []string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}
	suffix := ""
	if port != "80" {
		suffix = ":" + port
	}
	return []string{"http://timekeeper.local" + suffix + "/", "http://api.timekeeper.local" + suffix + "/", "http://guardian.local" + suffix + "/"}
}

func main() {
	exitCode := 0
	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	if len(os.Args) == 1 || os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h" {
		showHelp()
		return
	}
	if os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v" {
		_, _ = fmt.Println("TimeKeeper " + version.String())
		return
	}
	if handled, code := dispatchClient(os.Args[1:], os.Getenv("TIMEKEEPER_URL"), os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	addr := flag.String("addr", "timekeeper.local", "HTTP listen address")
	proxyAddr := flag.String("proxy-addr", envOr("TIMEKEEPER_PROXY_ADDR", "127.0.0.1:80"), "reverse-proxy listen address for the friendly URLs (http://timekeeper.local/ and http://api.timekeeper.local/); empty to disable")
	obsidianDescriptor := flag.String("obsidian-descriptor", envOr("TIMEKEEPER_OBSIDIAN_DESCRIPTOR", ""), "optional Obsidian Memory Galaxy endpoint descriptor path")
	edgeJournal := flag.String("edge-journal", envOr("TIMEKEEPER_EDGE_JOURNAL", ""), "optional edge event journal path; enables the observer plugin")
	historyRoot := flag.String("history-root", envOr("TIMEKEEPER_HISTORY_ROOT", ""), "optional tracked-file history root; empty disables file history routes")
	pluginStoreRoot := flag.String("plugin-store-root", envOr("TIMEKEEPER_PLUGIN_STORE_ROOT", ""), "optional plugin-store directory; empty disables plugin registry routes")
	dbPath := flag.String("db", "timekeeper.db", "SQLite database path")
	uiPath := flag.String("ui", ".timekeeper/web/index.html", "dashboard HTML path")
	keepAliveInterval := flag.Duration("keep-alive-interval", 5*time.Minute, "local dashboard keep-alive interval; 0 disables it")
	backupTo := flag.String("backup-to", "", "create a SQLite backup at this new path, then exit")
	offlineBatchFile := flag.String("offline-batch-file", "", "parse and Core-validate a batch against a new offline database copy")
	offlineBatchProject := flag.String("offline-batch-project", "", "target Project ID for offline batch validation/application")
	offlineBatchCopyTo := flag.String("offline-batch-copy-to", "", "new isolated database-copy path; source database is never modified")
	offlineBatchFormat := flag.String("offline-batch-format", "", "batch format override; empty auto-detects from the file extension")
	offlineBatchApply := flag.Bool("offline-batch-apply", false, "apply only to the isolated output copy after successful Core preflight; default is smoke-only")
	shadowCopyDir := flag.String("shadow-copy-dir", envOr("TIMEKEEPER_SHADOW_COPY_DIR", ""), "A/B SQLite shadow directory; defaults to recovery beside the primary database")
	shadowCopyInterval := flag.Duration("shadow-copy-interval", 5*time.Minute, "verified local shadow cadence; 0 disables, minimum 1m")
	restoreShadowSlot := flag.String("restore-shadow-slot", "", "offline restore from the explicit shadow-a.db or shadow-b.db slot")
	confirmShadowRestore := flag.Bool("confirm-shadow-restore", false, "required confirmation for offline shadow restore")
	pulseGuardianInterval := flag.Duration("pulse-guardian-interval", 5*time.Minute, "run the local Pulse Guardian at this interval; 0 disables it")
	pulseGuardianNtfyServer := flag.String("pulse-guardian-ntfy-server", "", "ntfy server URL for guardian wake-up (e.g. https://ntfy.sh); empty disables ntfy publishing")
	pulseGuardianNtfyTopic := flag.String("pulse-guardian-ntfy-topic", "", "ntfy topic for guardian wake-up messages")
	pulseGuardianNtfyToken := flag.String("pulse-guardian-ntfy-token", "", "ntfy authentication token (optional)")
	guardianStateDir := flag.String("guardian-state-dir", envOr("TIMEKEEPER_GUARDIAN_STATE_DIR", ""), "existing private Guardian recovery artifact directory; enables POST /api/v1/guardian/recover")
	shutdownOnStdinClose := flag.Bool("shutdown-on-stdin-close", false, "gracefully stop this server when its parent closes stdin")
	logPath := flag.String("log", "/data/logs/app.log", "structured JSON log path; empty disables file logging")
	logMode := logging.ModeNormal
	flag.Var(modeFlag{&logMode}, "log-mode", "log mode: normal|debug (debug logs every step, raw inputs, full errors)")
	flag.Parse()

	// Ensure addr has a port; default to 1618 if hostname only
	if !strings.Contains(*addr, ":") {
		*addr = *addr + ":1618"
	}

	offlineBatchRequested := strings.TrimSpace(*offlineBatchFile) != "" || strings.TrimSpace(*offlineBatchProject) != "" || strings.TrimSpace(*offlineBatchCopyTo) != "" || strings.TrimSpace(*offlineBatchFormat) != "" || *offlineBatchApply
	if offlineBatchRequested {
		databaseFlagSet := false
		flag.Visit(func(parsed *flag.Flag) {
			if parsed.Name == "db" {
				databaseFlagSet = true
			}
		})
		if !databaseFlagSet {
			fmt.Fprintln(os.Stderr, "offline batch requires an explicit -db standalone source database")
			exitCode = 2
			return
		}
		if strings.TrimSpace(*offlineBatchFile) == "" || strings.TrimSpace(*offlineBatchProject) == "" || strings.TrimSpace(*offlineBatchCopyTo) == "" {
			fmt.Fprintln(os.Stderr, "offline batch requires -offline-batch-file, -offline-batch-project, and -offline-batch-copy-to")
			exitCode = 2
			return
		}
		if *backupTo != "" || *restoreShadowSlot != "" || *confirmShadowRestore {
			fmt.Fprintln(os.Stderr, "offline batch cannot be combined with backup or shadow-restore maintenance modes")
			exitCode = 2
			return
		}
		report, err := runOfflineBatchCopy(context.Background(), *dbPath, *offlineBatchCopyTo, *offlineBatchProject, *offlineBatchFile, *offlineBatchFormat, !*offlineBatchApply)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Offline batch failed: %v\n", err)
			if report.CopyDB != "" {
				fmt.Fprintf(os.Stderr, "Source was not intentionally modified; isolated copy retained at: %s\n", report.CopyDB)
			}
			exitCode = 1
			return
		}
		counts := report.Validation.Counts
		mode := "smoke passed; no batch objects created"
		if *offlineBatchApply {
			counts = report.Created.Counts
			mode = "applied to isolated copy after successful smoke"
		}
		fmt.Printf("Offline batch %s: Tasks=%d Subtasks=%d Sprints=%d Total=%d\n", mode, counts.Tasks, counts.Subtasks, counts.Sprints, counts.TotalObjects)
		fmt.Printf("Source DB: %s\nSource SHA-256: %s\nIsolated output copy: %s\nCopy-before-batch SHA-256: %s\nHTTP service started: no\n", report.SourceDB, report.SourceSHA256, report.CopyDB, report.CopyBeforeBatchHash)
		return
	}

	logger := logging.Init(logging.Config{Path: *logPath, Mode: logMode, MaxSizeMiB: 10, MaxBackups: 5})
	logger.Info("timekeeper starting",
		slog.String("addr", *addr),
		slog.String("db", *dbPath),
		slog.String("ui", *uiPath),
		slog.Duration("keep_alive", *keepAliveInterval),
		slog.Duration("pulse_guardian", *pulseGuardianInterval),
		slog.String("log_mode", logMode.String()),
		slog.String("go_version", runtime.Version()),
	)
	if *pulseGuardianInterval != 0 && *pulseGuardianInterval < time.Second {
		log.Fatalf("Pulse Guardian interval must be at least %s", time.Second)
	}
	if *shadowCopyInterval != 0 && *shadowCopyInterval < time.Minute {
		log.Fatalf("shadow copy interval must be at least %s or 0", time.Minute)
	}

	// One Core per TimeKeeper installation, independent of the configured DB or
	// listener. Short-lived tk CLI processes remain clients and do not take this lock.
	if *backupTo == "" {
		coreLock, err := instanceguard.AcquireCoreInstance(*dbPath, *uiPath)
		if err != nil {
			if errors.Is(err, instanceguard.ErrCoreAlreadyRunning) {
				log.Fatal("tk is already running")
			}
			log.Fatalf("acquire TimeKeeper Core singleton lock: %v", err)
		}
		defer coreLock.Close()
		logger.Info("TimeKeeper Core singleton lock acquired", slog.String("path", coreLock.Path))
	}

	if *restoreShadowSlot != "" {
		if *backupTo != "" {
			log.Fatal("-backup-to and -restore-shadow-slot cannot be used together")
		}
		shadowDirectory, err := resolveShadowCopyDirectory(*dbPath, *shadowCopyDir)
		if err != nil {
			log.Fatalf("resolve shadow recovery directory: %v", err)
		}
		result, err := runShadowRestore(context.Background(), *dbPath, shadowDirectory, *restoreShadowSlot, *addr, *confirmShadowRestore)
		if err != nil {
			log.Fatalf("SQLite shadow restore: %v", err)
		}
		logger.Warn("SQLite primary restored from verified shadow", slog.String("slot", result.Slot), slog.String("primary", result.PrimaryPath), slog.String("preserved", result.PreservedDirectory), slog.String("sha256", result.SHA256))
		fmt.Printf("Restored %s from %s. Previous database and sidecars preserved in %s. Restart Core explicitly.\n", result.PrimaryPath, result.Slot, result.PreservedDirectory)
		for _, warning := range result.CleanupWarnings {
			logger.Warn("shadow restore cleanup warning", slog.String("warning", warning))
		}
		return
	}
	if *confirmShadowRestore {
		log.Fatal("-confirm-shadow-restore requires -restore-shadow-slot")
	}
	if *backupTo == "" {
		if err := validateServerConfig(*addr, *uiPath); err != nil {
			log.Fatalf("validate Time Keeper configuration: %v", err)
		}
	}

	// Serialize Core processes that target the same durable database. This is
	// separate from the listener bind: it prevents a second instance using a
	// different port from running its own migrations and Pulse Guardian against
	// the same SQLite state. Backup mode is intentionally exempt/read-only.
	if *backupTo == "" {
		instanceLock, err := instanceguard.Acquire(*dbPath)
		if err != nil {
			if errors.Is(err, instanceguard.ErrAlreadyRunning) {
				log.Fatal("tk is already running")
			}
			log.Fatalf("TimeKeeper database instance guard: %v", err)
		}
		defer instanceLock.Close()
		logger.Info("TimeKeeper database lock acquired", slog.String("path", instanceLock.Path))
	}

	// Reserve the primary listener before opening/migrating the database or
	// starting Pulse Guardian. A second process configured with another DB but
	// the same service address must fail before it can become a parallel Core.
	var primaryListener net.Listener
	if *backupTo == "" {
		listener, listenErr := net.Listen("tcp", *addr)
		if listenErr != nil {
			log.Fatalf("tk is already running or the configured Core listener is unavailable: %v", listenErr)
		}
		primaryListener = listener
		defer primaryListener.Close()
	}

	database, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open Time Keeper database: %v", err)
	}
	defer database.Close()
	if *backupTo != "" {
		message, err := runBackup(context.Background(), database, *backupTo)
		if err != nil {
			log.Fatalf("Time Keeper backup: %v", err)
		}
		log.Print(message)
		return
	}
	var stopShadow func()
	if *shadowCopyInterval > 0 {
		shadowDirectory, shadowErr := resolveShadowCopyDirectory(*dbPath, *shadowCopyDir)
		if shadowErr != nil {
			logger.Error("SQLite shadow copy unavailable", slog.Any("err", shadowErr))
		} else {
			manager, shadowErr := shadowcopy.New(database, shadowDirectory, store.CheckDatabaseFileIntegrity)
			if shadowErr != nil {
				logger.Error("SQLite shadow copy unavailable", slog.String("directory", shadowDirectory), slog.Any("err", shadowErr))
			} else {
				shadowContext, cancelShadow := context.WithCancel(context.Background())
				shadowDone := make(chan struct{})
				stopShadow = func() {
					cancelShadow()
					<-shadowDone
				}
				logger.Info("SQLite A/B shadow copy enabled", slog.String("directory", shadowDirectory), slog.Duration("interval", *shadowCopyInterval))
				go func() {
					defer close(shadowDone)
					if err := shadowcopy.RunPeriodic(shadowContext, manager, *shadowCopyInterval, func(result shadowcopy.Result, err error) {
						if err != nil {
							logger.Error("SQLite shadow copy skipped or failed", slog.Any("err", err))
							return
						}
						logger.Info("SQLite shadow copy verified", slog.String("slot", result.Slot), slog.String("path", result.Path), slog.String("sha256", result.SHA256), slog.Time("created_at", result.CreatedAt))
					}); err != nil {
						logger.Error("SQLite shadow copy worker stopped", slog.Any("err", err))
					}
				}()
			}
		}
	}
	guardianRecovery, err := newGuardianRecoveryHandler(database, *guardianStateDir)
	if err != nil {
		log.Fatalf("configure Guardian recovery: %v", err)
	}
	guardianRecoveryURL := ""
	if guardianRecovery != nil {
		guardianRecoveryURL = strings.TrimRight(friendlyURLs(*addr)[2], "/") + "/api/v1/guardian/recover"
		logger.Info("guardian recovery enabled", slog.String("url", guardianRecoveryURL), slog.String("state_dir", *guardianStateDir))
	}
	if *pulseGuardianInterval > 0 {
		guardianContext, guardianStop := context.WithCancel(context.Background())
		defer guardianStop()
		var ntfyConfig *guardian.NtfyConfig
		if *pulseGuardianNtfyServer != "" && *pulseGuardianNtfyTopic != "" {
			ntfyConfig = &guardian.NtfyConfig{
				ServerURL: *pulseGuardianNtfyServer,
				Topic:     *pulseGuardianNtfyTopic,
				Token:     *pulseGuardianNtfyToken,
			}
			logger.Info("pulse guardian ntfy enabled",
				slog.String("server", *pulseGuardianNtfyServer),
				slog.String("topic", *pulseGuardianNtfyTopic))
		}
		go guardian.Run(guardianContext, database, *pulseGuardianInterval, ntfyConfig, func(err error) {
			logger.Error("pulse guardian", slog.Any("err", err))
			log.Printf("Pulse Guardian: %v", err)
		})
		logger.Info("pulse guardian enabled", slog.Duration("interval", *pulseGuardianInterval))
		log.Printf("Pulse Guardian enabled with %s interval", *pulseGuardianInterval)
	}
	if *keepAliveInterval > 0 {
		go runKeepAlive(context.Background(), *addr, *keepAliveInterval)
	}

	// Bind before publishing runtime URLs, so clients see the actual reachable port.
	var proxyListener net.Listener
	friendlyAddr := *addr
	if strings.TrimSpace(*proxyAddr) != "" {
		if err := validateLoopbackAddr(*proxyAddr); err != nil {
			log.Fatalf("proxy address: %v", err)
		}
		var err error
		proxyListener, err = net.Listen("tcp", *proxyAddr)
		if err != nil {
			logger.Warn("friendly listener unavailable; use canonical-port hostnames", slog.Any("err", err))
		} else {
			friendlyAddr = proxyListener.Addr().String()
			defer proxyListener.Close()
		}
	}
	var knowledgeService api.KnowledgeService
	if descriptor := strings.TrimSpace(*obsidianDescriptor); descriptor != "" {
		// Resolve the provider through the plugin registry so core never
		// names a concrete plugin. An unavailable plugin degrades to
		// "unavailable" instead of preventing startup.
		instance, err := plugin.Build(plugin.KindKnowledge, obsidianplugin.ID, map[string]string{
			obsidianplugin.DescriptorKey: descriptor,
		})
		if err != nil {
			logger.Warn("knowledge plugin unavailable", slog.Any("err", err))
		} else if provider, ok := instance.(knowledge.KnowledgeProvider); ok {
			knowledgeService = knowledge.NewService(provider, obsidianplugin.DefaultTimeout)
			logger.Info("knowledge plugin configured",
				slog.String("plugin", obsidianplugin.ID),
				slog.String("provider", knowledge.ProviderNameObsidian))
		} else {
			logger.Warn("knowledge plugin returned an unexpected type", slog.String("plugin", obsidianplugin.ID))
		}
	}
	// The observer is optional. When no journal path is configured the
	// capability is simply absent; the server still runs.
	var eventObserver observer.Observer
	if path := strings.TrimSpace(*edgeJournal); path != "" {
		instance, err := plugin.Build(plugin.KindObserver, edgeplugin.ID, map[string]string{
			edgeplugin.JournalPathKey: path,
		})
		if err != nil {
			logger.Warn("observer plugin unavailable", slog.Any("err", err))
		} else if obs, ok := instance.(observer.Observer); ok {
			eventObserver = obs
			logger.Info("observer plugin configured",
				slog.String("plugin", edgeplugin.ID))
		} else {
			logger.Warn("observer plugin returned an unexpected type", slog.String("plugin", edgeplugin.ID))
		}
	}
	// Report the observer on the runtime status so an agent can see whether
	// journalling is active without guessing from the flag.
	var observerEnabled, observerAvailable bool
	if eventObserver != nil {
		observerEnabled = true
		if st, err := eventObserver.Status(context.Background()); err == nil {
			observerAvailable = st.Available
		}
	}

	var historyService api.FileHistoryService
	if strings.TrimSpace(*historyRoot) != "" {
		configuredHistory, historyErr := history.New(*historyRoot, database)
		if historyErr != nil {
			log.Fatalf("configure file history: %v", historyErr)
		}
		historyService = configuredHistory
		logger.Info("tracked-file history configured", slog.String("root", *historyRoot))
	}
	apiHandler := api.NewWithGuardianRecovery(database, api.RuntimeStatus{
		PulseGuardianEnabled:         *pulseGuardianInterval > 0,
		PulseGuardianIntervalSeconds: int64(pulseGuardianInterval.Seconds()),
		RecoveryPolicy:               recoveryPolicy(),
		GuardianRecoveryURL:          guardianRecoveryURL,
		FriendlyURLs:                 friendlyURLs(friendlyAddr),
		PluginStoreRoot:              *pluginStoreRoot,
		ObserverEnabled:              observerEnabled,
		ObserverAvailable:            observerAvailable,
	}, knowledgeService, historyService, eventObserver, guardianRecovery)
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/health", apiHandler)
	portalDir := filepath.Join(filepath.Dir(*uiPath), "..", "portal")
	if info, err := os.Stat(portalDir); err == nil && info.IsDir() {
		mux.Handle("/portal/", http.StripPrefix("/portal/", http.FileServer(http.Dir(portalDir))))
	}
	mux.HandleFunc("/", dashboard(*uiPath))

	primaryHandler := securityHeaders(mux)
	server := &http.Server{
		Addr:              *addr,
		Handler:           primaryHandler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Proxy listener for the friendly URLs. Always on; can be disabled
	// by passing an empty -proxy-addr. The proxy forwards to the same
	// handler; the Host header is preserved so any future Host-based
	// routing (or per-host access logs) keeps working. A bind failure
	// is logged but does not abort the canonical server — the
	// canonical 127.0.0.1:1618 listener still answers tools and
	// scripts, and `tk doctor` reports the proxy as failed.
	var proxyServer *http.Server
	if strings.TrimSpace(*proxyAddr) != "" {
		if err := validateLoopbackAddr(*proxyAddr); err != nil {
			logger.Error("invalid proxy address", slog.String("addr", *proxyAddr), slog.Any("err", err))
			log.Fatalf("proxy address %q: %v", *proxyAddr, err)
		}
		proxyServer = newProxyServer(*proxyAddr, primaryHandler, logger)
	}

	contactURLs := friendlyURLs(friendlyAddr)
	defer logging.Close()
	logger.Info("timekeeper listening",
		slog.String("url", contactURLs[0]),
		slog.String("canonical_url", "http://"+*addr+"/"),
		slog.String("proxy_addr", *proxyAddr),
		slog.Any("friendly_urls", contactURLs),
	)
	if proxyServer != nil && proxyListener != nil {
		logger.Info("proxy listener bound", slog.String("addr", proxyServer.Addr))
	}
	log.Printf("Contact point: %s", contactURLs[0])
	for _, u := range contactURLs[1:] {
		log.Printf("  - %s (friendly local endpoint)", u)
	}
	log.Printf("  - http://%s/ (loopback diagnostic fallback)\n", *addr)
	shutdownContext, stopServer := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopServer()
	stopRegistrationQueueWorker := startRegistrationQueueWorker(shutdownContext, database, logger)
	defer stopRegistrationQueueWorker()
	if stopShadow != nil {
		defer stopShadow()
	}
	watchParentInput(shutdownContext, os.Stdin, *shutdownOnStdinClose, stopServer)
	if err := serveUntilShutdown(shutdownContext, server, primaryListener, proxyServer, proxyListener); err != nil {
		logger.Error("server exited", slog.Any("err", err))
		log.Printf("Time Keeper server: %v", err)
		exitCode = 1
	}
}

// watchParentInput enables graceful desktop-child shutdown without exposing a
// public shutdown route. Ordinary service instances leave this disabled.
func watchParentInput(ctx context.Context, input io.Reader, enabled bool, stop context.CancelFunc) {
	if !enabled || input == nil || stop == nil {
		return
	}
	go func() {
		if _, err := io.Copy(io.Discard, input); err == nil && ctx.Err() == nil {
			stop()
		}
	}()
}

// serveUntilShutdown owns both already-bound listeners and shuts them down
// together when signalled. The caller's deferred Store and instance-lock
// cleanup runs after this function returns.
func serveUntilShutdown(ctx context.Context, primary *http.Server, primaryListener net.Listener, proxy *http.Server, proxyListener net.Listener) error {
	serveErrors := make(chan error, 2)
	go func() { serveErrors <- primary.Serve(primaryListener) }()
	if proxy != nil && proxyListener != nil {
		go func() { serveErrors <- proxy.Serve(proxyListener) }()
	}
	var serveErr error
	select {
	case <-ctx.Done():
	case err := <-serveErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	servers := []*http.Server{primary}
	if proxy != nil && proxyListener != nil {
		servers = append(servers, proxy)
	}
	shutdownErrors := make(chan error, len(servers))
	for _, server := range servers {
		go func(server *http.Server) {
			shutdownErrors <- server.Shutdown(shutdownCtx)
		}(server)
	}
	var shutdownErr error
	for range servers {
		shutdownErr = errors.Join(shutdownErr, <-shutdownErrors)
	}
	return errors.Join(serveErr, shutdownErr)
}

// runKeepAlive makes a low-frequency loopback health request so a local
// service remains active in environments that suspend otherwise-idle WSL
// workloads. It deliberately does not touch SQLite, write logs, or keep the
// host awake; service and OS power policy remain the real lifecycle owners.
func runKeepAlive(ctx context.Context, addr string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		select {
		case <-ticker.C:
			keepAliveOnce(ctx, client, addr)
		case <-ctx.Done():
			return
		}
	}
}

func keepAliveOnce(ctx context.Context, client *http.Client, addr string) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/health", nil)
	if err != nil {
		return
	}
	response, err := client.Do(request)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}

func runBackup(ctx context.Context, database *store.Store, destination string) (string, error) {
	if err := database.BackupTo(ctx, destination); err != nil {
		return "", err
	}
	if err := store.CheckDatabaseFileIntegrity(ctx, destination); err != nil {
		return "", fmt.Errorf("SQLite backup candidate integrity check failed: %w", err)
	}
	return "Time Keeper backup created and integrity-verified: " + destination, nil
}

func dashboard(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if identifier, ok := harness.QueryIdentifier(r.URL.RawQuery); ok {
				serveHarnessDocument(w, r, identifier)
				return
			}
		}
		if r.URL.Path == "/harness" || r.URL.Path == "/harness/" || r.URL.Path == "/harness.html" {
			serveHarnessViewer(w, r)
			return
		}
		if r.URL.Path == "/api/harness/catalog" {
			serveHarnessCatalogAPI(w, r)
			return
		}
		if r.URL.Path == "/api/harness/content" {
			serveHarnessContentAPI(w, r)
			return
		}
		// Serve the web directory statically. index.html is the canonical dashboard entrypoint.
		webDir := filepath.Dir(path)
		fs := http.FileServer(http.Dir(webDir))
		// Strip prefix so paths resolve relative to webDir
		http.StripPrefix("/", fs).ServeHTTP(w, r)
	}
}

// wantsHTML reports whether the request explicitly accepts an HTML response.
// Browsers always list text/html in their Accept header; API clients using
// */*, text/markdown, or no Accept header keep receiving the raw bytes.
func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func serveHarnessDocument(w http.ResponseWriter, r *http.Request, identifier string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := harness.FindRoot()
	if err != nil {
		http.Error(w, "bundled harness unavailable", http.StatusServiceUnavailable)
		return
	}
	item, content, err := harness.Lookup(root, identifier)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet && wantsHTML(r) {
		renderHarnessViewerForItem(w, item)
		return
	}
	contentType := "text/plain; charset=utf-8"
	switch filepath.Ext(item.Path) {
	case ".md":
		contentType = "text/markdown; charset=utf-8"
	case ".yaml":
		contentType = "application/yaml; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filepath.Base(item.Path)}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(content)
	}
}

func renderHarnessViewerForItem(w http.ResponseWriter, item harness.Item) {
	serveHarnessViewerWithSelection(w, &http.Request{Method: http.MethodGet}, item.ID)
}

func serveHarnessViewer(w http.ResponseWriter, r *http.Request) {
	selectedID := strings.TrimSpace(r.URL.Query().Get("item"))
	if selectedID != "" {
		root, err := harness.FindRoot()
		if err != nil {
			http.Error(w, "bundled harness unavailable", http.StatusServiceUnavailable)
			return
		}
		item, _, err := harness.Lookup(root, selectedID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		selectedID = item.ID
	}
	serveHarnessViewerWithSelection(w, r, selectedID)
}

func serveHarnessViewerWithSelection(w http.ResponseWriter, r *http.Request, selectedID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page := "<!doctype html>\n" +
		"<html lang=\"en\">\n<head>\n" +
		"  <meta charset=\"utf-8\">\n" +
		"  <meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n" +
		"  <title>TimeKeeper · Harness Viewer</title>\n" +
		"  <link rel=\"stylesheet\" href=\"/harness.css\">\n</head>\n<body data-selected-id=\"" + html.EscapeString(selectedID) + "\">\n" +
		"  <div class=\"harness-viewer\">\n" +
		"    <aside class=\"harness-sidebar\" aria-label=\"Harness documents\">\n" +
		"      <header class=\"harness-sidebar-header\"><h2>Harness Documents</h2><span id=\"harness-count\">Loading…</span></header>\n" +
		"      <ul class=\"harness-menu\" id=\"harness-menu\" role=\"listbox\"></ul>\n" +
		"    </aside>\n" +
		"    <main class=\"harness-main\">\n" +
		"      <header class=\"harness-header\"><p class=\"harness-eyebrow\">TimeKeeper harness document</p>\n" +
		"        <h1 id=\"harness-title\">Select a document</h1><p id=\"harness-path\" class=\"harness-path\"></p>\n" +
		"        <div class=\"harness-actions\"><button class=\"harness-copy\" id=\"harness-copy\" type=\"button\" disabled>Copy file</button> <a class=\"harness-back\" href=\"/\">← Dashboard</a></div>\n" +
		"      </header>\n" +
		"      <textarea class=\"harness-source\" id=\"harness-source\" readonly spellcheck=\"false\" aria-label=\"Document source\" placeholder=\"Select a harness document from the menu\"></textarea>\n" +
		"    </main>\n  </div>\n<script src=\"/harness-viewer.js\"></script>\n</body>\n</html>\n"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(page)))
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, page)
	}
}

func serveHarnessCatalogAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := harness.FindRoot()
	if err != nil {
		http.Error(w, "bundled harness unavailable", http.StatusServiceUnavailable)
		return
	}
	items, err := harness.Entries(root)
	if err != nil {
		http.Error(w, "harness catalog error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(items)
	}
}

func serveHarnessContentAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}
	root, err := harness.FindRoot()
	if err != nil {
		http.Error(w, "bundled harness unavailable", http.StatusServiceUnavailable)
		return
	}
	_, content, err := harness.Lookup(root, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(content)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
