package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Windows PowerShell 5.1 reads a BOM-less script as the active ANSI code page.
// Bytes that are valid UTF-8 then decode to characters that change parsing, so
// an em dash inside a double-quoted string turns into a closing quote and the
// whole script fails to parse. pwsh 7 defaults to UTF-8 and hides the defect,
// so the scripts must stay ASCII.
func TestRepoWindowsScriptsAreASCII(t *testing.T) {
	root, err := repoRootForTest()
	if err != nil {
		t.Skipf("repository root unavailable: %v", err)
	}
	for _, script := range windowsScriptsUnderTest(root) {
		data, err := os.ReadFile(script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		for i, b := range data {
			if b <= 0x7f {
				continue
			}
			line := 1
			for j := 0; j < i; j++ {
				if data[j] == '\n' {
					line++
				}
			}
			rel, _ := filepath.Rel(root, script)
			t.Errorf("%s:%d: non-ASCII byte 0x%02x breaks Windows PowerShell 5.1 parsing; use an ASCII dash", rel, line, b)
		}
	}
}

func windowsScriptsUnderTest(root string) []string {
	patterns := []string{
		filepath.Join("scripts", "service", "install-windows.ps1"),
		filepath.Join("scripts", "service", "elevate-batch.ps1"),
		filepath.Join("scripts", "service", "kick-server.bat"),
		"install.bat",
		"restart.bat",
	}
	var found []string
	for _, pattern := range patterns {
		candidate := filepath.Join(root, pattern)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			found = append(found, candidate)
		}
	}
	return found
}

func repoRootForTest() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir || strings.TrimSpace(parent) == "" {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}