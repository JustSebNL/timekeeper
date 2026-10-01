// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/observer"
)

// fakeObserver records appended events so tests can assert what the
// middleware actually journalled.
type fakeObserver struct {
	mu     sync.Mutex
	events []recordedEvent
}

type recordedEvent struct {
	ID      string
	Payload []byte
}

func (f *fakeObserver) Status(context.Context) (observer.Status, error) {
	return observer.Status{Installed: true, Available: true}, nil
}

func (f *fakeObserver) Append(_ context.Context, id string, payload []byte) (uint64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, recordedEvent{ID: id, Payload: payload})
	return uint64(len(f.events)), true, nil
}

func (f *fakeObserver) Pending(context.Context, int) ([]observer.Record, error) {
	return nil, nil
}

func (f *fakeObserver) Ack(context.Context, []string) (int64, error) { return 0, nil }
func (f *fakeObserver) Close() error                                 { return nil }

func (f *fakeObserver) snapshot() []recordedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedEvent, len(f.events))
	copy(out, f.events)
	return out
}

// waitForEvent polls because the middleware appends asynchronously: a
// journal write must never block the caller's response.
func waitForEvent(t *testing.T, f *fakeObserver) []recordedEvent {
	t.Helper()
	for i := 0; i < 100; i++ {
		if got := f.snapshot(); len(got) > 0 {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	return nil
}

// TestObserveJournalsRequest proves the seam actually emits: a request
// through the middleware produces a journalled event.
func TestObserveJournalsRequest(t *testing.T) {
	obs := &fakeObserver{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	srv := httptest.NewServer(Observe(obs)(inner))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/projects")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	events := waitForEvent(t, obs)
	if len(events) == 0 {
		t.Fatalf("no event was journalled")
	}

	var payload struct {
		Kind    string `json:"kind"`
		Method  string `json:"method"`
		Path    string `json:"path"`
		Status  int    `json:"status"`
		Elapsed int64  `json:"elapsed"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("payload %q: %v", events[0].Payload, err)
	}
	if payload.Kind != "http_request" {
		t.Fatalf("kind = %q, want http_request", payload.Kind)
	}
	if payload.Method != "GET" {
		t.Fatalf("method = %q, want GET", payload.Method)
	}
	if payload.Status != http.StatusCreated {
		t.Fatalf("status = %d, want %d", payload.Status, http.StatusCreated)
	}
}

// TestObserveRedactsIdentifiers is the important one: entity ids must
// never reach the journal unredacted.
func TestObserveRedactsIdentifiers(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/api/v1/projects", "/api/v1/projects"},
		{"/api/v1/projects/P-10289", "/api/v1/projects/{id}"},
		{"/api/v1/projects/P-10289/tasks/T-10359", "/api/v1/projects/{id}/tasks/{id}"},
		{"/api/v1/projects/P-10289/sprints/SP-10363", "/api/v1/projects/{id}/sprints/{id}"},
		{"/api/v1/projects/P-10289/messages/M-244", "/api/v1/projects/{id}/messages/{id}"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := redactPath(tc.path); got != tc.want {
				t.Fatalf("redactPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestObserveNilObserverDisables proves the capability stays optional:
// with no observer, requests still work and nothing is journalled.
func TestObserveNilObserverDisables(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(Observe(nil)(inner))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestObserveDoesNotBlockResponse proves journalling is off the critical
// path. A stalled journal must not delay the response.
func TestObserveDoesNotBlockResponse(t *testing.T) {
	slow := &slowObserver{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(Observe(slow)(inner))
	defer srv.Close()

	done := make(chan int, 1)
	go func() {
		resp, err := http.Get(srv.URL + "/health")
		if err != nil {
			done <- 0
			return
		}
		defer resp.Body.Close()
		done <- resp.StatusCode
	}()

	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("status = %d, want %d", code, http.StatusOK)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("response blocked by journalling")
	}
}

// slowObserver blocks on append to prove the middleware does not wait.
type slowObserver struct{}

func (s *slowObserver) Status(context.Context) (observer.Status, error) {
	return observer.Status{Installed: true, Available: true}, nil
}
func (s *slowObserver) Append(context.Context, string, []byte) (uint64, bool, error) {
	select {} // never returns
}
func (s *slowObserver) Pending(context.Context, int) ([]observer.Record, error) {
	return nil, nil
}
func (s *slowObserver) Ack(context.Context, []string) (int64, error) { return 0, nil }
func (s *slowObserver) Close() error                                 { return nil }
