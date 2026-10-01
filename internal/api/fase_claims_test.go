package api_test

import (
	"bytes"
	"encoding/json"
	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFASEAPIRejectsCompetingAndAnonymousExecution(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "claims.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	p, e := s.CreateProject(t.Context(), model.CreateProjectInput{Name: "claims"})
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.CreateCategory(t.Context(), p.ProjectID, model.CreateCategoryInput{Name: "work"})
	if e != nil {
		t.Fatal(e)
	}
	task, e := s.CreateTask(t.Context(), p.ProjectID, model.CreateTaskInput{Name: "task", CategoryID: c.CategoryID})
	if e != nil {
		t.Fatal(e)
	}
	sp, e := s.CreateSprint(t.Context(), task.TaskID, model.CreateSprintInput{Name: "step", EstimatedDurationSeconds: 60})
	if e != nil {
		t.Fatal(e)
	}
	ids := []model.FASEIdentity{}
	for _, f := range []string{"a", "b"} {
		id, e := s.ResolveFASEIdentityAt(t.Context(), model.FASEIdentityInput{Fingerprint: strings.Repeat(f, 64), Runtime: "test", IdentityScope: "runtime-session"}, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, id)
	}
	h := api.New(s)
	call := func(index int, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		if index >= 0 {
			r.Header.Set("X-Agent-ID", ids[index].AgentID)
			r.Header.Set("X-Agent-Session-ID", model.FASEActivitySession(strings.Repeat([]string{"a", "b"}[index], 64)))
			r.Header.Set("X-Agent-Runtime", "test")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call(0, "POST", "/api/v1/sprints/"+sp.SprintID+"/start", "{}"); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, index := range []int{1, -1} {
		w := call(index, "POST", "/api/v1/sprints/"+sp.SprintID+"/complete", "{}")
		if w.Code != 409 || !strings.Contains(w.Body.String(), "item_already_claimed") {
			t.Fatalf("claim bypass: %d %s", w.Code, w.Body)
		}
	}
	w := call(0, "GET", "/api/v1/projects/"+p.ProjectID+"/execution-claims", "")
	var claims struct {
		Items []model.FASEClaim `json:"items"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &claims) != nil || len(claims.Items) != 2 {
		t.Fatalf("claims unavailable: %d %s", w.Code, w.Body)
	}
	events, e := s.ListProjectEvents(t.Context(), p.ProjectID)
	if e != nil {
		t.Fatal(e)
	}
	if events[0].Actor == nil || events[0].Actor.AgentKey != ids[0].AgentKey {
		t.Fatalf("caller not attributed: %+v", events[0])
	}
}
