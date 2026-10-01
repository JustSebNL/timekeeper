// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package plugin_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/plugin"
)

func TestSubprocessLauncherLifecycle(t *testing.T) {
	launcher := plugin.NewLauncher()

	// Launch a lightweight helper process (go executable printing hello or sleeping)
	execPath, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to resolve executable path: %v", err)
	}

	config := plugin.LaunchConfig{
		ID:       "test-plugin-01",
		Name:     "Test Plugin",
		ExecPath: execPath,
		Args:     []string{"version"},
		Env:      map[string]string{"TEST_ENV": "active"},
		CoreURL:  "http://127.0.0.1:1618",
	}

	proc, err := launcher.Launch(context.Background(), config)
	if err != nil {
		t.Fatalf("failed to launch plugin process: %v", err)
	}

	if proc.ID != "test-plugin-01" {
		t.Fatalf("unexpected process ID: %s", proc.ID)
	}

	// Verify status lookup
	status, err := launcher.Status("test-plugin-01")
	if err != nil {
		t.Fatalf("failed to retrieve process status: %v", err)
	}

	if status.ID != proc.ID {
		t.Fatalf("status ID mismatch: got %s, want %s", status.ID, proc.ID)
	}

	// Verify process list
	list := launcher.List()
	if len(list) != 1 || list[0].ID != "test-plugin-01" {
		t.Fatalf("unexpected list output: %+v", list)
	}

	// Stop process
	if err := launcher.Stop("test-plugin-01"); err != nil {
		t.Fatalf("failed to stop process: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	finalStatus, err := launcher.Status("test-plugin-01")
	if err != nil {
		t.Fatalf("failed to get final status: %v", err)
	}

	if finalStatus.State == plugin.StateRunning {
		t.Fatalf("process expected to be stopped/finished, but state is %s", finalStatus.State)
	}
}

func TestSubprocessLauncherRequiresIDAndPath(t *testing.T) {
	launcher := plugin.NewLauncher()

	_, err := launcher.Launch(context.Background(), plugin.LaunchConfig{})
	if err == nil {
		t.Fatal("expected error launching with empty config, got nil")
	}

	_, err = launcher.Launch(context.Background(), plugin.LaunchConfig{ID: "no-path"})
	if err == nil {
		t.Fatal("expected error launching without exec path, got nil")
	}
}
