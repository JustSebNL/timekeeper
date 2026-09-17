// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
)

func TestMessageCLIResolvesProjectAliasBeforeCallingBoardAPI(t *testing.T) {
	var messagePath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"project_id":"P-1","project_name":"HSAM","project_alias":"hsam"}]}`))
			return
		}
		messagePath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"query":"queue"}`))
	}))
	defer server.Close()

	var out, errOut strings.Builder
	code := cli.Run([]string{"msg", "hsam", "search", "queue"}, &out, &errOut, server.URL)
	if code != 0 || messagePath != "/api/v1/projects/P-1/messages/search" {
		t.Fatalf("alias message path=%q code=%d out=%q err=%s", messagePath, code, out.String(), errOut.String())
	}
}
