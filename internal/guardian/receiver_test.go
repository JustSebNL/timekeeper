// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package guardian_test

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/model"
)

func newTestStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func TestReceiverRejectsNonLoopbackBind(t *testing.T) {
	_, err := guardian.NewReceiver(guardian.ReceiverConfig{
		BindAddr:          "0.0.0.0:1619",
		TimeKeeperBaseURL: "http://127.0.0.1:1618",
		StateDir:          t.TempDir(),
		Action:            guardian.RecoveryActionLocalArtifact,
		PolicyLabel:       "local-artifact",
	})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback bind rejection, got %v", err)
	}
}

func TestReceiverRejectsUnsupportedAction(t *testing.T) {
	_, err := guardian.NewReceiver(guardian.ReceiverConfig{
		BindAddr:          "127.0.0.1:1619",
		TimeKeeperBaseURL: "http://127.0.0.1:1618",
		StateDir:          t.TempDir(),
		Action:            guardian.RecoveryAction("restart-agent"),
		PolicyLabel:       "local-artifact",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported action rejection, got %v", err)
	}
}

func startReadyServer(t *testing.T, receiver *guardian.Receiver) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	receiver.SetBindAddr(ln.Addr().String())
	srv := &http.Server{Handler: receiver.Handler()}
	go func() { _ = srv.Serve(ln) }()
	return receiver.BindAddr(), func() { _ = srv.Close() }
}

func TestReceiverAcceptsSignalWritesArtifactAndAcks(t *testing.T) {
	stateDir := newTestStateDir(t)

	var ackedAgent, ackedNudge string
	ackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		// api/v1/agents/{agentID}/nudges/{nudgeID}/ack
		ackedAgent = parts[3]
		ackedNudge = parts[5]
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "Acknowledged"})
	}))
	defer ackServer.Close()

	receiver, err := guardian.NewReceiver(guardian.ReceiverConfig{
		BindAddr:          "127.0.0.1:0",
		TimeKeeperBaseURL: ackServer.URL,
		StateDir:          stateDir,
		Action:            guardian.RecoveryActionLocalArtifact,
		PolicyLabel:       "local-artifact",
	})
	if err != nil {
		t.Fatalf("create receiver: %v", err)
	}

	addr, stop := startReadyServer(t, receiver)
	defer stop()

	signal := model.PulseGuardianSignal{
		Format: "timekeeper-pulse-guardian/v1",
		Action: "recover_attention",
		Nudge: model.PulseNudge{
			NudgeID:              42,
			AgentID:              "xatia",
			ActiveSprintID:       "SP-10028",
			Kind:                 "agent_unresponsive",
			DetectedAfterSeconds: 1900,
		},
	}
	body, _ := json.Marshal(signal)
	resp, err := http.Post("http://"+addr+"/v1/recover", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post signal: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("X-Timekeeper-Pulse-Accepted") != "v1" {
		t.Fatalf("receiver did not confirm acceptance; status=%s", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %s", resp.Status)
	}

	if ackedAgent != "xatia" || ackedNudge != "42" {
		t.Fatalf("ack not sent to Time Keeper: agent=%q nudge=%q", ackedAgent, ackedNudge)
	}

	artifact := filepath.Join(stateDir, "recovery", "xatia", "42.json")
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatalf("recovery artifact not written: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("artifact not valid JSON: %v", err)
	}
	if decoded["active_sprint_id"] != "SP-10028" {
		t.Fatalf("artifact missing active sprint: %#v", decoded)
	}
}

func TestReceiverRejectsBadFormat(t *testing.T) {
	stateDir := newTestStateDir(t)
	receiver, err := guardian.NewReceiver(guardian.ReceiverConfig{
		BindAddr:          "127.0.0.1:0",
		TimeKeeperBaseURL: "http://127.0.0.1:1618",
		StateDir:          stateDir,
		Action:            guardian.RecoveryActionLocalArtifact,
		PolicyLabel:       "local-artifact",
	})
	if err != nil {
		t.Fatalf("create receiver: %v", err)
	}
	addr, stop := startReadyServer(t, receiver)
	defer stop()

	bad := `{"format":"wrong/v9","action":"recover_attention","nudge":{"nudge_id":1,"agent_id":"xatia"}}`
	resp, err := http.Post("http://"+addr+"/v1/recover", "application/json", bytes.NewReader([]byte(bad)))
	if err != nil {
		t.Fatalf("post signal: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad format, got %s", resp.Status)
	}
	// No artifact should exist for the rejected signal.
	if _, statErr := os.Stat(filepath.Join(stateDir, "recovery", "xatia", "1.json")); !os.IsNotExist(statErr) {
		t.Fatalf("artifact must not be written for rejected signal")
	}
}

func TestReceiverFailsClosedOnAckError(t *testing.T) {
	stateDir := newTestStateDir(t)
	// ack server returns 500, so the receiver must not claim acceptance.
	ackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ackServer.Close()

	receiver, _ := guardian.NewReceiver(guardian.ReceiverConfig{
		BindAddr:          "127.0.0.1:0",
		TimeKeeperBaseURL: ackServer.URL,
		StateDir:          stateDir,
		Action:            guardian.RecoveryActionLocalArtifact,
		PolicyLabel:       "local-artifact",
	})
	addr, stop := startReadyServer(t, receiver)
	defer stop()

	signal := model.PulseGuardianSignal{Format: "timekeeper-pulse-guardian/v1", Action: "recover_attention",
		Nudge: model.PulseNudge{NudgeID: 7, AgentID: "xatia"}}
	body, _ := json.Marshal(signal)
	resp, err := http.Post("http://"+addr+"/v1/recover", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post signal: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 when ack fails, got %s", resp.Status)
	}
	if resp.Header.Get("X-Timekeeper-Pulse-Accepted") == "v1" {
		t.Fatalf("receiver must not confirm acceptance when ack failed")
	}
}

func TestWriteRecoveryArtifactRejectsUnsafeAgent(t *testing.T) {
	dir := t.TempDir()
	if err := guardian.WriteRecoveryArtifactForTest(dir, model.PulseNudge{NudgeID: 1, AgentID: "../escape"}); err == nil {
		t.Fatalf("expected rejection for unsafe agent_id")
	}
}
