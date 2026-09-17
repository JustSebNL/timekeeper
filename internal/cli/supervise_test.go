package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCodexExecutableSkipsWindowsNpmShim(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows executable discovery")
	}
	shim := t.TempDir()
	native := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, "codex.cmd"), []byte("@echo off"), 0600); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(native, "codex.exe")
	if err := os.WriteFile(want, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shim+string(os.PathListSeparator)+native)
	got, err := codexExecutable("codex")
	if err != nil || got != want {
		t.Fatalf("%s %v", got, err)
	}
}
