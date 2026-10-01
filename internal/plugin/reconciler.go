// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

// Reconciler provides durable event catch-up for plugin processes after restarts or network drops.
type Reconciler struct {
	database *store.Store
	mu       sync.RWMutex
	offsets  map[string]int64
}

// NewReconciler creates a reconciler bound to the durable store.
func NewReconciler(database *store.Store) *Reconciler {
	return &Reconciler{
		database: database,
		offsets:  make(map[string]int64),
	}
}

// GetOffset returns the recorded high-water mark event ID for a plugin.
func (r *Reconciler) GetOffset(pluginID string) int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.offsets[pluginID]
}

// SetOffset manually sets or updates the high-water mark event ID for a plugin.
func (r *Reconciler) SetOffset(pluginID string, eventID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if eventID > r.offsets[pluginID] {
		r.offsets[pluginID] = eventID
	}
}

// CatchUp queries unhandled events since afterEventID and delivers them in order to handler.
// Returns the updated high-water mark event ID, the count of processed events, and any error encountered.
func (r *Reconciler) CatchUp(ctx context.Context, pluginID, projectID string, afterEventID int64, handler func(event model.ProjectEvent) error) (int64, int, error) {
	if r.database == nil {
		return afterEventID, 0, errors.New("nil database store")
	}
	if handler == nil {
		return afterEventID, 0, errors.New("nil event handler")
	}

	currentID := afterEventID
	processedCount := 0

	for {
		select {
		case <-ctx.Done():
			return currentID, processedCount, ctx.Err()
		default:
		}

		events, err := r.database.ListEventsAfterID(ctx, projectID, currentID, 100)
		if err != nil {
			return currentID, processedCount, fmt.Errorf("reconciler catch-up fetch failed: %w", err)
		}

		if len(events) == 0 {
			break
		}

		for _, ev := range events {
			if err := handler(ev); err != nil {
				return currentID, processedCount, fmt.Errorf("reconciler handler error on event %d: %w", ev.EventID, err)
			}
			currentID = ev.EventID
			processedCount++
			r.SetOffset(pluginID, currentID)
		}

		if len(events) < 100 {
			break
		}
	}

	return currentID, processedCount, nil
}

// ReconcileLoop runs catch-up continuously in a background routine until context is cancelled.
func (r *Reconciler) ReconcileLoop(ctx context.Context, pluginID, projectID string, pollInterval time.Duration, handler func(event model.ProjectEvent) error, processedCount *atomic.Int64) error {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}

	lastID := r.GetOffset(pluginID)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		newID, count, err := r.CatchUp(ctx, pluginID, projectID, lastID, handler)
		if err != nil && !errors.Is(err, context.Canceled) {
			// Failures log and return error
			return err
		}

		lastID = newID
		if processedCount != nil && count > 0 {
			processedCount.Add(int64(count))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
