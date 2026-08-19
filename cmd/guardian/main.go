// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Command guardian is the repo-local Pulse Guardian recovery receiver. It is a
// separate process surface from the Time Keeper server so a hung agent work
// loop cannot take it down. It accepts local recover_attention signals on a
// numeric loopback port, records a durable recovery artifact, and acknowledges
// the nudge back through Time Keeper's documented API. It performs no process
// control and never mutates Time Keeper's database directly.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/JustSebNL/timekeeper/internal/guardian"
)

func main() {
	bindAddr := flag.String("addr", "127.0.0.1:1619", "numeric loopback bind address for the recovery receiver")
	stateDir := flag.String("state-dir", "", "private state directory that already holds recoverable Time Keeper state (required)")
	timekeeperURL := flag.String("timekeeper-url", "http://127.0.0.1:1618", "Time Keeper loopback API base URL used to acknowledge nudges")
	agentID := flag.String("agent", "xatia", "agent id whose progress lease this receiver recovers")
	register := flag.Bool("register", true, "register this receiver's loopback URL as the agent's Guardian callback on startup")
	flag.Parse()

	if *stateDir == "" {
		log.Fatalf("guardian receiver requires -state-dir (an existing private state directory)")
	}
	absState, err := filepath.Abs(*stateDir)
	if err != nil {
		log.Fatalf("resolve state directory: %v", err)
	}
	if info, statErr := os.Stat(absState); statErr != nil || !info.IsDir() {
		log.Fatalf("guardian receiver state directory unavailable: %v", statErr)
	}

	receiver, err := guardian.NewReceiver(guardian.ReceiverConfig{
		BindAddr:          *bindAddr,
		TimeKeeperBaseURL: *timekeeperURL,
		StateDir:          absState,
		Action:            guardian.RecoveryActionLocalArtifact,
		PolicyLabel:       "local-artifact: durable recovery marker only; no process control",
	})
	if err != nil {
		log.Fatalf("guardian receiver configuration: %v", err)
	}

	if *register {
		guardianURL := "http://" + *bindAddr + "/v1/recover"
		registered := false
		for attempt := 1; attempt <= 10; attempt++ {
			if regErr := registerGuardianURL(*timekeeperURL, *agentID, guardianURL); regErr != nil {
				log.Printf("guardian callback registration attempt %d failed: %v", attempt, regErr)
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			registered = true
			break
		}
		if !registered {
			log.Fatalf("could not register guardian callback with Time Keeper after retries")
		}
		log.Printf("Pulse Guardian receiver registered at %s for agent %s", guardianURL, *agentID)
	}

	ctx, stop := signalContext()
	defer stop()
	log.Printf("Pulse Guardian recovery receiver listening at http://%s/", *bindAddr)
	if runErr := receiver.Run(ctx); runErr != nil {
		log.Fatalf("guardian recovery receiver: %v", runErr)
	}
}

func registerGuardianURL(baseURL, agentID, guardianURL string) error {
	body := fmt.Sprintf(`{"lease_duration_seconds":%d,"guardian_url":%q}`, int64((30 * time.Minute).Seconds()), guardianURL)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/agents/"+url.PathEscape(agentID)+"/progress", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("build registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("register guardian callback: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("Time Keeper returned %s when registering guardian callback", resp.Status)
	}
	return nil
}

func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()
	return ctx, cancel
}
