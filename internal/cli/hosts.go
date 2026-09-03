// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// hostsSubcommand is the single source of truth for writing and removing
// the friendly-URL hosts entries. Both `install.sh` (via shell) and
// `tk service install` / `tk uninstall` (via this Go routine) call into
// it, so the entries are added and removed symmetrically.
//
// The block is delimited by sentinel comments so re-running is safe and
// the lines are easy to spot in the file:
//
//   # >>> timekeeper begin
//   127.0.0.1 timekeeper.local
//   127.0.0.1 api.timekeeper.local
//   # <<< timekeeper end
func hostsSubcommand(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: tk hosts <add|remove|status>")
		return 2
	}
	path, err := resolveHostsFile()
	if err != nil {
		fmt.Fprintf(errOut, "could not resolve hosts file path: %v\n", err)
		return 1
	}
	switch args[0] {
	case "add":
		return hostsAdd(path, out, errOut)
	case "remove":
		return hostsRemove(path, out, errOut)
	case "status":
		return hostsStatus(path, out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown hosts subcommand %q (use add|remove|status)\n", args[0])
		return 2
	}
}

// resolveHostsFile returns the OS hosts file path. On Windows it is
// always %SystemRoot%\System32\drivers\etc\hosts. On Linux/WSL/macOS
// it is /etc/hosts.
func resolveHostsFile() (string, error) {
	if runtime.GOOS == "windows" {
		sysRoot := strings.TrimSpace(os.Getenv("SystemRoot"))
		if sysRoot == "" {
			sysRoot = `C:\Windows`
		}
		return filepath.Join(sysRoot, "System32", "drivers", "etc", "hosts"), nil
	}
	return "/etc/hosts", nil
}

const (
	hostsBegin = "# >>> timekeeper begin"
	hostsEnd   = "# <<< timekeeper end"
	hostsEntry1 = "127.0.0.1 timekeeper.local"
	hostsEntry2 = "127.0.0.1 api.timekeeper.local"
)

func hostsAdd(path string, out, errOut io.Writer) int {
	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(errOut, "could not read %s: %v\n", path, err)
		return 1
	}
	if hostsBlockExists(string(body)) {
		fmt.Fprintf(out, "Hosts entries already present in %s; nothing to do.\n", path)
		return 0
	}
	block := strings.Join([]string{"", hostsBegin, hostsEntry1, hostsEntry2, hostsEnd, ""}, "\n")
	newBody := string(body) + block
	if err := os.WriteFile(path, []byte(newBody), 0o644); err != nil {
		fmt.Fprintf(errOut, "could not write %s: %v\n", path, err)
		if isAccessDenied(err) {
			fmt.Fprintln(errOut, "The hosts file requires elevated privileges.")
			fmt.Fprintln(errOut, "On Windows, run from an Administrator PowerShell or use the installer.")
			fmt.Fprintln(errOut, "On Linux/WSL/macOS, run via sudo, or run the installer which uses sudo internally.")
		}
		return 1
	}
	fmt.Fprintf(out, "Added friendly-URL hosts entries to %s\n", path)
	return 0
}

func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "Access is denied") || strings.Contains(s, "permission denied")
}

func hostsRemove(path string, out, errOut io.Writer) int {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(out, "%s does not exist; nothing to remove.\n", path)
			return 0
		}
		fmt.Fprintf(errOut, "could not read %s: %v\n", path, err)
		return 1
	}
	stripped, removed := stripHostsBlock(string(body))
	if !removed {
		fmt.Fprintf(out, "No TimeKeeper hosts entries found in %s; nothing to do.\n", path)
		return 0
	}
	if err := os.WriteFile(path, []byte(stripped), 0o644); err != nil {
		fmt.Fprintf(errOut, "could not write %s: %v\n", path, err)
		return 1
	}
	fmt.Fprintf(out, "Removed friendly-URL hosts entries from %s\n", path)
	return 0
}

func hostsStatus(path string, out, errOut io.Writer) int {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(errOut, "could not open %s: %v\n", path, err)
		return 1
	}
	defer f.Close()
	found := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == hostsEntry1 || line == hostsEntry2 {
			found = true
			fmt.Fprintf(out, "found: %s\n", line)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(errOut, "could not read %s: %v\n", path, err)
		return 1
	}
	if !found {
		fmt.Fprintf(out, "no TimeKeeper hosts entries in %s\n", path)
		return 1
	}
	return 0
}

func hostsBlockExists(body string) bool {
	return strings.Contains(body, hostsBegin) && strings.Contains(body, hostsEnd)
}

// stripHostsBlock removes the delimited block (and any leading blank
// line that we wrote next to it) from the hosts file body. Returns
// the stripped body and whether anything was removed.
func stripHostsBlock(body string) (string, bool) {
	startIdx := strings.Index(body, hostsBegin)
	if startIdx < 0 {
		return body, false
	}
	endIdx := strings.Index(body, hostsEnd)
	if endIdx < 0 {
		return body, false
	}
	endIdx += len(hostsEnd)
	// Trim the trailing newline that followed the end sentinel, and
	// any leading blank line that preceded the begin sentinel.
	stripped := body[:startIdx] + body[endIdx:]
	stripped = strings.TrimRight(stripped, "\n") + "\n"
	return stripped, true
}
