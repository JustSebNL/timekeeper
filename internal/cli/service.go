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
// that runs TimeKeeper. It delegates to the Windows PowerShell installer or the
// Linux service-manager script; Windows uses Bend2Will.
func serviceManager(args []string, out, errOut io.Writer, baseURL string) int {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	exeDir := filepath.Dir(filepath.Clean(exe))

	action := "status"
	if len(args) > 1 {
		fmt.Fprintln(errOut, "usage: tk service <install|uninstall|start|stop|restart|status|logs>")
		return 2
	}
	if len(args) == 1 {
		action = args[0]
	}
	if action == "remove" {
		action = "uninstall"
	}
	switch action {
	case "install", "uninstall", "start", "stop", "restart", "status", "logs":
	default:
		fmt.Fprintln(errOut, "unknown service action")
		return 2
	}
	var script, shell string
	var cmdArgs []string
	if runtime.GOOS == "windows" {
		script = findWindowsServiceScript(exeDir)
		shell = "powershell.exe"
		cmdArgs = []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", action, "-Elevate"}
	} else {
		script = findServiceScript()
		shell = "bash"
		cmdArgs = []string{script, action}
	}
	if script == "" {
		fmt.Fprintln(errOut, "service manager script missing; rerun the installer")
		return 1
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
//  1. TIMEKEEPER_SERVICE_SCRIPT (explicit override)
//  2. Alongside the installed binary: .timekeeper/app/scripts/service/service-manager.sh
//  3. Repo-root relative: scripts/service/service-manager.sh
//  4. Working directory: scripts/service/service-manager.sh
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

func findWindowsServiceScript(exeDir string) string {
	candidates := []string{filepath.Join(exeDir, "..", "scripts", "service", "install-windows.ps1")}
	for dir := exeDir; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			candidates = append(candidates, filepath.Join(dir, "scripts", "service", "install-windows.ps1"))
			break
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			abs, err := filepath.Abs(p)
			if err == nil {
				return abs
			}
		}
	}
	return ""
}
