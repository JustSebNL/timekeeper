// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package edgeplugin_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/observer"
	"github.com/JustSebNL/timekeeper/internal/plugin"
	edgeplugin "github.com/JustSebNL/timekeeper/internal/plugin/edgeplugin"
)

// TestRegisteredKind proves the plugin declares the observer kind the
// core registry knows about.
func TestRegisteredKind(t *testing.T) {
	if !plugin.Has(plugin.KindObserver, edgeplugin.ID) {
		t.Fatalf("plugin %q is not registered as an observer", edgeplugin.ID)
	}
	if !plugin.Available(plugin.KindObserver) {
		t.Fatalf("observer kind is not available")
	}
	for _, d := range plugin.Descriptors(plugin.KindObserver) {
		if d.ID == edgeplugin.ID && d.Kind != plugin.KindObserver {
			t.Fatalf("kind = %q, want %q", d.Kind, plugin.KindObserver)
		}
	}
}

// TestBuiltSatisfiesObserver proves the built plugin can be used through
// the core-owned interface without core knowing any journal type.
func TestBuiltSatisfiesObserver(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]string{
		edgeplugin.JournalPathKey: filepath.Join(dir, "events.db"),
	}

	built, err := plugin.Build(plugin.KindObserver, edgeplugin.ID, cfg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	obs, ok := built.(observer.Observer)
	if !ok {
		t.Fatalf("built plugin is %T, want observer.Observer", built)
	}
	defer obs.Close()

	ctx := context.Background()

	st, err := obs.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Installed || !st.Available {
		t.Fatalf("status = %+v, want installed and available", st)
	}

	// Append, then replay, then ack. The full journal contract.
	seq, inserted, err := obs.Append(ctx, "evt-1", []byte(`{"id":"evt-1","kind":"test"}`))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if seq == 0 {
		t.Fatalf("sequence = 0, want non-zero")
	}
	if !inserted {
		t.Fatalf("inserted = false, want true on first append")
	}

	// Idempotency: the same event must not be recorded twice.
	if _, again, err := obs.Append(ctx, "evt-1", []byte(`{"id":"evt-1","kind":"test"}`)); err != nil {
		t.Fatalf("Append duplicate: %v", err)
	} else if again {
		t.Fatalf("inserted = true on duplicate append, want false")
	}

	pending, err := obs.Pending(ctx, 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1 (duplicate must not replay)", len(pending))
	}
	if len(pending[0].Payload) == 0 {
		t.Fatalf("pending record has no payload")
	}

	// Ack, then the queue must drain.
	if _, err := obs.Ack(ctx, []string{"evt-1"}); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	drained, err := obs.Pending(ctx, 10)
	if err != nil {
		t.Fatalf("Pending after ack: %v", err)
	}
	if len(drained) != 0 {
		t.Fatalf("pending after ack = %d, want 0", len(drained))
	}
}

// TestEmptyEventIDRejected guards the invariant that an event without
// an id cannot be journalled: replay would have no way to dedupe it.
func TestEmptyEventIDRejected(t *testing.T) {
	dir := t.TempDir()
	built, err := plugin.Build(plugin.KindObserver, edgeplugin.ID, map[string]string{
		edgeplugin.JournalPathKey: filepath.Join(dir, "events.db"),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	obs := built.(observer.Observer)
	defer obs.Close()

	if _, _, err := obs.Append(context.Background(), "", []byte(`{}`)); err == nil {
		t.Fatalf("Append with empty id succeeded, want error")
	}
}

// TestMalformedPayloadRejected proves a bad payload surfaces an error
// rather than journalling garbage.
func TestMalformedPayloadRejected(t *testing.T) {
	dir := t.TempDir()
	built, err := plugin.Build(plugin.KindObserver, edgeplugin.ID, map[string]string{
		edgeplugin.JournalPathKey: filepath.Join(dir, "events.db"),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	obs := built.(observer.Observer)
	defer obs.Close()

	if _, _, err := obs.Append(context.Background(), "evt-bad", []byte(`not json`)); err == nil {
		t.Fatalf("Append with malformed payload succeeded, want error")
	}
}

// TestDegradesWhenAbsent proves the missing-plugin path is a typed
// error, not a crash: core must run with zero plugins.
func TestDegradesWhenAbsent(t *testing.T) {
	if plugin.Has(plugin.KindObserver, "no.such.plugin") {
		t.Fatalf("Has found an unknown plugin, want false")
	}
	if _, err := plugin.Build(plugin.KindObserver, "no.such.plugin", nil); err == nil {
		t.Fatalf("Build of unknown plugin succeeded, want error")
	}
	if observer.ErrNotInstalled == nil {
		t.Fatalf("ErrNotInstalled must be defined")
	}
}
