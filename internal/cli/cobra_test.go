// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package cli_test

import (
	"bytes"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/cli"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestCobraAndViperEngineExecution(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "cobra_test.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	server := httptest.NewServer(api.New(s))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := cli.ExecuteCobra([]string{"list"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. err: %s", code, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	code = cli.ExecuteCobra([]string{"--help"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("expected exit code 0 for --help, got %d", code)
	}

	rootCmd := cli.BuildCobraTree(&out, &errOut, server.URL)
	if rootCmd == nil || rootCmd.Use != "tk" {
		t.Fatalf("unexpected root command: %+v", rootCmd)
	}

	hasList := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "list" {
			hasList = true
			break
		}
	}
	if !hasList {
		t.Fatal("expected 'list' subcommand in Cobra tree")
	}
}
