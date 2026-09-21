// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package bridge

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ControlTunnel is the stdio JSON tunnel between the boxes. One line
// per message, so framing is trivial and a half-written line can never
// be mistaken for a valid message.
//
// It is safe for concurrent use: reads are serialised by rmu, writes
// by wmu.
type ControlTunnel struct {
	in  *bufio.Reader
	out io.Writer

	rmu sync.Mutex
	wmu sync.Mutex
}

// NewControlTunnel wraps a stream pair — typically a child process's
// stdout/stdin.
func NewControlTunnel(in io.Reader, out io.Writer) *ControlTunnel {
	return &ControlTunnel{in: bufio.NewReader(in), out: out}
}

// Send writes one envelope as a single line.
func (t *ControlTunnel) Send(env Envelope) error {
	t.wmu.Lock()
	defer t.wmu.Unlock()

	line, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("bridge: encode envelope: %w", err)
	}
	if _, err := t.out.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("bridge: write envelope: %w", err)
	}
	return nil
}

// Receive reads the next envelope. io.EOF is translated to ErrClosed
// so callers can distinguish "the box went away" from a real error.
func (t *ControlTunnel) Receive() (Envelope, error) {
	t.rmu.Lock()
	defer t.rmu.Unlock()

	line, err := t.in.ReadBytes('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return Envelope{}, ErrClosed
		}
		return Envelope{}, fmt.Errorf("bridge: read envelope: %w", err)
	}
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Envelope{}, fmt.Errorf("bridge: decode envelope: %w", err)
	}
	return env, nil
}

// SendError reports a refusal back across the tunnel.
func (t *ControlTunnel) SendError(id, code, detail string) error {
	return SendError(t, id, code, detail)
}

// SendError is the package-level form, so a caller holding only an
// interface or a plain writer can still report a refusal.
func SendError(t *ControlTunnel, id, code, detail string) error {
	return t.Send(Envelope{
		ID:     id,
		Method: MethodError,
		Params: map[string]any{
			"code":   code,
			"detail": detail,
		},
	})
}

// ErrorFromEnvelope extracts an ErrorPayload from an error envelope.
func ErrorFromEnvelope(env Envelope) ErrorPayload {
	payload := ErrorPayload{}
	if env.Params == nil {
		return payload
	}
	if code, ok := env.Params["code"].(string); ok {
		payload.Code = code
	}
	if detail, ok := env.Params["detail"].(string); ok {
		payload.Detail = detail
	}
	return payload
}
