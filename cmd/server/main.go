// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/history"
	"github.com/JustSebNL/timekeeper/internal/knowledge"
	"github.com/JustSebNL/timekeeper/internal/logging"
	"github.com/JustSebNL/timekeeper/internal/plugin"
	obsidianplugin "github.com/JustSebNL/timekeeper/internal/plugin/knowledgeobsidian"
	"github.com/JustSebNL/timekeeper/internal/store"
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
  -obsidian-descriptor <path>                   Optional authenticated Galaxy endpoint descriptor
  -history-root <path>                           Optional tracked-file history root

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
	return []string{"http://timekeeper.local" + suffix + "/", "http://api.timekeeper.local" + suffix + "/"}
}

func main() {
	if len(os.Args) == 1 || os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h" {
		showHelp()
		return
	}
	if os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v" {
		_, _ = fmt.Println("TimeKeeper 1.0")
		return
	}
	if handled, code := dispatchClient(os.Args[1:], os.Getenv("TIMEKEEPER_URL"), os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	addr := flag.String("addr", "127.0.0.1:1618", "HTTP listen address")
	proxyAddr := flag.String("proxy-addr", envOr("TIMEKEEPER_PROXY_ADDR", "127.0.0.1:80"), "reverse-proxy listen address for the friendly URLs (http://timekeeper.local/ and http://api.timekeeper.local/); empty to disable")
	obsidianDescriptor := flag.String("obsidian-descriptor", envOr("TIMEKEEPER_OBSIDIAN_DESCRIPTOR", ""), "optional Obsidian Memory Galaxy endpoint descriptor path")
	historyRoot := flag.String("history-root", envOr("TIMEKEEPER_HISTORY_ROOT", ""), "optional tracked-file history root; empty disables file history routes")
	pluginStoreRoot := flag.String("plugin-store-root", envOr("TIMEKEEPER_PLUGIN_STORE_ROOT", ""), "optional plugin-store directory; empty disables plugin registry routes")
	dbPath := flag.String("db", "timekeeper.db", "SQLite database path")
	uiPath := flag.String("ui", ".timekeeper/web/index.html", "dashboard HTML path")
	keepAliveInterval := flag.Duration("keep-alive-interval", 5*time.Minute, "local dashboard keep-alive interval; 0 disables it")
	backupTo := flag.String("backup-to", "", "create a SQLite backup at this new path, then exit")
	pulseGuardianInterval := flag.Duration("pulse-guardian-interval", 5*time.Minute, "run the local Pulse Guardian at this interval; 0 disables it")
	logPath := flag.String("log", ".timekeeper/log/app.log", "structured JSON log path; empty disables file logging")
	logMode := logging.ModeNormal
	flag.Var(modeFlag{&logMode}, "log-mode", "log mode: normal|debug (debug logs every step, raw inputs, full errors)")
	flag.Parse()

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
	if *backupTo == "" {
		if err := validateServerConfig(*addr, *uiPath); err != nil {
			log.Fatalf("validate Time Keeper configuration: %v", err)
		}
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
	if *pulseGuardianInterval > 0 {
		guardianContext, guardianStop := context.WithCancel(context.Background())
		defer guardianStop()
		go guardian.Run(guardianContext, database, *pulseGuardianInterval, func(err error) {
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
	var historyService api.FileHistoryService
	if strings.TrimSpace(*historyRoot) != "" {
		configuredHistory, historyErr := history.New(*historyRoot, database)
		if historyErr != nil {
			log.Fatalf("configure file history: %v", historyErr)
		}
		historyService = configuredHistory
		logger.Info("tracked-file history configured", slog.String("root", *historyRoot))
	}
	apiHandler := api.NewWithHistory(database, api.RuntimeStatus{
		PulseGuardianEnabled:         *pulseGuardianInterval > 0,
		PulseGuardianIntervalSeconds: int64(pulseGuardianInterval.Seconds()),
		RecoveryPolicy:               recoveryPolicy(),
		FriendlyURLs:                 friendlyURLs(friendlyAddr),
		PluginStoreRoot:              *pluginStoreRoot,
	}, knowledgeService, historyService)
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/health", apiHandler)
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

	defer logging.Close()
	logger.Info("timekeeper listening",
		slog.String("url", "http://"+*addr+"/"),
		slog.String("proxy_addr", *proxyAddr),
		slog.Any("friendly_urls", friendlyURLs(friendlyAddr)),
	)
	if proxyServer != nil && proxyListener != nil {
		go func() {

			logger.Info("proxy listener bound", slog.String("addr", proxyServer.Addr))
			if err := proxyServer.Serve(proxyListener); err != nil && err != http.ErrServerClosed {
				logger.Error("proxy server exited", slog.Any("err", err))
			}
		}()
	}
	log.Println("TimeKeeper is at:")
	for _, u := range friendlyURLs(friendlyAddr) {
		log.Printf("  - %s (requires local hosts entry)", u)
	}
	log.Printf("  - http://%s/  (loopback, tools, diagnostics)\n", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server exited", slog.Any("err", err))
		log.Fatalf("Time Keeper server: %v", err)
	}
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
	return "Time Keeper backup created: " + destination, nil
}

func dashboard(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Serve the web directory statically. index.html is the canonical dashboard entrypoint.
		webDir := filepath.Dir(path)
		fs := http.FileServer(http.Dir(webDir))
		// Strip prefix so paths resolve relative to webDir
		http.StripPrefix("/", fs).ServeHTTP(w, r)
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
