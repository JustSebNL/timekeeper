// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGitIntegration(t *testing.T) {
	// Create temp dir and init git repo
	tempDir := t.TempDir()
	cwd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(cwd)

	exec.Command("git", "init").Run()
	exec.Command("git", "config", "user.name", "Test").Run()
	exec.Command("git", "config", "user.email", "test@example.com").Run()

	// 1. Write .tkrc
	tkrc := `{"project_id": "P-123", "auto_git_sync": false}`
	os.WriteFile(".tkrc", []byte(tkrc), 0644)

	// Mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "execution-tree") {
			w.Write([]byte(`{"project":{"project_id":"P-123"},"categories":[{"tasks":[{"task":{"task_id":"T-123"},"sprints":[{"sprint_id":"SP-123","status":"Active"}]}]}]}`))
		} else {
			w.Write([]byte(`{}`))
		}
	}))
	defer mockServer.Close()

	// Test git status
	var out, errOut bytes.Buffer
	code := gitCLI([]string{"status"}, &out, &errOut, mockServer.URL)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Untracked files: 1") {
		t.Errorf("expected Untracked files: 1 in status, got %s", out.String())
	}

	// Test commit
	out.Reset()
	errOut.Reset()
	code = gitCommitCLI([]string{"Initial commit", "-a"}, &out, &errOut, mockServer.URL)
	if code != 0 {
		t.Errorf("commit failed: %s", errOut.String())
	}
	if !strings.Contains(out.String(), "Commit") {
		t.Errorf("expected Commit output, got %s", out.String())
	}

	// Test sync (it will fail without remote, but let's check it doesn't panic)
	out.Reset()
	errOut.Reset()
	// Should fail because no origin
	code = gitSyncCLI([]string{}, &out, &errOut, "http://localhost:1618")
	if code == 0 {
		t.Errorf("sync should fail without remote")
	}

	// Test auto-sync logic
	os.Setenv("TIMEKEEPER_AUTO_GIT_SYNC", "false")
	performAutoGitSync("start", "SP-123", &out)

	// Just verify no panic and we tested the basics
}

func TestParseRepoOwnerName(t *testing.T) {
	cases := []struct {
		url   string
		owner string
		repo  string
	}{
		{"git@github.com:JustSebNL/timekeeper.git", "JustSebNL", "timekeeper"},
		{"https://github.com/JustSebNL/timekeeper.git", "JustSebNL", "timekeeper"},
		{"git@gitlab.com:foo/bar.git", "foo", "bar"},
		{"https://gitlab.com/foo/bar", "foo", "bar"},
		{"git@bitbucket.org:baz/qux.git", "baz", "qux"},
		{"https://bitbucket.org/baz/qux.git", "baz", "qux"},
		{"https://dev.azure.com/myorg/myproject/_git/myrepo", "myorg/myproject", "myrepo"},
		{"git@ssh.dev.azure.com:v3/myorg/myproject/myrepo", "myorg/myproject", "myrepo"},
		{"https://gitea.com/owner/repo", "owner", "repo"},
		{"git@gitea.com:owner/repo.git", "owner", "repo"},
	}

	for _, c := range cases {
		o, r := parseRepoOwnerName(c.url)
		if o != c.owner || r != c.repo {
			t.Errorf("parseRepoOwnerName(%q) = %q, %q; expected %q, %q", c.url, o, r, c.owner, c.repo)
		}
	}
}

func TestGetMRURL(t *testing.T) {
	cases := []struct {
		provider string
		url      string
		owner    string
		repo     string
		branch   string
		expected string
	}{
		{"GitHub", "git@github.com:o/r.git", "o", "r", "main", "https://github.com/o/r/pull/new/main"},
		{"GitLab", "git@gitlab.com:o/r.git", "o", "r", "main", "https://gitlab.com/o/r/-/merge_requests/new?merge_request%5Bsource_branch%5D=main"},
		{"Bitbucket", "git@bitbucket.org:o/r.git", "o", "r", "main", "https://bitbucket.org/o/r/pull-requests/new?source=main"},
		{"AzureDevOps", "https://dev.azure.com/o/p/_git/r", "o/p", "r", "main", "https://dev.azure.com/o/p/_git/r/pullrequestcreate?sourceRef=main"},
		{"Gitea", "https://gitea.example.com/o/r", "o", "r", "main", "https://gitea.example.com/o/r/compare/main...main"},
		{"Gitea", "git@gitea.com:o/r.git", "o", "r", "main", "https://gitea.com/o/r/compare/main...main"},
	}

	for _, c := range cases {
		u := getMRURL(c.provider, c.owner, c.repo, c.branch, c.url)
		if u != c.expected {
			t.Errorf("getMRURL(%q, %q, %q, %q, %q) = %q; expected %q", c.provider, c.owner, c.repo, c.branch, c.url, u, c.expected)
		}
	}
}
