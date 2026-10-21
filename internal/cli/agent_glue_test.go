package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentBootstrap(t *testing.T) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	tmpDir := t.TempDir()

	// Create mock files
	files := []string{
		filepath.Join(tmpDir, "AGENTS.md"),
		filepath.Join(tmpDir, ".cursorrules"),
		filepath.Join(tmpDir, ".windsurfrules"),
		filepath.Join(tmpDir, "CLAUDE.md"),
		filepath.Join(tmpDir, ".gemini", "rules.md"),
	}

	for _, file := range files {
		os.MkdirAll(filepath.Dir(file), 0755)
		os.WriteFile(file, []byte("existing rule"), 0644)
	}

	// Change dir to tmp
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	code := agentBootstrap(out, errOut)
	if code != 0 {
		t.Fatalf("agentBootstrap failed: %v", code)
	}

	for _, file := range files {
		content, _ := os.ReadFile(file)
		if !strings.Contains(string(content), "tk doctor") {
			t.Errorf("File %s does not contain tk doctor", file)
		}
	}
}

func TestLeaseCommand(t *testing.T) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Test claim
	code := leaseCommand([]string{"claim", "task-123"}, out, errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("lease claim failed: %d", code)
	}

	// Test list
	out.Reset()
	code = leaseCommand([]string{"list"}, out, errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("lease list failed: %d", code)
	}
	if !strings.Contains(out.String(), "task-123") {
		t.Errorf("lease list output missing target: %s", out.String())
	}

	// Test release
	code = leaseCommand([]string{"release", "task-123"}, out, errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("lease release failed: %d", code)
	}
}

func TestStoreConnectorsCommands(t *testing.T) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	code := storeConnectors([]string{"install", "jira-plugin"}, out, errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("store install failed: %d", code)
	}

	out.Reset()
	code = storeConnectors([]string{"list"}, out, errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("store list failed: %d", code)
	}
	if !strings.Contains(out.String(), "jira-plugin") {
		t.Errorf("store list missing plugin")
	}

	code = connectCommand([]string{"jira-plugin"}, out, errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("connect command failed: %d", code)
	}

	code = connectorCreateCommand([]string{"my-connector"}, out, errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("connector create failed: %d", code)
	}

	code = storeConnectors([]string{"pack", "my-connector"}, out, errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("store pack failed: %d", code)
	}

	code = storeConnectors([]string{"publish", "my-connector.tkp"}, out, errOut, "http://localhost")
	if code != 0 {
		t.Fatalf("store publish failed: %d", code)
	}
}
