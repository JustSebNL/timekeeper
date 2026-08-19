// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
)

func TestRunSprintAttemptPostsReasonAndReportsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sprints/SP-10004/retrieval-attempts" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"reason":"provider gave no usable result"}` {
			t.Fatalf("body=%s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"attempt_number":4,"timed_out":true}`))
	}))
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"sp", "attempt", "SP-10004", "provider gave no usable result"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if got := out.String(); got != "SP-10004\tattempt=4\tTimedOut\n" {
		t.Fatalf("output=%q", got)
	}
}
