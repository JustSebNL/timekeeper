// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/store"
)

func eventsStream(database *store.Store, scopedProject bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)

		projectID := ""
		if scopedProject {
			projectID = r.PathValue("projectID")
			if projectID != "" {
				resolved, err := database.ResolveProjectID(r.Context(), projectID)
				if err == nil {
					projectID = resolved
				}
			}
		}

		var lastID int64
		if lastHeader := r.Header.Get("Last-Event-ID"); lastHeader != "" {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(lastHeader), 10, 64); err == nil {
				lastID = parsed
			}
		}
		if lastQuery := r.URL.Query().Get("since_id"); lastQuery != "" {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(lastQuery), 10, 64); err == nil {
				lastID = parsed
			}
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		if flusher != nil {
			flusher.Flush()
		}

		pollInterval := 250 * time.Millisecond
		maxPollInterval := 1 * time.Second
		timer := time.NewTimer(pollInterval)
		defer timer.Stop()
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				_, _ = fmt.Fprintln(w, ": heartbeat")
				if flusher != nil {
					flusher.Flush()
				}
			case <-timer.C:
				events, err := database.ListEventsAfterID(ctx, projectID, lastID, 50)
				if err != nil || len(events) == 0 {
					pollInterval = pollInterval * 2
					if pollInterval > maxPollInterval {
						pollInterval = maxPollInterval
					}
					timer.Reset(pollInterval)
					continue
				}
				pollInterval = 100 * time.Millisecond
				for _, evt := range events {
					data, err := json.Marshal(evt)
					if err != nil {
						continue
					}
					_, _ = fmt.Fprintf(w, "id: %d\nevent: project_event\ndata: %s\n\n", evt.EventID, string(data))
					if flusher != nil {
						flusher.Flush()
					}
					if evt.EventID > lastID {
						lastID = evt.EventID
					}
				}
				timer.Reset(pollInterval)
			}
		}
	}
}
