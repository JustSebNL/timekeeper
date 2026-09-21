package storeclient

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type entry struct {
	name    string
	content string
}

// buildTkp writes a .tkp-shaped archive, including the "./" directory
// entry that a `tar -C dir .` invocation produces.
func buildTkp(t *testing.T, entries []entry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	writer := tar.NewWriter(gz)

	if err := writer.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatalf("write dir header: %v", err)
	}
	for _, e := range entries {
		if err := writer.WriteHeader(&tar.Header{Name: e.name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(e.content))}); err != nil {
			t.Fatalf("write header %s: %v", e.name, err)
		}
		if _, err := writer.Write([]byte(e.content)); err != nil {
			t.Fatalf("write %s: %v", e.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	path := filepath.Join(t.TempDir(), "package.tkp")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return path
}

func checksumsFor(entries []entry) string {
	sums := map[string]string{}
	for _, e := range entries {
		sum := sha256.Sum256([]byte(e.content))
		sums[strings.TrimPrefix(e.name, "./")] = hex.EncodeToString(sum[:])
	}
	encoded, _ := json.Marshal(sums)
	return string(encoded)
}

func validEntries() []entry {
	return []entry{
		{name: "./manifest.json", content: `{"id":"extension.test","version":"1.0.0","permissions":["projects.read"]}`},
		{name: "./extension.js", content: "console.log('hi');\n"},
	}
}

func TestExtractPackageUnpacksAndReadsTheManifest(t *testing.T) {
	entries := validEntries()
	entries = append(entries, entry{name: "./checksums.json", content: checksumsFor(entries)})
	archive := buildTkp(t, entries)

	dest := filepath.Join(t.TempDir(), "unpacked")
	manifest, err := ExtractPackage(archive, dest)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if manifest.ID != "extension.test" || manifest.Version != "1.0.0" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if len(manifest.Permissions) != 1 || manifest.Permissions[0] != "projects.read" {
		t.Fatalf("unexpected permissions: %v", manifest.Permissions)
	}
	if _, err := os.Stat(filepath.Join(dest, "extension.js")); err != nil {
		t.Fatalf("extension.js was not extracted: %v", err)
	}
}

func TestExtractPackageRejectsAPathThatEscapes(t *testing.T) {
	entries := validEntries()
	entries = append(entries, entry{name: "../evil.js", content: "bad"})
	entries = append(entries, entry{name: "./checksums.json", content: checksumsFor(entries)})
	archive := buildTkp(t, entries)

	if _, err := ExtractPackage(archive, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected a traversal entry to be refused")
	}
}

func TestExtractPackageRejectsAChecksumMismatch(t *testing.T) {
	entries := validEntries()
	entries = append(entries, entry{name: "./checksums.json", content: `{"extension.js":"deadbeef"}`})
	archive := buildTkp(t, entries)

	if _, err := ExtractPackage(archive, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected a checksum mismatch to be refused")
	}
}

func TestExtractPackageRequiresAManifest(t *testing.T) {
	entries := []entry{{name: "./extension.js", content: "x"}}
	entries = append(entries, entry{name: "./checksums.json", content: checksumsFor(entries)})
	archive := buildTkp(t, entries)

	if _, err := ExtractPackage(archive, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected a package without manifest.json to be refused")
	}
}

func TestExtractPackageRequiresChecksums(t *testing.T) {
	archive := buildTkp(t, validEntries())
	if _, err := ExtractPackage(archive, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected a package without checksums.json to be refused")
	}
}

func TestCleanArchiveNameNormalisesDotSlash(t *testing.T) {
	cases := map[string]string{
		"./":            "",
		".":             "",
		"":              "",
		"./manifest.js": "manifest.js",
		"assets/a.png":  "assets/a.png",
	}
	for input, want := range cases {
		if got := cleanArchiveName(input); got != want {
			t.Fatalf("cleanArchiveName(%q) = %q, want %q", input, got, want)
		}
	}
}
