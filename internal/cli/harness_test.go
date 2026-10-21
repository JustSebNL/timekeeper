package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeHarnessFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"harness/SKILL.md":                   "# Core contract\n",
		"harness/manifest.yaml":              "version: 1\nskills:\n  - harness/skills/bug-hunter/SKILL.md\n",
		"harness/skills/bug-hunter/SKILL.md": "---\nname: bug-hunter\ndescription: Use when investigating a suspected bug.\n---\n# Bug hunter\nTrace the bug.\n",
		"docs/HARNESS.md":                    "# First use\n",
		"docs/agent.md":                      "# Agent router\n",
	}
	for relative, content := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestHarnessEntriesListCoreDocsAndManifestSkills(t *testing.T) {
	root := writeHarnessFixture(t)
	items, err := harnessEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"core":         "harness/SKILL.md",
		"manifest":     "harness/manifest.yaml",
		"first-use":    "docs/HARNESS.md",
		"agent-router": "docs/agent.md",
		"bug-hunter":   "harness/skills/bug-hunter/SKILL.md",
	}
	for _, item := range items {
		if path, ok := want[item.ID]; ok {
			if item.Path != path {
				t.Errorf("item %q path=%q want=%q", item.ID, item.Path, path)
			}
			delete(want, item.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing harness items: %#v", want)
	}
}

func TestHarnessShowReturnsSelectedContent(t *testing.T) {
	root := writeHarnessFixture(t)
	items, err := harnessEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := showHarnessItem(root, items, "bug-hunter", &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	if got := out.String(); got != "---\nname: bug-hunter\ndescription: Use when investigating a suspected bug.\n---\n# Bug hunter\nTrace the bug.\n" {
		t.Fatalf("content=%q", got)
	}
}

func TestHarnessShowAcceptsListedPathButRejectsUnlistedPath(t *testing.T) {
	root := writeHarnessFixture(t)
	items, err := harnessEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := showHarnessItem(root, items, "harness/skills/bug-hunter/SKILL.md", &out, &errOut); code != 0 {
		t.Fatalf("listed path code=%d stderr=%q", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := showHarnessItem(root, items, "../../secret.txt", &out, &errOut); code == 0 || !strings.Contains(errOut.String(), "not listed") {
		t.Fatalf("unlisted path code=%d stderr=%q", code, errOut.String())
	}
}

func TestHarnessCommandListsAndShowsManifestItem(t *testing.T) {
	root := writeHarnessFixture(t)
	t.Setenv("TIMEKEEPER_REPO", root)
	var out, errOut bytes.Buffer
	if code := RunTracked([]string{"harness", "list"}, &out, &errOut, "http://127.0.0.1:1"); code != 0 {
		t.Fatalf("list code=%d stderr=%q", code, errOut.String())
	}
	for _, entry := range []string{"bug-hunter	harness/skills/bug-hunter/SKILL.md", "agent-router	docs/agent.md", "Use when investigating a suspected bug.", "http://timekeeper.local/harness.html?item=bug-hunter"} {
		if !strings.Contains(out.String(), entry) {
			t.Fatalf("list output missing %q: %q", entry, out.String())
		}
	}
	if !strings.Contains(out.String(), "http://timekeeper.local/harness.html?item=manifest") {
		t.Fatalf("list output missing YAML URL: %q", out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := RunTracked([]string{"harness", "show", "bug-hunter"}, &out, &errOut, "http://127.0.0.1:1"); code != 0 {
		t.Fatalf("show code=%d stderr=%q", code, errOut.String())
	}
	if got := out.String(); got != "---\nname: bug-hunter\ndescription: Use when investigating a suspected bug.\n---\n# Bug hunter\nTrace the bug.\n" {
		t.Fatalf("show output=%q", got)
	}
}

func TestHarnessCommandRejectsUnlistedItems(t *testing.T) {
	root := writeHarnessFixture(t)
	t.Setenv("TIMEKEEPER_REPO", root)
	var out, errOut bytes.Buffer
	if code := Run([]string{"harness", "show", "../../secret.txt"}, &out, &errOut, ""); code == 0 || !strings.Contains(errOut.String(), "not listed") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
}

func TestHarnessEntriesRejectsManifestPathTraversal(t *testing.T) {
	root := writeHarnessFixture(t)
	manifest := filepath.Join(root, "harness", "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("version: 1\nskills:\n  - harness/skills/../../secret.md\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := harnessEntries(root); err == nil {
		t.Fatal("expected unsafe manifest entry to be rejected")
	}
}
