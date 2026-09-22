// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package observer defines the capability an event journal plugin
// implements.
//
// Core owns this interface so a journal plugin can be installed without
// editing core packages. The shape mirrors the journal contract: append
// durably, replay what is pending, acknowledge what has been delivered.
package observer

import (
	"context"
	"errors"
)

// ErrNotInstalled is returned when no observer plugin is registered.
//
// It is a typed error rather than a generic failure so callers can
// distinguish "the capability is absent" from "the capability broke".
// Absence is a normal state: core runs with zero plugins.
var ErrNotInstalled = errors.New("observer: not installed")

// Record is one journalled event with its local sequence.
type Record struct {
	Sequence uint64
	// Payload carries the original event. It is intentionally opaque so
	// core never needs to know the journal's event schema.
	Payload []byte
}

// Observer is the capability a journal plugin implements.
//
// Every method may be called concurrently. Implementations must be safe
// for concurrent use.
type Observer interface {
	// Status reports whether the journal is usable.
	Status(context.Context) (Status, error)

	// Append durably records an event and returns its local sequence.
	// Appending the same event twice must be idempotent: inserted is
	// false and no error is returned.
	Append(ctx context.Context, eventID string, payload []byte) (sequence uint64, inserted bool, err error)

	// Pending returns undelivered records in sequence order. Limit is
	// bounded by the implementation; a non-positive limit yields an
	// error rather than an unbounded read.
	Pending(ctx context.Context, limit int) ([]Record, error)

	// Ack marks records delivered. Unknown identifiers are harmless so
	// a replay after reconnect does not duplicate anything.
	Ack(ctx context.Context, eventIDs []string) (int64, error)

	// Close releases the journal. A caller that builds an observer owns
	// it and must close it; without this the SQLite file stays locked
	// and nothing else can open it.
	Close() error
}

// Status describes journal health.
type Status struct {
	Installed bool   `json:"installed"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
}

// NotInstalled builds the degraded status core reports when no observer
// is registered. Degrading is correct: a missing plugin must not stop
// the server.
func NotInstalled() Status {
	return Status{Installed: false, Available: false, Detail: ErrNotInstalled.Error()}
}
