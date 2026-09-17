package store

import (
	"context"
	"fmt"
	"github.com/JustSebNL/timekeeper/internal/model"
	"path/filepath"
	"testing"
)

func TestSupervisedTaskCannotResetLaunchAllowance(t *testing.T) {
	db, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx := context.Background()
	p, _ := db.CreateProject(ctx, model.CreateProjectInput{Name: "test"})
	c, _ := db.CreateCategory(ctx, p.ProjectID, model.CreateCategoryInput{Name: "c"})
	task, _ := db.CreateTask(ctx, p.ProjectID, model.CreateTaskInput{CategoryID: c.CategoryID, Name: "t", EstimatedDurationSeconds: 60})
	var sp model.Sprint
	for i := 0; i < 5; i++ {
		if i < 4 {
			sp, e = db.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: fmt.Sprintf("replacement-%d", i), EstimatedDurationSeconds: 1000})
			if e != nil {
				t.Fatal(e)
			}
		}
		v, e := db.ReserveSupervisedLaunch(ctx, sp.SprintID, model.SupervisedLaunchInput{SessionID: fmt.Sprint(i), AgentID: "test"})
		if i == 4 {
			if e == nil {
				t.Fatal("replacement reset exhausted task allowance")
			}
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		if v.Attempts != i+1 || v.RemainingSeconds > 120 {
			t.Fatalf("allowance %+v", v)
		}
		_, e = db.TransitionSprint(ctx, sp.SprintID, "hold", "test completed")
		if e != nil {
			t.Fatal(e)
		}
	}
}
