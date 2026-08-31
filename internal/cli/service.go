// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// serviceManager installs, removes, starts, stops, and queries the OS service
// that runs TimeKeeper. It delegates to scripts/service/service-manager.sh,
// which handles Windows (NSSM) and Linux (systemd) specifics.
func serviceManager(args []string, out, errOut io.Writer, baseURL string) int {
	script := findServiceScript()
	if script == "" {
		_, _ = fmt.Fprintln(errOut, "service manager script not found — expected at scripts/service/service-manager.sh")
		return 1
	}

	shell := "bash"
	if runtime.GOOS == "windows" {
		shell = "bash" // WSL / Git Bash
	}

	cmd := exec.Command(shell, script)
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		_, _ = fmt.Fprintf(errOut, "service command failed: %v\n", err)
		return 1
	}
	return 0
}

// findServiceScript locates service-manager.sh. It checks, in order:
//   1. TIMEKEEPER_SERVICE_SCRIPT (explicit override)
//   2. Alongside the installed binary: .timekeeper/app/scripts/service/service-manager.sh
//   3. Repo-root relative: scripts/service/service-manager.sh
//   4. Working directory: scripts/service/service-manager.sh
func findServiceScript() string {
	if explicit := os.Getenv("TIMEKEEPER_SERVICE_SCRIPT"); explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit
		}
	}

	candidates := []string{}

	// Path 2: alongside the installed binary (.timekeeper/app/bin/tk -> ../scripts/service/...)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(filepath.Clean(exe))
		candidates = append(candidates, filepath.Join(exeDir, "..", "scripts", "service", "service-manager.sh"))
	}

	// Path 3: repo root (TIMEKEEPER_REPO or inferred from binary: .timekeeper/app/bin -> 3 levels up)
	if repoRoot := os.Getenv("TIMEKEEPER_REPO"); repoRoot != "" {
		candidates = append(candidates, filepath.Join(repoRoot, "scripts", "service", "service-manager.sh"))
	} else if exe, err := os.Executable(); err == nil {
		repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Clean(exe))))
		candidates = append(candidates, filepath.Join(repoRoot, "scripts", "service", "service-manager.sh"))
	}

	// Path 4: current working directory
	candidates = append(candidates, filepath.Join("scripts", "service", "service-manager.sh"))

	for _, c := range candidates {
		if c == "" {
			continue
		}
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return ""
}
