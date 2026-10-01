// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package cli_test

import (
	"bytes"
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/cli"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestCLICommandHandoff(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "CLI Handoff Project", Alias: "cli-handoff"})
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(api.New(database))
	t.Cleanup(server.Close)

	var out, errOut bytes.Buffer

	// Test 1: Markdown CLI output by default
	code := cli.Run([]string{"handoff", project.ProjectID}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("expected status 0, got %d. errOut=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "# Agent Handoff Snapshot:") {
		t.Errorf("expected Markdown header in CLI output, got:\n%s", out.String())
	}

	// Test 2: JSON CLI output with --json flag
	out.Reset()
	errOut.Reset()
	codeJSON := cli.Run([]string{"handoff", project.ProjectID, "--json"}, &out, &errOut, server.URL)
	if codeJSON != 0 {
		t.Fatalf("expected status 0 for --json, got %d. errOut=%s", codeJSON, errOut.String())
	}
	if !strings.Contains(out.String(), `"project"`) {
		t.Errorf("expected JSON output, got:\n%s", out.String())
	}
}
