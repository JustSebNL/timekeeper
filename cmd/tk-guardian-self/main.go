// tk-guardian-self is the self-nudge receiver for the Pulse Guardian.
//
// It binds to a loopback-only HTTP endpoint, receives guardian nudge signals,
// runs a local work cycle, and acknowledges the nudge back through the
// documented TimeKeeper API. The agent registers itself with a progress lease
// and guardian_url pointing at this receiver; when the lease expires the
// guardian fires a nudge, the receiver wakes the agent into a work cycle
// instead of letting it idle.
//
// Security:
//   - Binds to 127.0.0.1 only (loopback)
//   - No remote execution — runs only local CLI commands via the work script
//   - Validates nudge payload format before acting
//   - Acknowledges nudge only after work cycle completes
package main

import (
	"bytes"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/stuck"
	"syscall"
	"time"
)

const (
	defaultBindAddr   = "127.0.0.1:1619"
	defaultWorkScript = "scripts/guardian/self-nudge-work.sh"
)

// nudgeSignal mirrors model.PulseGuardianSignal.
type nudgeSignal struct {
	Format   string `json:"format"`
	Action   string `json:"action"`
	Reminder string `json:"reminder"`
	Nudge    struct {
		NudgeID int64  `json:"nudge_id"`
		AgentID string `json:"agent_id"`
		Kind    string `json:"kind"`
	} `json:"nudge"`
}

func main() {
	bindAddr := defaultBindAddr
	if v := os.Getenv("TK_GUARDIAN_SELF_BIND"); v != "" {
		bindAddr = v
	}
	workScript := defaultWorkScript
	if v := os.Getenv("TK_GUARDIAN_SELF_WORK"); v != "" {
		workScript = v
	}
	tkBaseURL := "http://127.0.0.1:1618"
	if v := os.Getenv("TK_BASE_URL"); v != "" {
		tkBaseURL = v
	}
	agentID := os.Getenv("TK_AGENT_ID")
	if agentID == "" {
		log.Fatal("TK_AGENT_ID is required")
	}
	workGate := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT/SIGTERM gracefully.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down self-nudge receiver")
		cancel()
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// The guardian (pulse engine) delivers to whatever URL was registered
	// as guardian_url. We register http://127.0.0.1:1619/pulse and handle
	// both /pulse and /v1/recover for robustness.
	nudgeHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var sig nudgeSignal
		if err := json.Unmarshal(body, &sig); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if sig.Format != "timekeeper-pulse-guardian/v1" {
			http.Error(w, "unsupported format", http.StatusBadRequest)
			return
		}
		if sig.Nudge.NudgeID == 0 || sig.Nudge.AgentID == "" {
			http.Error(w, "missing nudge fields", http.StatusBadRequest)
			return
		}
		if sig.Nudge.AgentID != agentID {
			// A shared loopback port may receive stale signals intended for
			// another worker. Never run this worker for another identity.
			http.Error(w, "nudge is for another agent", http.StatusNotFound)
			return
		}
		select {
		case workGate <- struct{}{}:
			defer func() { <-workGate }()
		default:
			http.Error(w, "work cycle already running", http.StatusServiceUnavailable)
			return
		}
		log.Printf("received nudge %d for agent %s (kind=%s)", sig.Nudge.NudgeID, sig.Nudge.AgentID, sig.Nudge.Kind)
		if sig.Reminder != "" {
			log.Printf("REMINDER: %s", sig.Reminder)
		}

		// Run the work cycle with stuck detection.
		detector := stuck.NewDetector(stuck.DefaultConfig())
		workErr := runWorkCycle(ctx, workScript, detector)
		if !shouldAcknowledgeWorkCycle(workErr) {
			log.Printf("work cycle failed: %v", workErr)
			// Do not ack a failed recovery. The Guardian must retry it instead
			// of treating a printed reminder as completed work.
			http.Error(w, "work cycle failed", http.StatusInternalServerError)
			return
		}

		// Acknowledge the nudge.
		if err := acknowledgeNudge(ctx, tkBaseURL, sig.Nudge.AgentID, sig.Nudge.NudgeID); err != nil {
			log.Printf("failed to ack nudge: %v", err)
			http.Error(w, "ack failed", http.StatusInternalServerError)
			return
		}
		log.Printf("nudge %d acknowledged", sig.Nudge.NudgeID)
		w.Header().Set("X-Timekeeper-Pulse-Accepted", "v1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}
	mux.HandleFunc("/pulse", nudgeHandler)
	mux.HandleFunc("/v1/recover", nudgeHandler)

	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("self-nudge receiver listening on %s (agent=%s, work=%s)", bindAddr, agentID, workScript)
	if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}

func shouldAcknowledgeWorkCycle(err error) bool {
	return err == nil
}

// runWorkCycle executes the work script. The script is responsible for
// running tk commands (pulse, sp next, build, test, msg add, sp complete).
func runWorkCycle(ctx context.Context, script string, detector *stuck.Detector) error {
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("work script %s: %w", script, err)
	}
	cmd := buildWorkCmd(ctx, script)
	if detector != nil {
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("create stdout pipe: %w", err)
		}
		cmd.Stderr = cmd.Stdout
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start work cycle: %w", err)
		}
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Build OK") || strings.Contains(line, "build pass") {
				detector.RecordNow(stuck.SignalBuildPass, nil)
			} else if strings.Contains(line, "Tests passed") || strings.Contains(line, "test pass") {
				detector.RecordNow(stuck.SignalTestPass, nil)
			} else if strings.Contains(line, "Sprint") && strings.Contains(line, "completed") {
				detector.RecordNow(stuck.SignalWorkCycle, nil)
			} else if strings.Contains(line, "failed") || strings.Contains(line, "error") {
				detector.RecordNow(stuck.SignalError, map[string]string{"reason": line})
			}
			fmt.Println(line)
		}
		return cmd.Wait()
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// acknowledgeNudge sends an ack to the TimeKeeper API.
func acknowledgeNudge(ctx context.Context, baseURL, agentID string, nudgeID int64) error {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + agentID + "/nudges/" + strconv.FormatInt(nudgeID, 10) + "/ack"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ack returned %s", resp.Status)
	}
	return nil
}

func buildWorkCmd(ctx context.Context, script string) *exec.Cmd {
	ext := strings.ToLower(filepath.Ext(script))
	if runtime.GOOS == "windows" {
		switch ext {
		case ".ps1":
			return exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script)
		case ".bat", ".cmd":
			return exec.CommandContext(ctx, "cmd.exe", "/c", script)
		default:
			if _, err := exec.LookPath("bash"); err == nil {
				return exec.CommandContext(ctx, "bash", script)
			}
			if _, err := exec.LookPath("sh"); err == nil {
				return exec.CommandContext(ctx, "sh", script)
			}
			return exec.CommandContext(ctx, "cmd.exe", "/c", script)
		}
	}
	switch ext {
	case ".ps1":
		if _, err := exec.LookPath("pwsh"); err == nil {
			return exec.CommandContext(ctx, "pwsh", "-File", script)
		}
		return exec.CommandContext(ctx, "powershell", "-File", script)
	default:
		if _, err := exec.LookPath("bash"); err == nil {
			return exec.CommandContext(ctx, "bash", script)
		}
		return exec.CommandContext(ctx, "sh", script)
	}
}
