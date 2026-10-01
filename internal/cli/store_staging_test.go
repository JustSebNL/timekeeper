package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateStoreArchiveRefusesPreexistingSymlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "outside.bin")
	archive := filepath.Join(tmp, "package.tkp")
	const sentinel = "do not overwrite"
	if err := os.WriteFile(target, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, archive); err != nil {
		t.Skipf("symlinks unavailable in this test environment: %v", err)
	}

	file, err := createStoreArchive(archive)
	if err == nil {
		_ = file.Close()
		t.Fatal("expected an existing symlink to be refused")
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != sentinel {
		t.Fatalf("symlink target changed: %q", got)
	}
}

func TestCreateStoreArchiveCreatesNewFileExclusively(t *testing.T) {
	path := filepath.Join(t.TempDir(), "package.tkp")
	file, err := createStoreArchive(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := createStoreArchive(path); err == nil {
		t.Fatal("expected a second create to be refused")
	}
}

func TestEnsureStoreDirectoryRejectsSymlinkedAncestor(t *testing.T) {
	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(tmp, "state")
	if err := os.Mkdir(state, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(state, "extensions")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable in this test environment: %v", err)
	}

	staging := filepath.Join(link, ".staging")
	if err := ensureStoreDirectory(staging); err == nil {
		t.Fatal("expected a symlinked Store directory ancestor to be refused")
	}
	if _, err := os.Lstat(filepath.Join(outside, ".staging")); !os.IsNotExist(err) {
		t.Fatalf("symlink target was modified: %v", err)
	}
}

func TestEnsureStoreDirectoryCreatesNormalTree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "extensions", ".staging")
	if err := ensureStoreDirectory(path); err != nil {
		t.Fatalf("create Store directory: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("Store staging path is not a directory")
	}
}
