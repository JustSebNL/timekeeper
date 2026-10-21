package desktop

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/version"
)

func TestLaunchUsesExplicitPathsAndLoopback(t *testing.T) {
	addr := reserveLoopbackAddress(t)
	cfg := validLaunchConfig(t, "http://"+addr)
	launcher := &fakeLauncher{}
	launcher.start = func(_ string, args []string) (ManagedProcess, error) {
		process := newFakeProcess(nil)
		launcher.process = process
		server, err := serveFakeCore(addr, process)
		if err != nil {
			return nil, err
		}
		process.onClose = func() error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			return server.Shutdown(ctx)
		}
		return process, nil
	}

	session, err := StartOrAttach(context.Background(), cfg, launcher)
	if err != nil {
		t.Fatal(err)
	}
	if !session.Owned() {
		t.Fatal("launched Core was not marked as owned")
	}
	if launcher.startCount != 1 {
		t.Fatalf("launch count = %d, want 1", launcher.startCount)
	}
	assertArg(t, launcher.args, "-addr", addr)
	assertArg(t, launcher.args, "-db", cfg.DatabasePath)
	assertArg(t, launcher.args, "-ui", cfg.DashboardPath)
	if !containsArg(launcher.args, "-shutdown-on-stdin-close") {
		t.Fatalf("child args do not enable private stdin shutdown: %q", launcher.args)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if launcher.process.closeN != 1 || launcher.process.killN != 0 {
		t.Fatalf("graceful close/kill = %d/%d, want 1/0", launcher.process.closeN, launcher.process.killN)
	}
}

func TestStartupTimeoutStopsOnlyOwnedChild(t *testing.T) {
	addr := reserveLoopbackAddress(t)
	cfg := validLaunchConfig(t, "http://"+addr)
	cfg.StartupTimeout = 80 * time.Millisecond
	cfg.ShutdownTimeout = 40 * time.Millisecond
	process := newFakeProcess(nil)
	process.ignoreClose = true
	launcher := &fakeLauncher{process: process}

	_, err := StartOrAttach(context.Background(), cfg, launcher)
	if !errors.Is(err, ErrStartupTimeout) {
		t.Fatalf("StartOrAttach() error = %v, want ErrStartupTimeout", err)
	}
	if launcher.startCount != 1 {
		t.Fatalf("launch count = %d, want 1", launcher.startCount)
	}
	if process.closeN != 1 || process.killN != 1 {
		t.Fatalf("owned process close/kill = %d/%d, want 1/1", process.closeN, process.killN)
	}
}

func TestChildExitBeforeReadyFails(t *testing.T) {
	addr := reserveLoopbackAddress(t)
	cfg := validLaunchConfig(t, "http://"+addr)
	process := newFakeProcess(nil)
	process.diagnostic = "database is locked"
	process.finish(errors.New("exit code 2"))
	launcher := &fakeLauncher{process: process}

	_, err := StartOrAttach(context.Background(), cfg, launcher)
	if !errors.Is(err, ErrChildExited) {
		t.Fatalf("StartOrAttach() error = %v, want ErrChildExited", err)
	}
	if !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("StartOrAttach() error omitted bounded child diagnostics: %v", err)
	}
	if launcher.startCount != 1 {
		t.Fatalf("launch count = %d, want 1", launcher.startCount)
	}
	if process.killN != 0 {
		t.Fatalf("already exited child was killed %d times", process.killN)
	}
}

func TestBoundedDiagnosticBufferCapsOutput(t *testing.T) {
	buffer := &boundedDiagnosticBuffer{}
	input := strings.Repeat("x", maxStartupDiagnosticBytes+17)
	written, err := buffer.Write([]byte(input))
	if err != nil || written != len(input) {
		t.Fatalf("Write() = %d, %v; want %d, nil", written, err, len(input))
	}
	if got := len(buffer.String()); got != maxStartupDiagnosticBytes {
		t.Fatalf("diagnostic bytes = %d, want bounded %d", got, maxStartupDiagnosticBytes)
	}
}

func TestExecLauncherCapturesBoundedChildStderr(t *testing.T) {
	const helperEnv = "TIMEKEEPER_DESKTOP_STDERR_HELPER"
	if os.Getenv(helperEnv) == "1" {
		_, _ = fmt.Fprint(os.Stderr, strings.Repeat("database-lock-diagnostic ", 512))
		os.Exit(0)
	}
	t.Setenv(helperEnv, "1")

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process, err := (ExecLauncher{}).Start(context.Background(), executable, []string{"-test.run=^TestExecLauncherCapturesBoundedChildStderr$"})
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Wait(); err != nil {
		t.Fatalf("child Wait() = %v", err)
	}
	provider, ok := process.(startupDiagnosticProvider)
	if !ok {
		t.Fatal("ExecLauncher child does not expose startup diagnostics")
	}
	diagnostic := provider.StartupDiagnostics()
	if !strings.Contains(diagnostic, "database-lock-diagnostic") {
		t.Fatal("child stderr was not captured")
	}
	if len(diagnostic) > maxStartupDiagnosticBytes {
		t.Fatalf("captured stderr length = %d, limit %d", len(diagnostic), maxStartupDiagnosticBytes)
	}
}

func TestCloseDoesNotStopAttachedCore(t *testing.T) {
	server := httptest.NewServer(coreHandler())
	defer server.Close()
	launcher := &fakeLauncher{}
	session, err := StartOrAttach(context.Background(), Config{CoreURL: server.URL}, launcher)
	if err != nil {
		t.Fatal(err)
	}
	if session.Owned() {
		t.Fatal("attached Core was marked as owned")
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if launcher.startCount != 0 {
		t.Fatalf("attached Core started a child: %d", launcher.startCount)
	}
	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("attached Core stopped at desktop close: %v", err)
	}
	_ = resp.Body.Close()
}

func serveFakeCore(addr string, process *fakeProcess) (*http.Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: coreHandler()}
	go func() { _ = server.Serve(listener) }()
	return server, nil
}

func coreHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/version":
			_, _ = fmt.Fprintf(w, `{"service":"TimeKeeper","version":%q}`, version.Get().Version)
		case "/api/v1/connection-info":
			_, _ = w.Write([]byte(`{"service":"TimeKeeper","instance_id":"tk_instance_0123456789abcdef0123456789abcdef","version":"` + version.Get().Version + `","api_version":"v1","capabilities":["projects.list","tasks.lifecycle"]}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func assertArg(t *testing.T, args []string, name, want string) {
	t.Helper()
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			if args[i+1] != want {
				t.Fatalf("argument %s = %q, want %q (all args %q)", name, args[i+1], want, args)
			}
			return
		}
	}
	t.Fatalf("argument %s missing from %q", name, args)
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if strings.EqualFold(arg, want) {
			return true
		}
	}
	return false
}
