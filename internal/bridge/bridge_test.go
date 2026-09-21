// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package bridge_test

import (
	"errors"
	"net"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/bridge"
)

// newPipe builds a control tunnel over an in-memory pipe, standing in
// for a real child process's stdio.
func newPipe() (core *bridge.ControlTunnel, plugin *bridge.ControlTunnel) {
	a, b := net.Pipe()
	return bridge.NewControlTunnel(a, a), bridge.NewControlTunnel(b, b)
}

func TestHandshakeAgreedOnBothSides(t *testing.T) {
	core, plugin := newPipe()

	type shake struct {
		params bridge.HelloParams
		result bridge.HandshakeResult
		err    error
	}
	done := make(chan shake, 1)
	go func() {
		result, err := bridge.PerformHandshake(core, bridge.HelloParams{
			PluginID: "extension.memory-galaxy",
			Kind:     "knowledge",
		})
		done <- shake{result: result, err: err}
	}()

	params, err := bridge.ServeHandshake(plugin, "extension.memory-galaxy", []string{"knowledge.search"})
	if err != nil {
		t.Fatalf("plugin handshake: %v", err)
	}
	if params.PluginID != "extension.memory-galaxy" {
		t.Fatalf("plugin saw id %q", params.PluginID)
	}

	got := <-done
	if got.err != nil {
		t.Fatalf("core handshake: %v", got.err)
	}
	if got.result.PluginID != "extension.memory-galaxy" {
		t.Fatalf("plugin id = %q", got.result.PluginID)
	}
	if len(got.result.DeclaredCapabilities) != 1 || got.result.DeclaredCapabilities[0] != "knowledge.search" {
		t.Fatalf("capabilities = %v", got.result.DeclaredCapabilities)
	}
}

func TestProtocolMismatchIsRefused(t *testing.T) {
	core, plugin := newPipe()

	done := make(chan error, 1)
	go func() {
		// Core claims a version the plugin will not accept.
		_, err := bridge.PerformHandshake(core, bridge.HelloParams{
			ProtocolVersion: "timekeeper-bridge/v99",
			PluginID:        "extension.memory-galaxy",
			Kind:            "knowledge",
		})
		done <- err
	}()

	_, err := bridge.ServeHandshake(plugin, "extension.memory-galaxy", nil)
	if err == nil {
		t.Fatal("plugin accepted a mismatched protocol")
	}
	if !errors.Is(err, bridge.ErrProtocolMismatch) {
		t.Fatalf("err = %v, want ErrProtocolMismatch", err)
	}
	if err := <-done; err == nil {
		t.Fatal("core accepted a mismatched protocol")
	}
}

func TestWrongPluginIDIsRefused(t *testing.T) {
	core, plugin := newPipe()

	done := make(chan error, 1)
	go func() {
		_, err := bridge.PerformHandshake(core, bridge.HelloParams{
			PluginID: "extension.something-else",
			Kind:     "knowledge",
		})
		done <- err
	}()

	if _, err := bridge.ServeHandshake(plugin, "extension.memory-galaxy", nil); err == nil {
		t.Fatal("plugin answered for a different id")
	}
	if err := <-done; !errors.Is(err, bridge.ErrHandshakeFailed) {
		t.Fatalf("err = %v, want ErrHandshakeFailed", err)
	}
}

func TestNonLoopbackCapabilityOriginIsRefused(t *testing.T) {
	core, plugin := newPipe()

	done := make(chan error, 1)
	go func() {
		_, err := bridge.PerformHandshake(core, bridge.HelloParams{
			PluginID:       "extension.memory-galaxy",
			Kind:           "knowledge",
			CapabilityBase: "https://evil.example.com:8443",
		})
		done <- err
	}()

	// The plugin is well behaved; core itself must refuse the origin.
	_, _ = bridge.ServeHandshake(plugin, "extension.memory-galaxy", nil)
	if err := <-done; err == nil {
		t.Fatal("core accepted a non-loopback capability origin")
	}
}

func TestClosedTunnelReportsErrClosed(t *testing.T) {
	a, b := net.Pipe()
	core := bridge.NewControlTunnel(a, a)
	_ = b.Close()

	if _, err := core.Receive(); !errors.Is(err, bridge.ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}

func TestErrorPayloadRoundTrips(t *testing.T) {
	sender, receiver := newPipe()

	// net.Pipe is unbuffered: the send blocks until someone reads, so
	// the reader must already be waiting.
	type received struct {
		env bridge.Envelope
		err error
	}
	got := make(chan received, 1)
	go func() {
		env, err := receiver.Receive()
		got <- received{env: env, err: err}
	}()

	if err := bridge.SendError(sender, "7", "not_configured", "descriptor missing"); err != nil {
		t.Fatalf("send error: %v", err)
	}

	result := <-got
	if result.err != nil {
		t.Fatalf("receive: %v", result.err)
	}
	if result.env.ID != "7" {
		t.Fatalf("id = %q", result.env.ID)
	}
	if result.env.Method != bridge.MethodError {
		t.Fatalf("method = %q", result.env.Method)
	}
	payload := bridge.ErrorFromEnvelope(result.env)
	if payload.Code != "not_configured" || payload.Detail != "descriptor missing" {
		t.Fatalf("payload = %+v", payload)
	}
}
