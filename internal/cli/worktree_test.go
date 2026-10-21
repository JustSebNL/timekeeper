// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWorktreePorcelain(t *testing.T) {
	output := `worktree /path/to/repo
HEAD 1234567890abcdef
branch refs/heads/main

worktree /path/to/repo/.worktrees/sprint-SP-123
HEAD abcdef1234567890
branch refs/heads/sprint/SP-123
`
	wts := parseWorktreePorcelain(output)
	if len(wts) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(wts))
	}
	if wts[0].Path != "/path/to/repo" {
		t.Errorf("wts[0].Path = %s", wts[0].Path)
	}
	if wts[1].Branch != "sprint/SP-123" {
		t.Errorf("wts[1].Branch = %s", wts[1].Branch)
	}
}

func setupGitRepo(t *testing.T) string {
	dir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = dir
	cmd.Run()

	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = dir
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = dir
	cmd.Run()

	return dir
}

func TestWorktreeCommands(t *testing.T) {
	repo := setupGitRepo(t)

	origDir, _ := os.Getwd()
	os.Chdir(repo)
	defer os.Chdir(origDir)

	os.WriteFile(filepath.Join(repo, ".env"), []byte("FOO=bar"), 0644)

	var out, errOut bytes.Buffer

	// Test wt new
	code := wtNew([]string{"SP-123"}, &out, &errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("wtNew failed: %s", errOut.String())
	}

	expWtPath := strings.TrimSpace(out.String())
	if !strings.Contains(expWtPath, "sprint-SP-123") {
		t.Errorf("expected expWtPath to contain sprint-SP-123, got %s", expWtPath)
	}

	if _, err := os.Stat(filepath.Join(expWtPath, ".env")); err != nil {
		t.Errorf("expected .env to be copied: %v", err)
	}

	// Test wt list
	out.Reset()
	errOut.Reset()
	code = wtList([]string{}, &out, &errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("wtList failed: %s", errOut.String())
	}
	if !strings.Contains(out.String(), "sprint-SP-123") {
		t.Errorf("wtList output missing new worktree: %s", out.String())
	}

	// Test wt path
	out.Reset()
	errOut.Reset()
	code = wtPath([]string{"SP-123"}, &out, &errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("wtPath failed: %s", errOut.String())
	}
	if filepath.Clean(strings.TrimSpace(out.String())) != filepath.Clean(expWtPath) {
		t.Errorf("wtPath output mismatch: expected %s, got %s", expWtPath, out.String())
	}

	// Test wt remove
	out.Reset()
	errOut.Reset()
	code = wtRemove([]string{"SP-123", "--force"}, &out, &errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("wtRemove failed: %s", errOut.String())
	}

	if _, err := os.Stat(expWtPath); !os.IsNotExist(err) {
		t.Errorf("expected worktree path to be removed")
	}

	// Test wt prune
	out.Reset()
	errOut.Reset()
	code = wtPrune([]string{}, &out, &errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("wtPrune failed: %s", errOut.String())
	}
}
