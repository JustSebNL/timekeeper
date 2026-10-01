package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/storeclient"
)

func TestGuardianWorkCommandUsesHostShell(t *testing.T) {
	t.Run("linux", func(t *testing.T) {
		shell, args := guardianWorkCommand("linux", "scripts/guardian/self-nudge-work.sh")
		if shell != "bash" || len(args) != 1 || args[0] != "scripts/guardian/self-nudge-work.sh" {
			t.Fatalf("shell=%q args=%#v", shell, args)
		}
	})

	t.Run("windows shell hook uses WSL", func(t *testing.T) {
		shell, args := guardianWorkCommand("windows", "scripts/guardian/self-nudge-work.sh")
		if shell != "wsl.exe" {
			t.Fatalf("shell=%q, want wsl.exe", shell)
		}
		if len(args) != 3 || args[0] != "--" || args[1] != "bash" || !strings.HasSuffix(args[2], "/scripts/guardian/self-nudge-work.sh") {
			t.Fatalf("args=%#v, want WSL bash with absolute script path", args)
		}
	})

	t.Run("windows batch hook uses cmd", func(t *testing.T) {
		shell, args := guardianWorkCommand("windows", "scripts/guardian/self-nudge-work.bat")
		if shell != "cmd.exe" || len(args) != 2 || args[0] != "/c" || args[1] != "scripts/guardian/self-nudge-work.bat" {
			t.Fatalf("shell=%q args=%#v", shell, args)
		}
	})
}

func TestGuardianWakeOptionsParseContinuousBudget(t *testing.T) {
	options, err := parseGuardianWakeOptions([]string{"--agent", "xatia", "--poll", "--continuous", "--max-cycles", "4"})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if !options.continuous || options.maxCycles != 4 {
		t.Fatalf("options=%+v, want continuous budget 4", options)
	}
}

func TestGuardianWakeOptionsParseUntilIdle(t *testing.T) {
	options, err := parseGuardianWakeOptions([]string{"--agent", "xatia", "--poll", "--continuous", "--until-idle", "--max-cycles", "4"})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if !options.continuous || !options.untilIdle || options.maxCycles != 4 {
		t.Fatalf("options=%+v, want until-idle continuous budget 4", options)
	}
	if _, err := parseGuardianWakeOptions([]string{"--agent", "xatia", "--until-idle"}); err == nil {
		t.Fatal("expected --until-idle to require --continuous")
	}
}

func TestProjectHasRunnableWorkUsesDurableSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects/P-10053/operational-summary" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"open_sprints":1,"active_sprints":0}`))
	}))
	defer server.Close()
	tk := &storeclient.Client{BaseURL: server.URL, HTTPClient: server.Client()}
	runnable, err := projectHasRunnableWork(t.Context(), tk, "P-10053")
	if err != nil || !runnable {
		t.Fatalf("runnable=%v err=%v", runnable, err)
	}
}

func TestWorkUntilIdleRearmsOnlyWhileProjectHasSprints(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell adapter test uses POSIX sh")
	}
	dir := t.TempDir()
	countPath := filepath.Join(dir, "cycles")
	scriptPath := filepath.Join(dir, "adapter.sh")
	script := "#!/bin/sh\nprintf x >> \"" + countPath + "\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TK_GUARDIAN_SELF_WORK", scriptPath)
	var summaryCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		summaryCalls++
		if summaryCalls == 1 {
			_, _ = w.Write([]byte(`{"open_sprints":1,"active_sprints":0}`))
			return
		}
		_, _ = w.Write([]byte(`{"open_sprints":0,"active_sprints":0}`))
	}))
	defer server.Close()
	tk := &storeclient.Client{BaseURL: server.URL, HTTPClient: server.Client()}
	ctx := &AgentContext{AgentID: "test-agent", LastProjectID: "P-10053"}
	completed := runWorkUntilIdle("test-agent", tk, ctx, nil, 2, &strings.Builder{}, &strings.Builder{})
	if completed != 1 || ctx.WorkCycles != 1 || summaryCalls != 2 {
		t.Fatalf("completed=%d work_cycles=%d summary_calls=%d, want one rearmed cycle then idle", completed, ctx.WorkCycles, summaryCalls)
	}
	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "x" {
		t.Fatalf("adapter cycles=%q, want one", got)
	}
}

func TestGuardianWakeOptionsRejectsInvalidBudget(t *testing.T) {
	if _, err := parseGuardianWakeOptions([]string{"--agent", "xatia", "--continuous", "--max-cycles", "0"}); err == nil {
		t.Fatal("expected invalid max-cycles error")
	}
}

func TestAdditionalWorkCyclesPreserveContinuationIntent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell adapter test uses POSIX sh")
	}
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "result")
	scriptPath := filepath.Join(dir, "adapter.sh")
	script := "#!/bin/sh\nprintf '%s|%s|%s|%s|%s' \"$GUARDIAN_EVENT\" \"$GUARDIAN_PRIORITY\" \"$GUARDIAN_CONTINUATION_MODE\" \"$GUARDIAN_CONTINUATION_CYCLE\" \"$GUARDIAN_CONTINUATION_BUDGET\" > \"" + resultPath + "\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TK_GUARDIAN_SELF_WORK", scriptPath)
	ctx := &AgentContext{AgentID: "test-agent"}
	completed := runAdditionalWorkCycles("test-agent", ctx, nil, 3, &strings.Builder{}, &strings.Builder{})
	if completed != 2 || ctx.WorkCycles != 2 {
		t.Fatalf("completed=%d context=%+v, want two follow-up cycles", completed, ctx)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "guardian.continuation|actionable|continuous|3|3"; got != want {
		t.Fatalf("continuation intent=%q, want %q", got, want)
	}
}

func TestWindowsPathToWSL(t *testing.T) {
	for _, tc := range []struct {
		input, want string
	}{
		{`D:\dev\codebase\dev\TimeKeeper\scripts\guardian\self-nudge-work.sh`, "/mnt/d/dev/codebase/dev/TimeKeeper/scripts/guardian/self-nudge-work.sh"},
		{"/mnt/d/dev/codebase/dev/TimeKeeper/scripts/guardian/self-nudge-work.sh", "/mnt/d/dev/codebase/dev/TimeKeeper/scripts/guardian/self-nudge-work.sh"},
	} {
		if got := windowsPathToWSL(tc.input); got != tc.want {
			t.Errorf("windowsPathToWSL(%q)=%q, want %q", tc.input, got, tc.want)
		}
	}
}
