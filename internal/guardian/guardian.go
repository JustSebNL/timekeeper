// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package guardian runs TimeKeeper's dependency-free local Pulse Guardian.
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
	"github.com/JustSebNL/timekeeper/internal/ntfy"
	"github.com/JustSebNL/timekeeper/internal/store"
)

const minInterval = time.Second

// NtfyConfig holds the optional ntfy publisher configuration for guardian wake-up.
type NtfyConfig struct {
	// ServerURL is the ntfy server address (e.g. https://ntfy.sh).
	ServerURL string
	// Topic is the agent wake-up topic.
	Topic string
	// Token is the optional authentication token.
	Token string
}

type TickResult struct {
	Created          int
	DeliveryAttempts int
	Delivered        int
	DeliveryFailures int
}

// Tick expires material-progress leases and sends every unacknowledged nudge to
// the agent's explicitly registered numeric-loopback Guardian endpoint. Failed
// callbacks stay durable and are retried by a later tick.
// If ntfyConfig is non-nil, also publishes a wake message via ntfy for each
// pending nudge (the agent-side ntfy client runs `tk guardian wake`).
func Tick(ctx context.Context, database *store.Store, at time.Time, ntfyConfig *NtfyConfig) (TickResult, error) {
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

	// Publish wake messages via ntfy for newly created nudges.
	if ntfyConfig != nil && ntfyConfig.ServerURL != "" && ntfyConfig.Topic != "" {
		publisher, err := ntfy.NewPublisher(ntfy.Config{
			ServerURL: ntfyConfig.ServerURL,
			Topic:      ntfyConfig.Topic,
			Token:      ntfyConfig.Token,
		})
		if err == nil {
			for _, nudge := range created {
				if err := publisher.WakeAgent(ctx, nudge.NudgeID, nudge.AgentID); err != nil {
					// ntfy delivery failure is not fatal — the nudge stays pending
					// and the HTTP fallback will still deliver it.
					result.DeliveryFailures++
				}
			}
			publisher.Close()
		}
	}

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
// If ntfyConfig is non-nil, the guardian also publishes wake messages via ntfy
// for each newly created nudge (the agent-side ntfy client runs `tk guardian wake`).
func Run(ctx context.Context, database *store.Store, interval time.Duration, ntfyConfig *NtfyConfig, report func(error)) {
	if interval < minInterval {
		if report != nil {
			report(fmt.Errorf("Pulse Guardian interval must be at least %s", minInterval))
		}
		return
	}
	tick := func() {
		if _, err := Tick(ctx, database, time.Now().UTC().Round(0), ntfyConfig); err != nil && report != nil {
			report(err)
		}
	}
	tick()

	// Wake on the soonest stored lease expiry rather than a fixed global
	// timetable. A 30-second lease must be noticed after 30 seconds, not
	// whenever the next five-minute tick happens to land.
	wait := func() time.Duration {
		// Compare in UTC: the stored deadline is UTC and a local-time
		// comparison would put an expired lease in the future.
		deadline, ok, err := database.NextPulseDeadline(ctx, time.Now().UTC())
		if err != nil || !ok {
			return interval
		}
		// The stored deadline already includes the lease's grace
		// period, so no extra margin is added here.
		d := time.Until(deadline)
		if d < minInterval {
			return minInterval
		}
		if d > interval {
			return interval
		}
		return d
	}

	timer := time.NewTimer(wait())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			tick()
			timer.Reset(wait())
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
