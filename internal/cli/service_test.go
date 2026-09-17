package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsServiceScriptPackagedPath(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, ".timekeeper", "app", "bin")
	script := filepath.Join(root, ".timekeeper", "app", "scripts", "service", "install-windows.ps1")
	os.MkdirAll(bin, 0700)
	os.MkdirAll(filepath.Dir(script), 0700)
	os.WriteFile(script, []byte("# fixture"), 0600)
	if got := findWindowsServiceScript(bin); got != script {
		t.Fatalf("got %s want %s", got, script)
	}
}
