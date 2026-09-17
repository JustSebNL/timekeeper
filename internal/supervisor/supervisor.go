package supervisor

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/JustSebNL/timekeeper/internal/model"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	"sync"
	"time"
)

type Config struct {
	Project, Sprint, Agent, Model, StateDir string
	Stall, Poll                             time.Duration
}
type Checkpoint struct {
	Project            string                   `json:"project"`
	Sprint             string                   `json:"sprint"`
	Agent              string                   `json:"agent"`
	Session            string                   `json:"session"`
	Thread             string                   `json:"thread,omitempty"`
	State              string                   `json:"state"`
	Reason             string                   `json:"reason"`
	Usage              model.UsageSnapshotInput `json:"usage"`
	PendingNudge       int64                    `json:"pending_nudge,omitempty"`
	UsagePersistedTurn int64                    `json:"usage_persisted_turn"`
	UsageKnown         bool                     `json:"usage_known"`
}
type event struct {
	Type   string `json:"type"`
	Thread string `json:"thread_id"`
	Usage  *struct {
		Input   int64 `json:"input_tokens"`
		Output  int64 `json:"output_tokens"`
		Cached  int64 `json:"cached_input_tokens"`
		Written int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// Run owns exactly one launched process tree. It never attaches to arbitrary PIDs.
func Run(ctx context.Context, c Client, cfg Config, cmd *exec.Cmd, in io.Reader, out, errOut io.Writer) error {
	if cfg.Project == "" || cfg.Sprint == "" || cfg.Agent == "" || cfg.Model == "" || cfg.StateDir == "" || cfg.Stall <= 0 {
		return errors.New("project, sprint, agent, model, state directory and positive stall timeout are required")
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 2 * time.Second
	}
	u, err := url.Parse(c.Base)
	if err != nil || u.Scheme != "http" || u.User != nil || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return errors.New("supervision requires a numeric-loopback TimeKeeper URL")
	}
	sp, err := c.find(ctx, cfg.Project, cfg.Sprint)
	if err != nil {
		return err
	}
	if sp.Status != "Open" && sp.Status != "On Hold" {
		return fmt.Errorf("Sprint must be Open or On Hold, got %s; refusing to take over active work", sp.Status)
	}
	ceiling := 2*sp.EstimatedDurationSeconds + sp.BufferDurationSeconds - sp.ActiveDurationSeconds
	if sp.EstimatedDurationSeconds <= 0 || ceiling <= 0 {
		return errors.New("Sprint execution budget exhausted")
	}
	if err := os.MkdirAll(cfg.StateDir, 0700); err != nil {
		return err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	session := "supervised-" + hex.EncodeToString(nonce)
	cfg.Agent += "-" + hex.EncodeToString(nonce)
	cp := Checkpoint{Project: cfg.Project, Sprint: cfg.Sprint, Agent: cfg.Agent, Session: session, State: "starting", Usage: model.UsageSnapshotInput{SessionID: session, AgentID: cfg.Agent, Model: cfg.Model, SprintID: cfg.Sprint}}
	save := func() error {
		b, e := json.MarshalIndent(cp, "", "  ")
		if e != nil {
			return e
		}
		name := filepath.Join(cfg.StateDir, session+".json")
		tmp := name + ".tmp"
		if err := os.WriteFile(tmp, b, 0600); err != nil {
			return err
		}
		return os.Rename(tmp, name)
	}
	if err := save(); err != nil {
		return err
	}
	fmt.Fprintln(errOut, "TimeKeeper recovery checkpoint:", filepath.Join(cfg.StateDir, session+".json"))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	notices := make(chan model.PulseNudge, 1)
	token := hex.EncodeToString(nonce)
	var mu sync.Mutex
	received := map[int64]bool{}
	callback := &http.Server{ReadHeaderTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/recover/"+token {
			http.NotFound(w, r)
			return
		}
		var signal model.PulseGuardianSignal
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&signal) != nil || signal.Format != "timekeeper-pulse-guardian/v1" || signal.Action != "recover_attention" || signal.Nudge.AgentID != cfg.Agent || signal.Nudge.ActiveSprintID != cfg.Sprint || signal.Nudge.NudgeID < 1 {
			http.Error(w, "invalid owning-agent signal", 400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if !received[signal.Nudge.NudgeID] {
			select {
			case notices <- signal.Nudge:
				received[signal.Nudge.NudgeID] = true
			default:
				http.Error(w, "notification queue full", 503)
				return
			}
		}
		w.Header().Set("X-Timekeeper-Pulse-Accepted", "v1")
		w.WriteHeader(202)
	})}
	go callback.Serve(ln)
	defer callback.Close()
	path := "/api/v1/sprints/" + url.PathEscape(cfg.Sprint)
	var allowance model.SupervisedLaunch
	if err := c.call(ctx, "POST", path+"/supervised-launches", model.SupervisedLaunchInput{SessionID: session, AgentID: cfg.Agent}, &allowance); err != nil {
		return err
	}
	if allowance.RemainingSeconds < ceiling {
		ceiling = allowance.RemainingSeconds
	}
	held := false
	hold := func(reason string) error {
		cp.State = "hold_pending"
		cp.Reason = reason
		if err := save(); err != nil {
			return err
		}
		if err := c.call(context.Background(), "POST", path+"/hold", map[string]string{"reason": reason}, nil); err != nil {
			return err
		}
		held = true
		cp.State = "held"
		return save()
	}
	callbackURL := "http://" + ln.Addr().String() + "/recover/" + token
	progressPath := "/api/v1/agents/" + url.PathEscape(cfg.Agent) + "/progress"
	lease := int64(cfg.Stall / time.Second)
	if lease < 1 {
		lease = 1
	}
	if err := c.call(ctx, "POST", progressPath, model.AgentPulseProgressInput{ActiveSprintID: &cfg.Sprint, GuardianURL: &callbackURL, LeaseDurationSeconds: lease}, nil); err != nil {
		_ = hold("Guardian registration failed")
		return err
	}
	defer func() {
		empty := ""
		if err := c.call(context.Background(), "POST", progressPath, model.AgentPulseProgressInput{ActiveSprintID: &empty, GuardianURL: &empty, LeaseDurationSeconds: lease}, nil); err != nil {
			fmt.Fprintln(errOut, "Guardian unregister failed:", err)
		}
	}()
	configureProcess(cmd)
	cmd.Stdin = in
	cmd.Stderr = errOut
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = hold("Process pipe failed")
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = hold("Codex launch failed")
		return err
	}
	cleanup, err := containProcess(cmd)
	if err != nil {
		_ = stopProcess(cmd)
		_ = cmd.Wait()
		_ = hold("Process containment failed")
		return err
	}
	var cleanupOnce sync.Once
	cleanupTree := func() { cleanupOnce.Do(cleanup) }
	defer cleanupTree()
	cp.State = "running"
	if err := save(); err != nil {
		_ = stopProcess(cmd)
		_ = cmd.Wait()
		_ = hold("Checkpoint failed after launch")
		return err
	}
	events := make(chan event, 16)
	scanDone := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(pipe)
		sc.Buffer(make([]byte, 64<<10), 8<<20)
		for sc.Scan() {
			line := append([]byte(nil), sc.Bytes()...)
			fmt.Fprintln(out, string(line))
			var e event
			if json.Unmarshal(line, &e) == nil && (e.Type == "turn.completed" || e.Type == "thread.started" || e.Type == "item.completed") {
				select {
				case events <- e:
				case <-ctx.Done():
					scanDone <- ctx.Err()
					return
				}
			}
		}
		scanDone <- sc.Err()
	}()
	done := make(chan error, 1)
	go func() {
		scanErr := <-scanDone
		waitErr := cmd.Wait()
		if scanErr != nil {
			done <- scanErr
		} else {
			done <- waitErr
		}
	}()
	poll := time.NewTicker(cfg.Poll)
	defer poll.Stop()
	hard := time.NewTimer(time.Duration(ceiling) * time.Second)
	defer hard.Stop()
	idle := time.NewTimer(cfg.Stall)
	defer idle.Stop()
	resetIdle := func() {
		if !idle.Stop() {
			select {
			case <-idle.C:
			default:
			}
		}
		idle.Reset(cfg.Stall)
	}
	record := func(e event) error {
		if e.Thread != "" {
			cp.Thread = e.Thread
		}
		if e.Usage != nil {
			v := e.Usage
			if v.Input < 0 || v.Output < 0 || v.Cached < 0 || v.Cached > v.Input || v.Written < 0 {
				return errors.New("invalid Codex usage event")
			}
			if cp.Usage.TurnSeq == math.MaxInt64 || v.Input > math.MaxInt64-cp.Usage.InputTokens || v.Output > math.MaxInt64-cp.Usage.OutputTokens || v.Cached > math.MaxInt64-cp.Usage.CacheReadTokens || v.Written > math.MaxInt64-cp.Usage.CacheCreationTokens {
				return errors.New("usage counters overflow")
			}
			cp.Usage.TurnSeq++
			cp.Usage.InputTokens += v.Input - v.Cached
			cp.Usage.OutputTokens += v.Output
			cp.Usage.CacheReadTokens += v.Cached
			cp.Usage.CacheCreationTokens += v.Written
			cp.UsageKnown = true
			if err := save(); err != nil {
				return err
			}
			if err := c.call(ctx, "POST", "/api/v1/projects/"+url.PathEscape(cfg.Project)+"/usage-sessions/"+session+"/snapshots", cp.Usage, nil); err != nil {
				return err
			}
			cp.UsagePersistedTurn = cp.Usage.TurnSeq
		}
		return save()
	}
	stop := func(reason string, nudge *model.PulseNudge) error {
		fmt.Fprintln(errOut, "TimeKeeper Guardian:", reason, "; stopping owned Codex process tree.")
		if nudge != nil {
			cp.PendingNudge = nudge.NudgeID
		}
		cp.State = "stopping"
		cp.Reason = reason
		_ = save()
		cleanupTree()
		killErr := stopProcess(cmd)
		timeout := time.NewTimer(10 * time.Second)
		defer timeout.Stop()
	stopping:
		for {
			select {
			case e := <-events:
				if err := record(e); err != nil {
					fmt.Fprintln(errOut, "Usage reconciliation pending:", err)
				}
			case <-done:
				for len(events) > 0 {
					if err := record(<-events); err != nil {
						fmt.Fprintln(errOut, "Usage reconciliation pending:", err)
					}
				}
				break stopping
			case <-timeout.C:
				cp.State = "stop_unconfirmed"
				_ = save()
				return errors.New("Codex stop not confirmed; nudge remains unacknowledged")
			}
		}
		if killErr != nil {
			fmt.Fprintln(errOut, "Process stop returned:", killErr)
		}
		if err := hold(reason); err != nil {
			return fmt.Errorf("process stopped; durable hold pending: %w", err)
		}
		if nudge != nil {
			if err := c.call(context.Background(), "POST", "/api/v1/agents/"+url.PathEscape(cfg.Agent)+"/nudges/"+fmt.Sprint(nudge.NudgeID)+"/ack", map[string]string{}, nil); err != nil {
				return err
			}
		}
		cp.PendingNudge = 0
		if err := save(); err != nil {
			return err
		}
		return errors.New(reason)
	}
	for {
		select {
		case e := <-events:
			if err := record(e); err != nil {
				return stop("usage/checkpoint persistence failed: "+err.Error(), nil)
			}
			if e.Type == "item.completed" || e.Type == "turn.completed" {
				resetIdle()
				if err := c.call(ctx, "POST", progressPath, model.AgentPulseProgressInput{LeaseDurationSeconds: lease}, nil); err != nil {
					return stop("progress tracking unavailable", nil)
				}
			}
		case n := <-notices:
			return stop("Guardian "+n.Kind, &n)
		case <-idle.C:
			return stop("no completed Codex work within stall timeout", nil)
		case <-hard.C:
			return stop("maximum Sprint execution budget exhausted", nil)
		case <-ctx.Done():
			return stop("supervision interrupted", nil)
		case <-poll.C:
			var pulse model.Pulse
			if err := c.call(ctx, "GET", "/api/v1/pulse", nil, &pulse); err != nil {
				return stop("TimeKeeper tracking unavailable", nil)
			}
			for _, a := range pulse.Attention {
				if a.SprintID == cfg.Sprint {
					return stop("Sprint deadline exceeded; extend explicitly before resuming", nil)
				}
			}
		case err := <-done:
			for len(events) > 0 {
				if e := <-events; record(e) != nil {
					err = errors.New("usage persistence failed")
				}
			}
			reason := "Codex exited; acceptance verification required"
			if err != nil {
				reason = "Codex failed: " + err.Error()
			}
			if !held {
				if e := hold(reason); e != nil {
					return e
				}
			}
			return err
		}
	}
}
