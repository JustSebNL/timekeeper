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
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultBindAddr = "127.0.0.1:1619"
	defaultWorkScript = "scripts/guardian/self-nudge-work.sh"
)

// nudgeSignal mirrors model.PulseGuardianSignal.
type nudgeSignal struct {
	Format string `json:"format"`
	Action string `json:"action"`
	Nudge  struct {
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
	mux.HandleFunc("/pulse", func(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("received nudge %d for agent %s (kind=%s)", sig.Nudge.NudgeID, sig.Nudge.AgentID, sig.Nudge.Kind)

		// Run the work cycle.
		if err := runWorkCycle(ctx, workScript); err != nil {
			log.Printf("work cycle failed: %v", err)
			// Still ack — the nudge was delivered; the work cycle failing
			// should not block the guardian from moving on.
		}

		// Acknowledge the nudge.
		if err := acknowledgeNudge(ctx, tkBaseURL, sig.Nudge.AgentID, sig.Nudge.NudgeID); err != nil {
			log.Printf("failed to ack nudge: %v", err)
			http.Error(w, "ack failed", http.StatusInternalServerError)
			return
		}
		log.Printf("nudge %d acknowledged", sig.Nudge.NudgeID)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})

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

// runWorkCycle executes the work script. The script is responsible for
// running tk commands (pulse, sp next, build, test, msg add, sp complete).
func runWorkCycle(ctx context.Context, script string) error {
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("work script %s: %w", script, err)
	}
	cmd := exec.CommandContext(ctx, "bash", script)
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
