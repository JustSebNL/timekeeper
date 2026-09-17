package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestBugReportRouteCreatesDurableReportIdempotently(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject(ctx, model.CreateProjectInput{Name: "Bug owner"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := db.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Defects"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := db.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Reporter"})
	if err != nil {
		t.Fatal(err)
	}
	sprint, err := db.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Reproduce", EstimatedDurationSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"project_id":"` + project.ProjectID + `","category_id":"` + category.CategoryID + `","task_id":"` + task.TaskID + `","sprint_id":"` + sprint.SprintID + `","title":"Sprint creation returns 400","severity":"P2","expected":"Valid sprint is accepted","actual":"API returns 400","reproduction_steps":["POST valid JSON"],"environment":{"component":"TimeKeeper API"},"source":{"agent_id":"test","session_id":"s1","command":"tk sp new task"},"evidence":[{"type":"command_output","ref":"M-1"}],"idempotency_key":"attempt-1","redaction_status":"verified"}`
	server := api.New(db)
	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bug-reports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(first, req)
	if first.Code != http.StatusCreated {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	var firstPayload struct {
		ReportID  string `json:"report_id"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := json.NewDecoder(first.Body).Decode(&firstPayload); err != nil {
		t.Fatal(err)
	}
	if firstPayload.ReportID == "" || firstPayload.Duplicate {
		t.Fatalf("first payload=%+v", firstPayload)
	}

	second := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/bug-reports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(second, req)
	if second.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", second.Code, second.Body.String())
	}
	var secondPayload struct {
		ReportID  string `json:"report_id"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := json.NewDecoder(second.Body).Decode(&secondPayload); err != nil {
		t.Fatal(err)
	}
	if secondPayload.ReportID != firstPayload.ReportID || !secondPayload.Duplicate {
		t.Fatalf("retry payload=%+v first=%+v", secondPayload, firstPayload)
	}

	events := httptest.NewRecorder()
	server.ServeHTTP(events, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/events", nil))
	if events.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", events.Code, events.Body.String())
	}
	statusBody := `{"project_id":"` + project.ProjectID + `","status":"Confirmed"}`
	statusResponse := httptest.NewRecorder()
	statusRequest := httptest.NewRequest(http.MethodPost, "/api/v1/bug-reports/"+firstPayload.ReportID+"/status", strings.NewReader(statusBody))
	statusRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("status transition=%d body=%s", statusResponse.Code, statusResponse.Body.String())
	}
}

func TestBugReportRouteAcceptsMarkdownEnvelope(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject(ctx, model.CreateProjectInput{Name: "Markdown bugs"})
	if err != nil {
		t.Fatal(err)
	}
	markdown := "---\nproject_id: " + project.ProjectID + "\nseverity: P2\nidempotency_key: markdown-1\nredaction_status: verified\nsource.agent_id: alpha-test\nsource.session_id: s-md\nsource.command: tk bug-report\n---\n# Markdown report\n\n## Expected\nThe alpha API accepts the request.\n\n## Actual\nThe alpha API rejects the request.\n\n## Reproduction\n- Start the local TimeKeeper API.\n- POST the documented request.\n\n## Environment\n- component: TimeKeeper API\n"
	server := api.New(db)
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bug-reports", strings.NewReader(markdown))
	req.Header.Set("Content-Type", "text/markdown")
	server.ServeHTTP(response, req)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestBugReportRouteRejectsUnknownFieldsAndSensitiveMaterial(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject(ctx, model.CreateProjectInput{Name: "Validation bugs"})
	if err != nil {
		t.Fatal(err)
	}
	server := api.New(db)
	base := `{"project_id":"` + project.ProjectID + `","title":"Bad report","severity":"P2","expected":"expected","actual":"actual","reproduction_steps":["step"],"source":{"agent_id":"test"},"idempotency_key":"validation-1","redaction_status":"verified"}`
	unknown := strings.TrimSuffix(base, "}") + `,"unexpected":true}`
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bug-reports", strings.NewReader(unknown))
	req.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", response.Code, response.Body.String())
	}

	sensitive := strings.TrimSuffix(base, "}") + `,"actual":"Authorization: Bearer secret"}`
	response = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/bug-reports", strings.NewReader(sensitive))
	req.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("sensitive status=%d body=%s", response.Code, response.Body.String())
	}

	invalidRef := strings.Replace(base, `"project_id":"`+project.ProjectID+`"`, `"project_id":"P-99999"`, 1)
	response = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/bug-reports", strings.NewReader(invalidRef))
	req.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid ref status=%d body=%s", response.Code, response.Body.String())
	}
}
