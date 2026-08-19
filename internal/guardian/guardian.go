// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package guardian runs Time Keeper's dependency-free local Pulse Guardian.
package guardian

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

const minInterval = time.Second

type TickResult struct {
	Created          int
	DeliveryAttempts int
	Delivered        int
	DeliveryFailures int
}

// Tick expires material-progress leases and sends every unacknowledged nudge to
// the agent's explicitly registered numeric-loopback Guardian endpoint. Failed
// callbacks stay durable and are retried by a later tick.
func Tick(ctx context.Context, database *store.Store, at time.Time) (TickResult, error) {
	if database == nil {
		return TickResult{}, errors.New("Pulse Guardian store is required")
	}
	if at.IsZero() {
		return TickResult{}, errors.New("Pulse Guardian tick time is required")
	}
	created, err := database.EvaluatePulseGuardianAt(ctx, at)
	if err != nil {
		return TickResult{}, err
	}
	result := TickResult{Created: len(created)}
	deliveries, err := database.ListPendingPulseNudgeDeliveries(ctx)
	if err != nil {
		return TickResult{}, err
	}
	for _, delivery := range deliveries {
		result.DeliveryAttempts++
		err := deliver(ctx, delivery.GuardianURL, delivery.Nudge)
		delivered := err == nil
		if err := database.RecordPulseNudgeDeliveryAttempt(ctx, delivery.Nudge.NudgeID, delivered, at); err != nil {
			return TickResult{}, err
		}
		if delivered {
			result.Delivered++
		} else {
			result.DeliveryFailures++
		}
	}
	return result, nil
}

// Run owns the periodic Guardian loop. It performs an immediate tick to surface
// already-stale work, then remains outside the agent's work loop until context
// cancellation. It has no dependency on any agent framework or other repo.
func Run(ctx context.Context, database *store.Store, interval time.Duration, report func(error)) {
	if interval < minInterval {
		if report != nil {
			report(fmt.Errorf("Pulse Guardian interval must be at least %s", minInterval))
		}
		return
	}
	tick := func() {
		if _, err := Tick(ctx, database, time.Now().UTC().Round(0)); err != nil && report != nil {
			report(err)
		}
	}
	tick()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}

func deliver(ctx context.Context, guardianURL string, nudge model.PulseNudge) error {
	signal := model.PulseGuardianSignal{
		Format: "timekeeper-pulse-guardian/v1",
		Action: "recover_attention",
		Nudge:  nudge,
	}
	body, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("encode Pulse Guardian nudge: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, guardianURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build Pulse Guardian callback request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("send Pulse Guardian callback: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Pulse Guardian callback returned %s", response.Status)
	}
	if response.Header.Get("X-Timekeeper-Pulse-Accepted") != "v1" {
		return errors.New("Pulse Guardian callback did not confirm nudge acceptance")
	}
	return nil
}
