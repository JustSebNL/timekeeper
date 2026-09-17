package supervisor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"github.com/JustSebNL/timekeeper/internal/supervisor"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorker(t *testing.T) {
	mode := os.Getenv("TK_SUPERVISOR_TEST_WORKER")
	if mode == "" {
		return
	}
	if mode == "success" {
		fmt.Println(`{"type":"thread.started","thread_id":"fixture-thread"}`)
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":20,"cached_input_tokens":50}}`)
		os.Exit(0)
	}
	if mode == "flood" {
		for i := 0; i < 200; i++ {
			fmt.Println(`{"type":"item.completed"}`)
		}
	}
	for {
		time.Sleep(time.Second)
	}
}
func TestOwnedProcessLifecycle(t *testing.T) {
	for _, mode := range []string{"success", "stall", "flood", "guardian", "outage"} {
		t.Run(mode, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ctx := context.Background()
			p, err := db.CreateProject(ctx, model.CreateProjectInput{Name: "test"})
			if err != nil {
				t.Fatal(err)
			}
			c, err := db.CreateCategory(ctx, p.ProjectID, model.CreateCategoryInput{Name: "c"})
			if err != nil {
				t.Fatal(err)
			}
			task, err := db.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{CategoryID: c.CategoryID, Name: "t"})
			if err != nil {
				t.Fatal(err)
			}
			sp, err := db.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "s", EstimatedDurationSeconds: 60})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(api.New(db))
			defer server.Close()
			cmd := exec.Command(os.Args[0], "-test.run=^TestWorker$")
			cmd.Env = append(os.Environ(), "TK_SUPERVISOR_TEST_WORKER="+mode)
			state := t.TempDir()
			var stderr bytes.Buffer
			stall := 300 * time.Millisecond
			if mode == "success" || mode == "guardian" || mode == "outage" {
				stall = 5 * time.Second
			}
			trigger := make(chan error, 1)
			if mode == "guardian" || mode == "outage" {
				go func() {
					until := time.Now().Add(5 * time.Second)
					for time.Now().Before(until) {
						regs, e := db.RegisteredGuardianURLs(ctx)
						if e != nil {
							trigger <- e
							return
						}
						if len(regs) > 0 {
							time.Sleep(100 * time.Millisecond)
							if mode == "outage" {
								server.Close()
								trigger <- nil
								return
							}
							result, e := guardian.Tick(ctx, db, time.Now().Add(2*time.Minute))
							if e == nil && result.Delivered != 1 {
								e = fmt.Errorf("delivery %+v", result)
							}
							trigger <- e
							return
						}
						time.Sleep(20 * time.Millisecond)
					}
					trigger <- fmt.Errorf("no callback registration")
				}()
			}
			err = supervisor.Run(ctx, supervisor.Client{Base: server.URL}, supervisor.Config{Project: p.ProjectID, Sprint: sp.SprintID, Agent: "fixture", Model: "fixture-model", StateDir: state, Stall: stall, Poll: 50 * time.Millisecond}, cmd, nil, io.Discard, &stderr)
			if mode == "success" && err != nil {
				t.Fatalf("%v %s", err, &stderr)
			}
			if (mode == "stall" || mode == "flood") && (err == nil || !strings.Contains(err.Error(), "stall timeout")) {
				t.Fatalf("expected stall error, got %v %s", err, &stderr)
			}
			if mode == "guardian" || mode == "outage" {
				if e := <-trigger; e != nil {
					t.Fatal(e)
				}
				if err == nil {
					t.Fatal("expected interrupted run")
				}
			}
			if mode == "guardian" {
				regs, _ := filepath.Glob(filepath.Join(state, "*.json"))
				b, _ := os.ReadFile(regs[0])
				var checkpoint supervisor.Checkpoint
				_ = json.Unmarshal(b, &checkpoint)
				history, e := db.ListPulseNudgeHistory(ctx, checkpoint.Agent)
				if e != nil || len(history) != 1 || history[0].Status != "Acknowledged" {
					t.Fatalf("recovery not acknowledged: %+v %v", history, e)
				}
			}
			if cmd.ProcessState == nil {
				t.Fatal("process exit unconfirmed")
			}
			files, _ := filepath.Glob(filepath.Join(state, "*.json"))
			if len(files) != 1 {
				t.Fatalf("checkpoints %v", files)
			}
			b, _ := os.ReadFile(files[0])
			var cp supervisor.Checkpoint
			if err := json.Unmarshal(b, &cp); err != nil {
				t.Fatal(err)
			}
			if (mode != "outage" && cp.State != "held") || (mode == "outage" && cp.State != "hold_pending") {
				t.Fatalf("checkpoint %s", b)
			}
			if mode == "success" {
				if !cp.UsageKnown || cp.Usage.InputTokens != 50 || cp.Usage.CacheReadTokens != 50 || cp.Thread != "fixture-thread" {
					t.Fatalf("usage %s", b)
				}
				outcome, _, err := db.RecordCodingOutcome(ctx, p.ProjectID, model.CodingOutcomeInput{SessionID: cp.Session, AgentID: cp.Agent, Provider: "test", Category: "test", Difficulty: "test", Benchmark: "v1", Configuration: "test", Result: "accepted", Reviewer: "test", Evidence: "fixture"})
				if err != nil || outcome.InputTokens != 50 {
					t.Fatalf("usage not persisted: %+v %v", outcome, err)
				}
			} else if cp.UsageKnown {
				t.Fatal("invented usage for interrupted turn")
			}
			if mode != "outage" {
				nudges, e := db.EvaluatePulseGuardianAt(ctx, time.Now().Add(time.Hour))
				if e != nil || len(nudges) != 0 {
					t.Fatalf("retired worker nudged: %+v %v", nudges, e)
				}
			}

		})
	}
}
