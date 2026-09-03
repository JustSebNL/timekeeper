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
// that runs TimeKeeper. It delegates to .timekeeper/scripts/service/service-install.{bat,sh},
// which handle Windows (NSSM) and Linux (systemd) specifics with local logging.
func serviceManager(args []string, out, errOut io.Writer, baseURL string) int {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	exeDir := filepath.Dir(filepath.Clean(exe))

	var script string
	var shell string
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
		script = filepath.Join(exeDir, "..", "scripts", "service", "service-install.bat")
	} else {
		shell = "bash"
		script = filepath.Join(exeDir, "..", "scripts", "service", "service-install.sh")
	}
	if _, err := os.Stat(script); err != nil {
		_, _ = fmt.Fprintln(errOut, "service manager script not found — expected at .timekeeper/scripts/service/")
		return 1
	}

	var cmdArgs []string
	if runtime.GOOS == "windows" {
		cmdArgs = append([]string{"/c", script}, args...)
	} else {
		cmdArgs = append([]string{script}, args...)
	}
	cmd := exec.Command(shell, cmdArgs...)
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
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

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(filepath.Clean(exe))
		candidates = append(candidates, filepath.Join(exeDir, "..", "scripts", "service", "service-manager.sh"))
		if repoRoot := os.Getenv("TIMEKEEPER_REPO"); repoRoot != "" {
			candidates = append(candidates, filepath.Join(repoRoot, "scripts", "service", "service-manager.sh"))
		} else {
			repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Clean(exe))))
			candidates = append(candidates, filepath.Join(repoRoot, "scripts", "service", "service-manager.sh"))
		}
	}

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
