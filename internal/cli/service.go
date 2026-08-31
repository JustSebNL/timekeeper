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
	repoRoot := os.Getenv("TIMEKEEPER_REPO")
	if repoRoot == "" {
		// Try to infer from binary location or working directory
		if exe, err := os.Executable(); err == nil {
			exe = filepath.Clean(exe)
			// Typical layout: .timekeeper/app/bin/tk -> repo root is 3 levels up
			if rel, err := filepath.Rel(filepath.Dir(filepath.Dir(filepath.Dir(exe))), "."); err == nil && rel != "." {
				repoRoot = "."
			}
		}
		if repoRoot == "" {
			repoRoot = "."
		}
	}

	script := filepath.Join(repoRoot, "scripts", "service", "service-manager.sh")
	if _, err := os.Stat(script); err != nil {
		_, _ = fmt.Fprintf(errOut, "service manager script not found: %s\n", script)
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
