// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFormatAPIErrorIncludesStructuredReasonAndRequestID(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "invalid_project_message", "message": "invalid message kind " + string(rune(34)) + "evidence" + string(rune(34))}})
	got := formatAPIError("HTTP 400 Bad Request", body, "rid-123")
	for _, want := range []string{
		"HTTP 400 Bad Request",
		"invalid_project_message",
		`invalid message kind "evidence"`,
		"request_id=rid-123",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted error %q does not contain %q", got, want)
		}
	}
}

func TestFormatConnectionFailureIncludesOperationEndpointAndCause(t *testing.T) {
	cause := errors.New("dial tcp 127.0.0.1:1618: connect: connection refused")
	got := formatConnectionFailure("POST", "http://127.0.0.1:1618/api/v1/projects/P-1/messages", cause)
	for _, want := range []string{
		"POST",
		"http://127.0.0.1:1618/api/v1/projects/P-1/messages",
		"connection refused",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted connection failure %q does not contain %q", got, want)
		}
	}
}

func TestDialTimeKeeperLocalMapsFriendlyAliasesToLoopback(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			close(accepted)
			_ = conn.Close()
		}
	}()

	_, port, _ := net.SplitHostPort(listener.Addr().String())
	conn, err := dialTimeKeeperLocal(context.Background(), "tcp", net.JoinHostPort("timekeeper.local", port))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("friendly alias did not reach loopback listener")
	}
}

func TestDiagnosticTransportAddsClientAgentCommandAndRequestIdentity(t *testing.T) {
	setActiveCommand([]string{"msg", "add", "sensitive-body-must-not-be-sent"})
	defer setActiveCommand(nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, want := range map[string]string{
			"X-TimeKeeper-Client":  "non-empty",
			"User-Agent":           "tk/",
			"X-TimeKeeper-Command": "msg add",
			"X-Request-ID":         "tk-",
		} {
			got := r.Header.Get(key)
			if want == "non-empty" && got == "" {
				t.Errorf("%s was empty", key)
			} else if want != "non-empty" && !strings.HasPrefix(got, want) {
				t.Errorf("%s = %q, want prefix %q", key, got, want)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	response, err := newHTTPClient(2 * time.Second).Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
}

func TestDiagnosticTransportConnectionErrorNamesCulprit(t *testing.T) {
	setActiveCommand([]string{"sp", "next"})
	defer setActiveCommand(nil)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL + "/api/v1/sprints/next"
	server.Close()

	_, err := newHTTPClient(2 * time.Second).Get(endpoint)
	if err == nil {
		t.Fatal("expected connection failure")
	}
	for _, want := range []string{"connection failed", "request_id=tk-", "client_version=", "command=sp next"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}
