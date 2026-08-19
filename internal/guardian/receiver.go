// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package guardian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const receiverAcceptedHeader = "X-Timekeeper-Pulse-Accepted"

// RecoveryAction describes what the receiver may do after accepting a Guardian
// signal. The receiver never executes arbitrary commands and never mutates
// Time Keeper's database directly; it only records durable local evidence and
// (optionally) acknowledges the nudge back through the documented API.
type RecoveryAction string

const (
	// RecoveryActionLocalArtifact writes a dated, machine-readable recovery
	// marker under the local state directory and acknowledges the nudge. It
	// performs no process control and needs no external authority.
	RecoveryActionLocalArtifact RecoveryAction = "local-artifact"
)

// ReceiverConfig is the fixed allowlist for the local recovery receiver. Every
// field is explicit; there is no implicit remote execution.
type ReceiverConfig struct {
	// BindAddr must be a numeric loopback host:port, mirroring the constraint
	// already enforced for registered Guardian URLs.
	BindAddr string
	// TimeKeeperBaseURL is the loopback API base used to acknowledge nudges.
	TimeKeeperBaseURL string
	// StateDir holds durable recovery artifacts. It must already exist and be
	// private (0700). The receiver never creates parent directories.
	StateDir string
	// Action is the only recovery behaviour the receiver is permitted to run.
	Action RecoveryAction
	// PolicyLabel is a stable, human-readable description of the recovery policy
	// shown in status output. It is evidence, not a capability.
	PolicyLabel string
}

func validateReceiverBindAddr(raw string) error {
	if raw == "" {
		return errors.New("guardian receiver bind address is required")
	}
	endpoint, err := url.Parse("http://" + raw)
	if err != nil || endpoint.Hostname() == "" || endpoint.Port() == "" {
		return errors.New("guardian receiver bind address must be host:port")
	}
	host := net.ParseIP(endpoint.Hostname())
	if host == nil || !host.IsLoopback() {
		return errors.New("guardian receiver bind address must use a numeric loopback host")
	}
	if _, err := strconv.ParseUint(endpoint.Port(), 10, 16); err != nil {
		return errors.New("guardian receiver bind address must include a valid numeric port")
	}
	// Port 0 is allowed: the kernel assigns a free loopback port, which tests
	// and ad-hoc launchers use before reading the chosen address back.
	return nil
}

// Receiver accepts local Pulse Guardian recovery signals and performs exactly
// one configured RecoveryAction. It is intentionally dependency-free from the
// watched agent's work loop and from Time Keeper's Store.
type Receiver struct {
	config ReceiverConfig
	client *http.Client
}

// NewReceiver validates the configuration and returns a ready receiver.
func NewReceiver(config ReceiverConfig) (*Receiver, error) {
	if err := validateReceiverBindAddr(config.BindAddr); err != nil {
		return nil, err
	}
	if config.TimeKeeperBaseURL == "" {
		return nil, errors.New("guardian receiver requires a Time Keeper API base URL")
	}
	if config.StateDir == "" {
		return nil, errors.New("guardian receiver requires a private state directory")
	}
	if config.Action != RecoveryActionLocalArtifact {
		return nil, fmt.Errorf("unsupported guardian recovery action %q", config.Action)
	}
	if config.PolicyLabel == "" {
		return nil, errors.New("guardian receiver requires a policy label")
	}
	return &Receiver{
		config: config,
		client: &http.Client{Timeout: 5 * time.Second},
	}, nil
}

// BindAddr returns the receiver's configured loopback bind address.
func (r *Receiver) BindAddr() string { return r.config.BindAddr }

// SetBindAddr overrides the loopback bind address. It is used by tests and by
// callers that resolve a free port before listening.
func (r *Receiver) SetBindAddr(addr string) { r.config.BindAddr = addr }

// Handler returns the HTTP handler for the loopback recovery endpoint.
func (r *Receiver) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/recover", r.handleRecover)
	return mux
}

func (r *Receiver) handleRecover(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 64<<10))
	if err != nil {
		http.Error(w, "read signal body", http.StatusBadRequest)
		return
	}
	var signal model.PulseGuardianSignal
	if err := json.Unmarshal(body, &signal); err != nil {
		http.Error(w, "invalid signal JSON", http.StatusBadRequest)
		return
	}
	if signal.Format != "timekeeper-pulse-guardian/v1" {
		http.Error(w, "unsupported signal format", http.StatusBadRequest)
		return
	}
	if signal.Action != "recover_attention" {
		http.Error(w, "unsupported signal action", http.StatusBadRequest)
		return
	}
	if signal.Nudge.NudgeID < 1 || signal.Nudge.AgentID == "" {
		http.Error(w, "signal nudge is incomplete", http.StatusBadRequest)
		return
	}

	switch r.config.Action {
	case RecoveryActionLocalArtifact:
		if err := writeRecoveryArtifact(r.config.StateDir, signal.Nudge); err != nil {
			http.Error(w, "record recovery artifact: "+err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "unsupported recovery action", http.StatusInternalServerError)
		return
	}

	if err := r.acknowledge(req.Context(), signal.Nudge); err != nil {
		// The durable artifact already exists; surface the ack failure so the
		// caller knows the nudge will be retried by the next Guardian tick.
		http.Error(w, "acknowledge nudge: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set(receiverAcceptedHeader, "v1")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accepted":      true,
		"action":        string(r.config.Action),
		"policy":        r.config.PolicyLabel,
		"nudge_id":      signal.Nudge.NudgeID,
		"agent_id":      signal.Nudge.AgentID,
		"active_sprint": signal.Nudge.ActiveSprintID,
	})
}

func (r *Receiver) acknowledge(ctx context.Context, nudge model.PulseNudge) error {
	endpoint := strings.TrimRight(r.config.TimeKeeperBaseURL, "/") +
		"/api/v1/agents/" + url.PathEscape(nudge.AgentID) +
		"/nudges/" + strconv.FormatInt(nudge.NudgeID, 10) + "/ack"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(`{}`))
	if err != nil {
		return fmt.Errorf("build acknowledge request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return fmt.Errorf("send acknowledge request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 16<<10))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Time Keeper returned %s when acknowledging nudge", response.Status)
	}
	return nil
}

// Run starts the loopback receiver and blocks until the listener fails or the
// context is cancelled. It is a separate process surface from the Time Keeper
// server, so a hung agent work loop cannot take it down.
func (r *Receiver) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:              r.config.BindAddr,
		Handler:           r.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("guardian recovery receiver: %w", err)
	}
	return nil
}
