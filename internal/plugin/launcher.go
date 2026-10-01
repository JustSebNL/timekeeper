// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ProcessState describes the runtime lifecycle state of an external plugin subprocess.
type ProcessState string

const (
	StateStopped  ProcessState = "stopped"
	StateStarting ProcessState = "starting"
	StateRunning  ProcessState = "running"
	StateErrored  ProcessState = "errored"
)

// PluginProcess represents a tracked external plugin process.
type PluginProcess struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	ExecPath  string       `json:"exec_path"`
	Args      []string     `json:"args,omitempty"`
	Env       []string     `json:"env,omitempty"`
	State     ProcessState `json:"state"`
	PID       int          `json:"pid,omitempty"`
	ExitCode  int          `json:"exit_code,omitempty"`
	ErrorMsg  string       `json:"error_msg,omitempty"`
	StartedAt time.Time    `json:"started_at,omitempty"`
}

// LaunchConfig contains launch specification for starting an external plugin.
type LaunchConfig struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	ExecPath string            `json:"exec_path"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	CoreURL  string            `json:"core_url,omitempty"`
}

// Launcher manages out-of-process plugin execution safely outside Core memory space.
type Launcher struct {
	mu        sync.RWMutex
	processes map[string]*managedProcess
}

type managedProcess struct {
	id        string
	name      string
	execPath  string
	args      []string
	env       []string
	cmd       *exec.Cmd
	state     ProcessState
	pid       int
	exitCode  int
	errorMsg  string
	startedAt time.Time
	cancel    context.CancelFunc
}

// NewLauncher initializes a process launcher instance.
func NewLauncher() *Launcher {
	return &Launcher{
		processes: make(map[string]*managedProcess),
	}
}

// Launch starts an external executable plugin process out-of-process.
func (l *Launcher) Launch(ctx context.Context, config LaunchConfig) (*PluginProcess, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if config.ID == "" {
		return nil, errors.New("plugin ID is required")
	}
	if config.ExecPath == "" {
		return nil, errors.New("executable path is required")
	}

	if existing, found := l.processes[config.ID]; found {
		if existing.state == StateRunning {
			return nil, fmt.Errorf("plugin process %s is already running (PID %d)", config.ID, existing.pid)
		}
	}

	envVars := make([]string, 0, len(config.Env)+2)
	for k, v := range config.Env {
		envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
	}
	envVars = append(envVars, fmt.Sprintf("TIMEKEEPER_PLUGIN_ID=%s", config.ID))
	if config.CoreURL != "" {
		envVars = append(envVars, fmt.Sprintf("TIMEKEEPER_API_URL=%s", config.CoreURL))
	}

	procCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(procCtx, config.ExecPath, config.Args...)
	cmd.Env = append(os.Environ(), envVars...)

	mp := &managedProcess{
		id:        config.ID,
		name:      config.Name,
		execPath:  config.ExecPath,
		args:      config.Args,
		env:       envVars,
		cmd:       cmd,
		state:     StateStarting,
		startedAt: time.Now(),
		cancel:    cancel,
	}

	l.processes[config.ID] = mp

	if err := cmd.Start(); err != nil {
		mp.state = StateErrored
		mp.errorMsg = err.Error()
		cancel()
		return mp.toPublic(), fmt.Errorf("failed to start plugin process %s: %w", config.ID, err)
	}

	mp.state = StateRunning
	mp.pid = cmd.Process.Pid

	go func(mp *managedProcess) {
		err := cmd.Wait()
		l.mu.Lock()
		defer l.mu.Unlock()

		if err != nil {
			mp.state = StateErrored
			mp.errorMsg = err.Error()
			if exitErr, ok := err.(*exec.ExitError); ok {
				mp.exitCode = exitErr.ExitCode()
			} else {
				mp.exitCode = -1
			}
		} else {
			mp.state = StateStopped
			mp.exitCode = 0
			mp.errorMsg = ""
		}
	}(mp)

	return mp.toPublic(), nil
}

// Stop gracefully terminates a running plugin subprocess.
func (l *Launcher) Stop(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	mp, found := l.processes[id]
	if !found {
		return fmt.Errorf("plugin process %s not found", id)
	}

	if mp.state != StateRunning {
		return nil
	}

	if mp.cancel != nil {
		mp.cancel()
	}

	if mp.cmd != nil && mp.cmd.Process != nil {
		_ = mp.cmd.Process.Kill()
	}

	mp.state = StateStopped
	return nil
}

// Status returns current status of a tracked plugin process.
func (l *Launcher) Status(id string) (*PluginProcess, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	mp, found := l.processes[id]
	if !found {
		return nil, fmt.Errorf("plugin process %s not found", id)
	}

	return mp.toPublic(), nil
}

// List returns all registered and tracked plugin processes.
func (l *Launcher) List() []*PluginProcess {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*PluginProcess, 0, len(l.processes))
	for _, mp := range l.processes {
		result = append(result, mp.toPublic())
	}
	return result
}

func (mp *managedProcess) toPublic() *PluginProcess {
	return &PluginProcess{
		ID:        mp.id,
		Name:      mp.name,
		ExecPath:  mp.execPath,
		Args:      mp.args,
		Env:       mp.env,
		State:     mp.state,
		PID:       mp.pid,
		ExitCode:  mp.exitCode,
		ErrorMsg:  mp.errorMsg,
		StartedAt: mp.startedAt,
	}
}
