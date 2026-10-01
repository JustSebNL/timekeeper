// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
)

func TestMessageCLIHelpDoesNotResolveAProject(t *testing.T) {
	var out, errOut strings.Builder
	if code := cli.Run([]string{"msg", "--help"}, &out, &errOut, "http://127.0.0.1:1"); code != 0 {
		t.Fatalf("help code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Subcommands:") || !strings.Contains(out.String(), "show <message-id>") {
		t.Fatalf("help output=%q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("help wrote stderr=%q", errOut.String())
	}
}

func TestMessageCLIUsesCompactReferencesForReplyEditAndThread(t *testing.T) {
	var requests []struct{ method, path, body string }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, struct{ method, path, body string }{r.Method, r.URL.RequestURI(), string(body)})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message_id":12,"ref":"M-12","project_id":"P-1","kind":"answer","body":"reply","revision":1}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/messages/12"):
			_, _ = w.Write([]byte(`{"message_id":12,"ref":"M-12","project_id":"P-1","kind":"note","author":"agent","body":"old","revision":1}`))
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/messages/12"):
			_, _ = w.Write([]byte(`{"message_id":12,"ref":"M-12","project_id":"P-1","kind":"lesson","body":"new","revision":2}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/messages/12/thread"):
			_, _ = w.Write([]byte(`{"items":[{"ref":"M-12","body":"new"}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/messages/12/context"):
			if r.URL.Query().Get("related_limit") != "3" {
				t.Fatalf("context related_limit=%q", r.URL.Query().Get("related_limit"))
			}
			_, _ = w.Write([]byte(`{"anchor":{"message_id":12,"ref":"M-12","body":"anchor"},"thread":[],"related":[],"revisions":[],"thread_truncated":false,"related_truncated":false,"revisions_truncated":false}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages/12/pin"):
			_, _ = w.Write([]byte(`{"message_id":12,"ref":"M-12","body":"anchor","pinned":true,"revision":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out, errOut strings.Builder
	if code := cli.Run([]string{"msg", "P-1", "add", "--reply-to", "M-7", "a", "reply"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("add code=%d err=%s", code, errOut.String())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(requests[0].body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["parent_message_id"] != float64(7) {
		t.Fatalf("reply payload=%#v", payload)
	}

	out.Reset()
	if code := cli.Run([]string{"msg", "P-1", "edit", "M-12", "new"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("edit code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "M-12") || !strings.Contains(requests[2].body, `"body":"new"`) {
		t.Fatalf("edit output=%q requests=%#v", out.String(), requests)
	}

	out.Reset()
	if code := cli.Run([]string{"msg", "P-1", "pin", "M-12"}, &out, &errOut, server.URL); code != 0 || !strings.Contains(out.String(), "M-12") || !strings.Contains(requests[len(requests)-1].body, `"pinned":true`) {
		t.Fatalf("pin code=%d out=%q err=%s requests=%#v", code, out.String(), errOut.String(), requests)
	}
}
