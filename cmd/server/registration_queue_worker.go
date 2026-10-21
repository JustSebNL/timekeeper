package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/JustSebNL/timekeeper/internal/store"
)

func processRegistrationQueueOnce(ctx context.Context, database *store.Store, logger *slog.Logger) error {
	items, err := database.ProcessPendingRegistrationQueue(ctx, 100)
	for _, item := range items {
		if logger != nil {
			logger.Info("registration offer processed", slog.String("queue_id", item.QueueID), slog.String("project_id", item.ProjectID), slog.String("kind", item.Kind), slog.String("status", item.Status))
		}
	}
	return err
}

// startRegistrationQueueWorker recovers queued offers on startup and retries
// transient storage failures while the Core service is running.
func startRegistrationQueueWorker(ctx context.Context, database *store.Store, logger *slog.Logger) func() {
	workerContext, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		drain := func() {
			if err := processRegistrationQueueOnce(workerContext, database, logger); err != nil && logger != nil {
				logger.Warn("registration queue processing deferred", slog.Any("err", err))
			}
		}
		drain()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-workerContext.Done():
				return
			case <-ticker.C:
				drain()
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}
