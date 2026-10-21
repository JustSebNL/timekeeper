package instanceguard

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInstanceGuardProcessHelper(t *testing.T) {
	if os.Getenv("TIMEKEEPER_INSTANCE_GUARD_HELPER") != "1" {
		return
	}
	lock, err := Acquire(os.Getenv("TIMEKEEPER_INSTANCE_GUARD_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	fmt.Println("locked")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func TestAcquireRejectsConcurrentInstanceForSameDatabase(t *testing.T) {
	database := filepath.Join(t.TempDir(), "timekeeper.db")
	first, err := Acquire(database)
	if err != nil {
		t.Fatalf("acquire first instance: %v", err)
	}
	defer first.Close()

	second, err := Acquire(database)
	if !errors.Is(err, ErrAlreadyRunning) {
		if err == nil {
			_ = second.Close()
		}
		t.Fatalf("second acquire error = %v, want ErrAlreadyRunning", err)
	}
}

func coreInstanceTestPaths(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	stateDir := filepath.Join(root, ".timekeeper")
	uiDir := filepath.Join(stateDir, "app", "web")
	if err := os.MkdirAll(uiDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(stateDir, "timekeeper.db"), filepath.Join(stateDir, "recovered", "timekeeper.db"), filepath.Join(uiDir, "timekeeper.html")
}

func TestAcquireCoreInstanceRejectsIndependentDatabasesInSameInstall(t *testing.T) {
	firstDB, secondDB, uiPath := coreInstanceTestPaths(t)
	first, err := AcquireCoreInstance(firstDB, uiPath)
	if err != nil {
		t.Fatalf("acquire first Core instance: %v", err)
	}
	defer first.Close()

	second, err := AcquireCoreInstance(secondDB, uiPath)
	if !errors.Is(err, ErrCoreAlreadyRunning) {
		if err == nil {
			_ = second.Close()
		}
		t.Fatalf("second Core acquire error = %v, want ErrCoreAlreadyRunning", err)
	}
	if ErrCoreAlreadyRunning.Error() != "tk is already running" {
		t.Fatalf("user-facing singleton error = %q", ErrCoreAlreadyRunning)
	}
}

func TestAcquireCoreInstanceCanRestartAfterRelease(t *testing.T) {
	firstDB, secondDB, uiPath := coreInstanceTestPaths(t)
	first, err := AcquireCoreInstance(firstDB, uiPath)
	if err != nil {
		t.Fatalf("acquire first Core instance: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("release first Core instance: %v", err)
	}
	second, err := AcquireCoreInstance(secondDB, uiPath)
	if err != nil {
		t.Fatalf("acquire Core instance after release: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("release second Core instance: %v", err)
	}
}

func TestAcquireAllowsNewInstanceAfterRelease(t *testing.T) {
	database := filepath.Join(t.TempDir(), "timekeeper.db")
	first, err := Acquire(database)
	if err != nil {
		t.Fatalf("acquire first instance: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("release first instance: %v", err)
	}
	second, err := Acquire(database)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("release second instance: %v", err)
	}
}

func TestAcquireRejectsSecondProcessForSameDatabase(t *testing.T) {
	database := filepath.Join(t.TempDir(), "timekeeper.db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestInstanceGuardProcessHelper$")
	cmd.Env = append(os.Environ(), "TIMEKEEPER_INSTANCE_GUARD_HELPER=1", "TIMEKEEPER_INSTANCE_GUARD_DB="+database)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("child lock handshake = %q, err=%v", line, err)
	}
	if _, err := Acquire(database); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second process acquire error = %v, want ErrAlreadyRunning", err)
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait for lock holder: %v", err)
	}
	lock, err := Acquire(database)
	if err != nil {
		t.Fatalf("acquire after child exits: %v", err)
	}
	_ = lock.Close()
}

func TestAcquireUsesCanonicalDatabasePath(t *testing.T) {
	database := filepath.Join(t.TempDir(), "timekeeper.db")
	first, err := Acquire(database)
	if err != nil {
		t.Fatalf("acquire first instance: %v", err)
	}
	defer first.Close()

	alias := filepath.Join(filepath.Dir(database), ".", filepath.Base(database))
	second, err := Acquire(alias)
	if !errors.Is(err, ErrAlreadyRunning) {
		if err == nil {
			_ = second.Close()
		}
		t.Fatalf("aliased second acquire error = %v, want ErrAlreadyRunning", err)
	}
}