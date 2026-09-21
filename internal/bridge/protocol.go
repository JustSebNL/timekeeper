// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package bridge is the only way the two boxes may talk.
//
// TimeKeeper is split into a core box and a plugin box. Neither may
// reach into the other's memory, types or database. Everything
// crossing between them goes through two predetermined tunnels:
//
//   - control  — newline-delimited JSON over stdio (spawn, handshake,
//     health, shutdown)
//   - capability — JSON over loopback HTTP (the actual work)
//
// A plugin that crashes, hangs or misbehaves is a process boundary
// away from core. Core never imports a plugin, and a plugin never
// receives a core pointer, a *store.Store or a filesystem path it did
// not declare.
package bridge

import "errors"

// ProtocolVersion is asserted on both sides during the handshake. A
// mismatch refuses the connection rather than half-working.
const ProtocolVersion = "timekeeper-bridge/v1"

var (
	// ErrProtocolMismatch is returned when the two boxes disagree.
	ErrProtocolMismatch = errors.New("bridge: protocol version mismatch")
	// ErrHandshakeFailed covers a malformed or absent hello.
	ErrHandshakeFailed = errors.New("bridge: handshake failed")
	// ErrClosed means the tunnel is gone; the plugin box is dead or gone.
	ErrClosed = errors.New("bridge: tunnel closed")
	// ErrCapabilityUnknown means core asked for something the plugin box
	// did not declare during its handshake.
	ErrCapabilityUnknown = errors.New("bridge: capability not declared")
)

// Method names the control tunnel understands.
const (
	MethodHello    = "hello"
	MethodHelloOK  = "hello_ok"
	MethodHealth   = "health"
	MethodHealthOK = "health_ok"
	MethodShutdown = "shutdown"
	MethodError    = "error"
)

// Envelope is the wire format for every control message. ID correlates
// a response with its request; Method selects the action; Params
// carries a per-method payload.
//
// The payload is intentionally untyped at this layer so the bridge
// never needs to know what capabilities exist.
type Envelope struct {
	ID     string         `json:"id,omitempty"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

// HelloParams is what core declares when it spawns a plugin box.
type HelloParams struct {
	ProtocolVersion string            `json:"protocol_version"`
	PluginID        string            `json:"plugin_id"`
	Kind            string            `json:"kind"`
	Config          map[string]string `json:"config,omitempty"`
	// CapabilityBase is the loopback HTTP origin the plugin box must
	// serve its capability tunnel on. Core gives it to the plugin; the
	// plugin never invents one.
	CapabilityBase string `json:"capability_base,omitempty"`
}

// HelloResult is what the plugin box answers. DeclaredCapabilities is
// the allowlist: core refuses to call anything absent from it.
type HelloResult struct {
	ProtocolVersion      string   `json:"protocol_version"`
	PluginID             string   `json:"plugin_id"`
	DeclaredCapabilities []string `json:"declared_capabilities"`
}

// HealthResult answers a health probe.
type HealthResult struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

// ErrorPayload carries a refusal across the tunnel. Code is safe to
// log; Detail is best-effort and may be empty.
type ErrorPayload struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}
