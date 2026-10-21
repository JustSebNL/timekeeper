package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const maxStartupDiagnosticBytes = 4096

// Launcher starts the configured server executable with explicit arguments.
type Launcher interface {
	Start(context.Context, string, []string) (ManagedProcess, error)
}

// ManagedProcess exposes only the operations required to supervise an owned
// Core child. It cannot find or stop arbitrary processes.
type ManagedProcess interface {
	Wait() error
	CloseInput() error
	Kill() error
}

// ExecLauncher starts a child with a private stdin pipe used for graceful
// parent-close shutdown.
type ExecLauncher struct{}

func (ExecLauncher) Start(ctx context.Context, executable string, args []string) (ManagedProcess, error) {
	// Detach process lifetime from a short-lived startup context. Session.Close
	// owns graceful shutdown and the only permitted force-kill path.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), executable, args...)
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create Core child stdin: %w", err)
	}
	stderr := &boundedDiagnosticBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = input.Close()
		return nil, fmt.Errorf("launch Core child: %w", err)
	}
	return &execManagedProcess{cmd: cmd, input: input, stderr: stderr}, nil
}

type execManagedProcess struct {
	cmd       *exec.Cmd
	input     interface{ Close() error }
	stderr    *boundedDiagnosticBuffer
	waitOnce  sync.Once
	waitDone  chan struct{}
	waitErr   error
	closeOnce sync.Once
	closeErr  error
}

func (p *execManagedProcess) StartupDiagnostics() string {
	if p.stderr == nil {
		return ""
	}
	return p.stderr.String()
}

type boundedDiagnosticBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *boundedDiagnosticBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(data)
	remaining := maxStartupDiagnosticBytes - len(b.data)
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		b.data = append(b.data, data...)
	}
	return written, nil
}

func (b *boundedDiagnosticBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}

type startupDiagnosticProvider interface {
	StartupDiagnostics() string
}

func withStartupDiagnostics(err error, child ManagedProcess) error {
	provider, ok := child.(startupDiagnosticProvider)
	if !ok {
		return err
	}
	diagnostic := strings.TrimSpace(provider.StartupDiagnostics())
	if diagnostic == "" {
		return err
	}
	return fmt.Errorf("%w; Core stderr: %s", err, diagnostic)
}

func (p *execManagedProcess) Wait() error {
	p.waitOnce.Do(func() {
		p.waitDone = make(chan struct{})
		go func() {
			p.waitErr = p.cmd.Wait()
			close(p.waitDone)
		}()
	})
	<-p.waitDone
	return p.waitErr
}

func (p *execManagedProcess) CloseInput() error {
	p.closeOnce.Do(func() { p.closeErr = p.input.Close() })
	return p.closeErr
}

func (p *execManagedProcess) Kill() error {
	if p.cmd.Process == nil {
		return errors.New("Core child process was not started")
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func (s *Session) Close(ctx context.Context) error {
	if s == nil || !s.owned {
		return nil
	}
	return s.stopOwned(ctx)
}

func (s *Session) stopOwned(ctx context.Context) error {
	if s.child == nil {
		return nil
	}
	closeErr := s.child.CloseInput()
	timeout := s.shutdownTimeout
	if timeout <= 0 {
		timeout = defaultShutdown
	}
	select {
	case childErr := <-s.childDone:
		if closeErr != nil {
			return closeErr
		}
		if childErr != nil {
			return fmt.Errorf("Core child exited during shutdown: %w", childErr)
		}
		return nil
	default:
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case childErr := <-s.childDone:
		if closeErr != nil {
			return closeErr
		}
		if childErr != nil {
			return fmt.Errorf("Core child exited during shutdown: %w", childErr)
		}
		return nil
	case <-ctx.Done():
		return s.killOwned()
	case <-timer.C:
		return s.killOwned()
	}
}

func (s *Session) killOwned() error {
	if err := s.child.Kill(); err != nil {
		return fmt.Errorf("force-stop owned Core child: %w", err)
	}
	select {
	case <-s.childDone:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("owned Core child did not exit after kill")
	}
}
