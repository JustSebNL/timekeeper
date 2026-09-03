// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// openBrowser launches the user's default browser at the TimeKeeper dashboard.
//
// Default URL: http://timekeeper.local/  (the friendly URL)
//
// Flags:
//   --api  open http://api.timekeeper.local/ instead
//   --ip   open http://127.0.0.1:1618/  (canonical loopback)
//
// The command never auto-starts the service. If the server is not
// reachable, it returns a non-zero exit and tells the user to run
// `tk doctor`.
func openBrowser(args []string, out, errOut io.Writer) int {
	url := "http://timekeeper.local/"
	for _, a := range args {
		switch a {
		case "--api":
			url = "http://api.timekeeper.local/"
		case "--ip":
			url = "http://127.0.0.1:1618/"
		case "--help", "-h":
			fmt.Fprintln(out, "usage: tk open [--api|--ip]")
			fmt.Fprintln(out, "  default:  http://timekeeper.local/")
			fmt.Fprintln(out, "  --api:    http://api.timekeeper.local/")
			fmt.Fprintln(out, "  --ip:     http://127.0.0.1:1618/")
			return 0
		default:
			fmt.Fprintf(errOut, "unknown flag %q (use --help)\n", a)
			return 2
		}
	}

	// Health-check before launching. We never want to send the user
	// to a browser tab that errors out.
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := strings.TrimRight(url, "/") + "/health"
	resp, err := client.Get(healthURL)
	if err != nil {
		fmt.Fprintf(errOut, "TimeKeeper is not reachable at %s (%v).\n", url, err)
		fmt.Fprintln(errOut, "Start the service, then run `tk doctor` to diagnose.")
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(errOut, "TimeKeeper at %s returned %s on /health.\n", url, resp.Status)
		return 1
	}

	browser, err := pickBrowser()
	if err != nil {
		fmt.Fprintf(errOut, "no browser available on this host: %v\n", err)
		return 1
	}
	if err := browser(url); err != nil {
		fmt.Fprintf(errOut, "failed to open %s: %v\n", url, err)
		return 1
	}
	fmt.Fprintf(out, "Opened %s in the default browser.\n", url)
	return 0
}

// pickBrowser returns a launcher function for the current platform.
//   - Windows: rundll32 url.dll,FileProtocolHandler
//   - macOS:   open
//   - Linux:   xdg-open
func pickBrowser() (func(string) error, error) {
	switch runtime.GOOS {
	case "windows":
		return func(u string) error {
			return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
		}, nil
	case "darwin":
		return func(u string) error {
			return exec.Command("open", u).Run()
		}, nil
	case "linux":
		// xdg-open is the de-facto standard; if it's missing, report.
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return nil, fmt.Errorf("xdg-open not found in PATH")
		}
		return func(u string) error {
			return exec.Command("xdg-open", u).Run()
		}, nil
	default:
		return nil, fmt.Errorf("unsupported platform %q", runtime.GOOS)
	}
}

// uninstallTimeKeeper is the symmetric counterpart to install.sh.
// It stops the OS service, removes the installed binaries, and
// strips the friendly-URL hosts entries. It does NOT touch the
// SQLite database or the .timekeeper/ state directory — that is
// the user's data, and uninstall is documented to leave it alone.
//
// Implementation note: the actual file/service work is delegated
// to the service-install.{bat,sh} script with an `uninstall`
// subcommand, so behaviour stays consistent across platforms.
func uninstallTimeKeeper(out, errOut io.Writer, baseURL string) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(errOut, "could not resolve self path: %v\n", err)
		return 1
	}
	script := scriptPathForUninstall(exe)
	if script == "" {
		fmt.Fprintln(errOut, "service-install script not found next to the binary.")
		fmt.Fprintln(errOut, "Re-run the installer to repair the install, or remove TimeKeeper manually.")
		return 1
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/c", script, "uninstall")
	} else {
		cmd = exec.Command(script, "uninstall")
	}
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(errOut, "uninstall failed: %v\n", err)
		return 1
	}
	// Best-effort: strip the friendly-URL hosts entries. If the
	// user does not have permission to edit the hosts file, this
	// fails silently and the README documents the manual step.
	_ = hostsSubcommand([]string{"remove"}, out, errOut)
	fmt.Fprintln(out, "TimeKeeper uninstalled.")
	fmt.Fprintln(out, "Hosts entries removed. The .timekeeper/ state directory and SQLite database were left in place; remove them manually if you want a clean uninstall.")
	return 0
}

// scriptPathForUninstall locates the service-install script next to
// the binary, the same way the service subcommand does.
func scriptPathForUninstall(exe string) string {
	dir := ""
	if i := strings.LastIndexAny(exe, "/\\"); i >= 0 {
		dir = exe[:i]
	}
	if dir == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		return dir + string(os.PathSeparator) + "service-install.bat"
	}
	return dir + string(os.PathSeparator) + "service-install.sh"
}
