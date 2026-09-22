// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package edgeplugin registers the edge event journal as an observer
// with the core plugin registry.
//
// This is the only place that names both core and the journal. Core
// imports this package when the plugin is enabled; it never imports the
// journal directly.
package edgeplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	runtimeevents "github.com/JustSebNL/edge/pkg/events"
	"github.com/JustSebNL/edge/pkg/journal"
	"github.com/JustSebNL/timekeeper/internal/observer"
	"github.com/JustSebNL/timekeeper/internal/plugin"
)

// ID is the stable plugin identifier used by configuration.
const ID = "ai-hub.edge"

// JournalPathKey names the configuration entry holding the SQLite
// journal path.
const JournalPathKey = "journal_path"

func init() {
	plugin.Register(plugin.Descriptor{
		ID:           ID,
		Kind:         plugin.KindObserver,
		Name:         "edge event journal",
		Capabilities: []string{"append", "replay", "ack"},
	}, Build)
}

// Build constructs an observer from configuration.
//
// A failure to build disables the plugin rather than failing startup:
// core must run with zero plugins. The error is returned so the caller
// can log it.
func Build(config map[string]string) (any, error) {
	path := config[JournalPathKey]
	j, err := journal.Open(path)
	if err != nil {
		return nil, err
	}
	return &adapter{journal: j}, nil
}

// adapter presents the journal through the core-owned observer
// interface, so core never touches the journal's own event type.
type adapter struct {
	journal *journal.Journal
}

// ErrEmptyEventID guards against journalling an unidentifiable event:
// without an ID, idempotent replay cannot work.
var ErrEmptyEventID = errors.New("observer: event id is required")

func (a *adapter) Status(context.Context) (observer.Status, error) {
	if a == nil || a.journal == nil {
		return observer.NotInstalled(), nil
	}
	return observer.Status{Installed: true, Available: true}, nil
}

// Append records one event. The payload is a JSON-encoded runtime event
// so core never depends on the journal's schema directly.
func (a *adapter) Append(ctx context.Context, eventID string, payload []byte) (uint64, bool, error) {
	if a == nil || a.journal == nil {
		return 0, false, observer.ErrNotInstalled
	}
	if eventID == "" {
		return 0, false, ErrEmptyEventID
	}
	var evt runtimeevents.Event
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &evt); err != nil {
			return 0, false, fmt.Errorf("observer: event payload: %w", err)
		}
	}
	// The caller's ID wins, so an event can be replayed by the same
	// identifier it was first recorded under.
	evt.ID = eventID
	if evt.Time.IsZero() {
		evt.Time = time.Now().UTC()
	}
	record, inserted, err := a.journal.Append(ctx, evt)
	if err != nil {
		return 0, false, err
	}
	return record.Sequence, inserted, nil
}

func (a *adapter) Pending(ctx context.Context, limit int) ([]observer.Record, error) {
	if a == nil || a.journal == nil {
		return nil, observer.ErrNotInstalled
	}
	records, err := a.journal.Pending(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]observer.Record, 0, len(records))
	for _, r := range records {
		// Payload is re-encoded so core sees bytes, not journal types.
		// An unencodable payload is skipped rather than failing the
		// whole replay: one bad record must not block the queue.
		raw, err := json.Marshal(r.Event)
		if err != nil {
			continue
		}
		out = append(out, observer.Record{Sequence: r.Sequence, Payload: raw})
	}
	return out, nil
}

func (a *adapter) Ack(ctx context.Context, eventIDs []string) (int64, error) {
	if a == nil || a.journal == nil {
		return 0, observer.ErrNotInstalled
	}
	return a.journal.Ack(ctx, eventIDs)
}

func (a *adapter) Close() error {
	if a == nil || a.journal == nil {
		return nil
	}
	return a.journal.Close()
}
