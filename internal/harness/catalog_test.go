package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCatalogFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"harness/SKILL.md":                   "# Core\n",
		"harness/manifest.yaml":              "version: 1\nskills:\n  - harness/skills/bug-hunter/SKILL.md\n",
		"harness/skills/bug-hunter/SKILL.md": "---\nname: bug-hunter\ndescription: Use to investigate a bug.\n---\n# Bug hunter\n",
		"docs/HARNESS.md":                    "# First use\n",
		"docs/agent.md":                      "# Router\n",
	}
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRootAtFindsInstalledBundleFromBinaryDirectory(t *testing.T) {
	root := writeCatalogFixture(t)
	binaryDir := filepath.Join(root, ".timekeeper", "app", "bin")
	if err := os.MkdirAll(binaryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := rootAt(binaryDir)
	if !ok || got != root {
		t.Fatalf("rootAt(%q)=(%q,%t), want %q", binaryDir, got, ok, root)
	}
}

func TestQueryIdentifierSupportsNakedAndNamedHarnessLinks(t *testing.T) {
	cases := map[string]string{
		"bug-hunter.html":       "bug-hunter",
		"bug-hunter.md":         "bug-hunter",
		"harness=bug-hunter":    "bug-hunter",
		"harness=bug-hunter.md": "bug-hunter",
		"manifest.yaml":         "manifest",
		"harness=manifest.yaml": "manifest",
	}
	for query, want := range cases {
		got, ok := QueryIdentifier(query)
		if !ok || got != want {
			t.Errorf("QueryIdentifier(%q)=(%q,%t), want (%q,true)", query, got, ok, want)
		}
	}
	for _, query := range []string{"", "tab=projects", "harness=", "bug-hunter.md&x=1"} {
		if _, ok := QueryIdentifier(query); ok {
			t.Errorf("QueryIdentifier(%q) unexpectedly recognized a harness link", query)
		}
	}
}

func TestEntriesAndLookupReadOnlyRegisteredHarnessItems(t *testing.T) {
	root := writeCatalogFixture(t)
	items, err := Entries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Fatalf("Entries returned %d items, want 5: %#v", len(items), items)
	}
	item, content, err := Lookup(root, "bug-hunter")
	if err != nil {
		t.Fatal(err)
	}
	if item.Path != "harness/skills/bug-hunter/SKILL.md" || !strings.Contains(item.Description, "Use to investigate a bug") || !strings.Contains(string(content), "# Bug hunter") {
		t.Fatalf("lookup item=%#v content=%q", item, content)
	}
	if _, _, err := Lookup(root, "../../secret.md"); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("traversal lookup error=%v, want not-listed refusal", err)
	}
}

func TestEntriesRejectsManifestTraversal(t *testing.T) {
	root := writeCatalogFixture(t)
	path := filepath.Join(root, "harness", "manifest.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nskills:\n  - harness/skills/../../secret.md\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Entries(root); err == nil {
		t.Fatal("expected traversal path in manifest to be rejected")
	}
}

func TestLookupRejectsSymlinkEscape(t *testing.T) {
	root := writeCatalogFixture(t)
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "harness", "skills", "bug-hunter", "SKILL.md")
	if err := os.Remove(inside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, inside); err != nil {
		t.Skipf("symlink unavailable in this environment: %v", err)
	}
	if _, _, err := Lookup(root, "bug-hunter"); err == nil || !strings.Contains(err.Error(), "unsafe harness path") {
		t.Fatalf("symlink lookup error=%v, want escape rejection", err)
	}
}
