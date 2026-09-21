// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package bridge

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// HandshakeResult is what core learns about a plugin box once it has
// introduced itself.
type HandshakeResult struct {
	PluginID             string
	Kind                 string
	DeclaredCapabilities []string
	CapabilityBase       string
}

// PerformHandshake is the core side: it declares the tunnel, waits for
// the plugin box to introduce itself, and validates what came back.
//
// Validation is deliberately strict and happens once, here:
//
//   - the protocol version must match exactly;
//   - the plugin must answer with the id core asked for;
//   - every declared capability must be a non-empty string.
//
// A plugin cannot widen its capabilities after this point — the
// allowlist is frozen at handshake.
func PerformHandshake(t *ControlTunnel, params HelloParams) (HandshakeResult, error) {
	if params.ProtocolVersion == "" {
		params.ProtocolVersion = ProtocolVersion
	}
	if err := t.Send(Envelope{Method: MethodHello, Params: helloParamsMap(params)}); err != nil {
		return HandshakeResult{}, err
	}

	env, err := t.Receive()
	if err != nil {
		return HandshakeResult{}, err
	}
	if env.Method == MethodError {
		payload := ErrorFromEnvelope(env)
		return HandshakeResult{}, fmt.Errorf("%w: %s %s", ErrHandshakeFailed, payload.Code, payload.Detail)
	}
	if env.Method != MethodHelloOK {
		return HandshakeResult{}, fmt.Errorf("%w: got method %q, want %q", ErrHandshakeFailed, env.Method, MethodHelloOK)
	}

	var result HelloResult
	decoded, err := json.Marshal(env.Params)
	if err != nil {
		return HandshakeResult{}, fmt.Errorf("%w: cannot re-encode params", ErrHandshakeFailed)
	}
	if err := json.Unmarshal(decoded, &result); err != nil {
		return HandshakeResult{}, fmt.Errorf("%w: %v", ErrHandshakeFailed, err)
	}

	if result.ProtocolVersion != ProtocolVersion {
		return HandshakeResult{}, fmt.Errorf("%w: plugin speaks %q, core speaks %q",
			ErrProtocolMismatch, result.ProtocolVersion, ProtocolVersion)
	}
	if result.PluginID != params.PluginID {
		return HandshakeResult{}, fmt.Errorf("%w: plugin identified as %q, core asked for %q",
			ErrHandshakeFailed, result.PluginID, params.PluginID)
	}

	capabilities := make([]string, 0, len(result.DeclaredCapabilities))
	seen := map[string]bool{}
	for _, capability := range result.DeclaredCapabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return HandshakeResult{}, fmt.Errorf("%w: empty capability declared", ErrHandshakeFailed)
		}
		if seen[capability] {
			continue
		}
		seen[capability] = true
		capabilities = append(capabilities, capability)
	}

	base := strings.TrimSpace(params.CapabilityBase)
	if base != "" {
		if err := validateLoopbackOrigin(base); err != nil {
			return HandshakeResult{}, fmt.Errorf("%w: %v", ErrHandshakeFailed, err)
		}
	}

	return HandshakeResult{
		PluginID:             result.PluginID,
		Kind:                 params.Kind,
		DeclaredCapabilities: capabilities,
		CapabilityBase:       base,
	}, nil
}

// ServeHandshake is the plugin-box side: it reads core's hello and
// answers with its own identity and capability allowlist.
func ServeHandshake(t *ControlTunnel, pluginID string, capabilities []string) (HelloParams, error) {
	env, err := t.Receive()
	if err != nil {
		return HelloParams{}, err
	}
	if env.Method != MethodHello {
		return HelloParams{}, fmt.Errorf("%w: got method %q, want %q", ErrHandshakeFailed, env.Method, MethodHello)
	}

	var params HelloParams
	decoded, err := json.Marshal(env.Params)
	if err != nil {
		return HelloParams{}, fmt.Errorf("%w: cannot re-encode params", ErrHandshakeFailed)
	}
	if err := json.Unmarshal(decoded, &params); err != nil {
		return HelloParams{}, fmt.Errorf("%w: %v", ErrHandshakeFailed, err)
	}
	if params.ProtocolVersion != ProtocolVersion {
		if err := t.SendError(env.ID, "protocol_mismatch",
			fmt.Sprintf("plugin speaks %q, core speaks %q", ProtocolVersion, params.ProtocolVersion)); err != nil {
			return HelloParams{}, ErrClosed
		}
		return HelloParams{}, ErrProtocolMismatch
	}
	if params.PluginID != pluginID {
		if err := t.SendError(env.ID, "plugin_mismatch",
			fmt.Sprintf("core asked for %q, this box is %q", params.PluginID, pluginID)); err != nil {
			return HelloParams{}, ErrClosed
		}
		return HelloParams{}, fmt.Errorf("%w: core asked for %q, this box is %q",
			ErrHandshakeFailed, params.PluginID, pluginID)
	}

	if err := t.Send(Envelope{
		ID:     env.ID,
		Method: MethodHelloOK,
		Params: map[string]any{
			"protocol_version":      ProtocolVersion,
			"plugin_id":             pluginID,
			"declared_capabilities": capabilities,
		},
	}); err != nil {
		return HelloParams{}, err
	}
	return params, nil
}

func helloParamsMap(p HelloParams) map[string]any {
	out := map[string]any{
		"protocol_version": p.ProtocolVersion,
		"plugin_id":        p.PluginID,
		"kind":             p.Kind,
	}
	if len(p.Config) > 0 {
		out["config"] = p.Config
	}
	if p.CapabilityBase != "" {
		out["capability_base"] = p.CapabilityBase
	}
	return out
}

// validateLoopbackOrigin refuses any capability origin that is not a
// plain http loopback address. A plugin box must never be told to
// serve — or to call — anything off-machine.
func validateLoopbackOrigin(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid capability origin: %v", err)
	}
	if parsed.Scheme != "http" {
		return fmt.Errorf("capability origin must be plain http, got %q", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("capability origin must be loopback, got %q", host)
	}
	if parsed.Port() == "" {
		return fmt.Errorf("capability origin must declare an explicit port")
	}
	return nil
}
