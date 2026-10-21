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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
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
	register := flag.Bool("register", true, "register the advertised canonical recovery URL, or this receiver's compatibility URL, on startup")
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
		client := &http.Client{Timeout: 3 * time.Second}
		guardianURL, discoveryErr := recoveryRegistrationURL(client, *timekeeperURL, *bindAddr)
		if discoveryErr != nil {
			log.Printf("canonical Guardian recovery callback unavailable; using compatibility receiver registration: %v", discoveryErr)
		}
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

// recoveryRegistrationURL prefers the canonical in-process recovery route when
// the API advertises it. The returned error is diagnostic only: the receiver can
// still register its compatibility listener for older Cores and existing agents.
func recoveryRegistrationURL(client *http.Client, baseURL, bindAddr string) (string, error) {
	legacyURL := "http://" + bindAddr + "/v1/recover"
	canonicalURL, err := discoverCanonicalRecoveryURL(client, baseURL)
	if err != nil {
		return legacyURL, err
	}
	if canonicalURL != "" {
		return canonicalURL, nil
	}
	return legacyURL, nil
}

func discoverCanonicalRecoveryURL(client *http.Client, baseURL string) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/guardian/status"
	response, err := client.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("read Guardian status: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return "", nil
	}
	var status struct {
		GuardianRecoveryURL string `json:"guardian_recovery_url"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&status); err != nil {
		return "", fmt.Errorf("decode Guardian status: %w", err)
	}
	if status.GuardianRecoveryURL == "" {
		return "", nil
	}
	if err := validateCanonicalRecoveryURL(status.GuardianRecoveryURL); err != nil {
		return "", fmt.Errorf("invalid canonical Guardian recovery URL: %w", err)
	}
	return status.GuardianRecoveryURL, nil
}

func validateCanonicalRecoveryURL(raw string) error {
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.Path != "/api/v1/guardian/recover" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("canonical Guardian recovery URL must be the plain http recovery route")
	}
	host := net.ParseIP(endpoint.Hostname())
	if host == nil || !host.IsLoopback() {
		return errors.New("canonical Guardian recovery URL must use a numeric loopback host")
	}
	port, err := strconv.ParseUint(endpoint.Port(), 10, 16)
	if err != nil || port == 0 {
		return errors.New("canonical Guardian recovery URL must include a valid numeric port")
	}
	return nil
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
