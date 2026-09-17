package cli

import (
	"context"
	"flag"
	"fmt"
	"github.com/JustSebNL/timekeeper/internal/supervisor"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func supervise(args []string, out, errOut io.Writer, base string) int {
	f := flag.NewFlagSet("supervise", flag.ContinueOnError)
	f.SetOutput(errOut)
	cfg := supervisor.Config{}
	f.StringVar(&cfg.Project, "project", "", "owning project ID or alias")
	f.StringVar(&cfg.Sprint, "sprint", "", "Open or On Hold Sprint ID")
	f.StringVar(&cfg.Agent, "agent", "", "unique stable owner ID for this supervised worker")
	f.StringVar(&cfg.Model, "model", "", "explicit Codex model")
	f.StringVar(&cfg.StateDir, "state-dir", "", "private checkpoint directory")
	f.DurationVar(&cfg.Stall, "stall-timeout", 5*time.Minute, "maximum time without a completed work item")
	binary := f.String("codex", "codex", "Codex executable path")
	if err := f.Parse(args); err != nil {
		return 2
	}
	if len(f.Args()) == 0 {
		fmt.Fprintln(errOut, "usage: tk supervise --project P-ID --sprint SP-ID --agent worker --model MODEL --state-dir DIR -- [codex exec options] PROMPT")
		return 2
	}
	for _, a := range f.Args() {
		if a == "-m" || strings.HasPrefix(a, "--model") || strings.Contains(a, "model=") || a == "--oss" || a == "--local-provider" {
			fmt.Fprintln(errOut, "set the model only through supervisor --model")
			return 2
		}
	}
	p, err := resolveProjectID(base, cfg.Project)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	cfg.Project = p
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	executable, err := codexExecutable(*binary)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	cmd := exec.Command(executable, append([]string{"exec", "--json", "--model", cfg.Model}, f.Args()...)...)
	if err := supervisor.Run(ctx, supervisor.Client{Base: base}, cfg, cmd, os.Stdin, out, errOut); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}

// Windows npm shims are scripts, not directly launchable CreateProcess images.
// Prefer the native Codex executable already on PATH without adding a shell.
func codexExecutable(name string) (string, error) {
	if runtime.GOOS != "windows" {
		return exec.LookPath(name)
	}
	if name == "codex" {
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			if dir == "" || !filepath.IsAbs(dir) {
				continue
			}
			candidate := filepath.Join(dir, "codex.exe")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(p), ".exe") {
		return "", fmt.Errorf("Codex resolved to a script shim; provide --codex with the native codex.exe path")
	}
	return p, nil
}
