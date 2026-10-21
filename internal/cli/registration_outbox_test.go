package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestRegistrationOutboxStacksAndPersistsOffers(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", stateDir)
	firstBody := []byte(`{"tasks":[{"name":"first"}]}`)
	secondBody := []byte(`{"tasks":[{"name":"second"}]}`)

	first, err := enqueueRegistrationOutbox("P-10000", "task_batch", "/api/v1/projects/P-10000/execution-batches/validate", "/api/v1/projects/P-10000/execution-batches", firstBody)
	if err != nil {
		t.Fatal(err)
	}
	second, err := enqueueRegistrationOutbox("P-10000", "task_batch", "/api/v1/projects/P-10000/execution-batches/validate", "/api/v1/projects/P-10000/execution-batches", secondBody)
	if err != nil {
		t.Fatal(err)
	}
	if first.Offer.LocalID == second.Offer.LocalID || first.Offer.IdempotencyKey == second.Offer.IdempotencyKey {
		t.Fatal("stacked offers must have distinct durable and Core idempotency IDs")
	}

	entries, err := listRegistrationOutbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Offer.LocalID != first.Offer.LocalID || entries[1].Offer.LocalID != second.Offer.LocalID {
		t.Fatalf("outbox order=%+v", entries)
	}
	if entries[0].State.Status != registrationOutboxPending || entries[1].State.Status != registrationOutboxPending {
		t.Fatalf("new outbox statuses=%q,%q", entries[0].State.Status, entries[1].State.Status)
	}
	if !bytes.Equal(entries[0].Offer.Body, firstBody) || !bytes.Equal(entries[1].Offer.Body, secondBody) {
		t.Fatal("outbox did not preserve the exact normalized API payloads")
	}

	reloaded, err := loadRegistrationOutboxEntry(first.Offer.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reloaded.Offer.Body, firstBody) || reloaded.Offer.IdempotencyKey != first.Offer.IdempotencyKey {
		t.Fatalf("reloaded outbox offer=%+v", reloaded.Offer)
	}
}

func TestRegistrationOutboxConcurrentOffersRemainFIFOAndDistinct(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	const offerCount = 12
	var workers sync.WaitGroup
	errorsFound := make(chan error, offerCount)
	for index := 0; index < offerCount; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			body := []byte(fmt.Sprintf(`{"name":"concurrent-%02d"}`, index))
			_, err := enqueueRegistrationOutbox("P-10000", "task", "", "/api/v1/projects/P-10000/tasks", body)
			if err != nil {
				errorsFound <- err
			}
		}(index)
	}
	workers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	entries, err := listRegistrationOutbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != offerCount {
		t.Fatalf("concurrent local offers=%d, want %d", len(entries), offerCount)
	}
	keys := make(map[string]struct{}, offerCount)
	for index, entry := range entries {
		if _, exists := keys[entry.Offer.IdempotencyKey]; exists {
			t.Fatalf("duplicate idempotency key %q", entry.Offer.IdempotencyKey)
		}
		keys[entry.Offer.IdempotencyKey] = struct{}{}
		if index > 0 && !entries[index-1].Offer.CreatedAt.Before(entry.Offer.CreatedAt) {
			t.Fatalf("local offers are not strictly FIFO at %d: %s then %s", index, entries[index-1].Offer.CreatedAt, entry.Offer.CreatedAt)
		}
	}
}

func TestRegistrationOutboxRejectsUnsafeRequestPaths(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	for _, path := range []string{"https://example.invalid/collect", "/api/v1/../tasks", "/api/v1/projects/P/tasks?redirect=http://evil.invalid"} {
		if _, err := enqueueRegistrationOutbox("P-10000", "task", "", path, []byte(`{"name":"safe"}`)); err == nil {
			t.Errorf("unsafe registration path %q was accepted", path)
		}
	}
	entries, err := listRegistrationOutbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unsafe paths created local offers: %+v", entries)
	}
}

func TestTaskSubtaskAndSprintCLIOffersPersistLocallyWhenCoreIsUnavailable(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	baseURL := "http://127.0.0.1:1"
	var out, errOut bytes.Buffer
	if code := task([]string{"new", "P-10000", "C-10000", "offline task", "30m"}, &out, &errOut, baseURL); code != 0 {
		t.Fatalf("offline Task offer code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if code := subtask([]string{"new", "T-10000", "offline subtask", "30m"}, &out, &errOut, baseURL); code != 0 {
		t.Fatalf("offline Subtask offer code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if code := sprint([]string{"new", "task", "T-10000", "offline sprint", "30m"}, &out, &errOut, baseURL); code != 0 {
		t.Fatalf("offline Sprint offer code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	entries, err := listRegistrationOutbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("local outbox offers=%d, want Task/Subtask/Sprint: %+v", len(entries), entries)
	}
	wantKinds := []string{"task", "subtask", "sprint"}
	wantPaths := []string{
		"/api/v1/projects/P-10000/tasks",
		"/api/v1/tasks/T-10000/subtasks",
		"/api/v1/tasks/T-10000/sprints",
	}
	for index, entry := range entries {
		if entry.Offer.Kind != wantKinds[index] || entry.Offer.SubmitPath != wantPaths[index] || entry.State.Status != registrationOutboxPending || entry.Offer.IdempotencyKey == "" {
			t.Errorf("entry[%d]=%+v", index, entry)
		}
	}
}

func TestRegistrationOutboxStateTransitionsAreAppendOnly(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	entry, err := enqueueRegistrationOutbox("P-10053", "execution_batch", "/api/v1/projects/P-10053/execution-batches/validate", "/api/v1/projects/P-10053/execution-batches", []byte(`{"version":"timekeeper-execution-batch/v1"}`))
	if err != nil {
		t.Fatal(err)
	}
	updated := entry.State
	updated.Status = registrationOutboxAwaitingCore
	updated.CoreQueueID = "RQ-0123456789abcdef"
	updated.Attempts = 1
	updated.LastError = "Core accepted; worker has not completed registration yet"
	updated.UpdatedAt = time.Now().UTC()
	if err := appendRegistrationOutboxState(entry.Offer.LocalID, updated); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadRegistrationOutboxEntry(entry.Offer.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State.Status != registrationOutboxAwaitingCore || reloaded.State.CoreQueueID != updated.CoreQueueID || reloaded.State.Attempts != 1 {
		t.Fatalf("latest outbox state=%+v", reloaded.State)
	}
	stateFiles, err := filepath.Glob(filepath.Join(registrationOutboxItemDir(entry.Offer.LocalID), "state", "*.json"))
	if err != nil || len(stateFiles) != 2 {
		t.Fatalf("append-only state files=%v err=%v", stateFiles, err)
	}
	for _, path := range stateFiles {
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			t.Fatalf("state evidence missing: %s err=%v", path, err)
		}
	}
}

func TestTaskJSONOfflineOfferOutputsOnlyLocalReceiptJSON(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	code := Run([]string{"--json", "t", "new", "P-10000", "C-10000", "offline task", "30m"}, &out, &errOut, "http://127.0.0.1:1")
	if code != 0 {
		t.Fatalf("offline JSON Task offer code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	var receipt struct {
		LocalQueueID string `json:"local_queue_id"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(out.Bytes(), &receipt); err != nil {
		t.Fatalf("offline offer stdout must be one JSON receipt: %v; raw=%q", err, out.String())
	}
	if receipt.LocalQueueID == "" || receipt.Status != registrationOutboxPending {
		t.Fatalf("offline JSON receipt=%+v", receipt)
	}
}

func TestRunDoesNotLeakJSONModeIntoLaterCommands(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	if code := Run([]string{"--json", "queue", "list"}, &out, &errOut, "http://timekeeper.local"); code != 0 {
		t.Fatalf("JSON queue list code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	if jsonOutputEnabled {
		t.Fatal("JSON output mode leaked beyond one CLI invocation")
	}
}

func TestRunQueueListShowsDurableLocalRegistrationOffers(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	entry, err := enqueueRegistrationOutbox("P-10000", "task", "", "/api/v1/projects/P-10000/tasks", []byte(`{"name":"local queue task"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run([]string{"queue", "list"}, &out, &errOut, "http://timekeeper.local")
	if code != 0 {
		t.Fatalf("queue list code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), entry.Offer.LocalID) || !strings.Contains(out.String(), registrationOutboxPending) || !strings.Contains(out.String(), "task") {
		t.Fatalf("queue list omitted durable offer: %s", out.String())
	}
}

func TestRunQueueFlushDeliversLocalOfferWithStableIdempotencyKey(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	var receivedKey string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/projects/P-10000/tasks" {
			t.Fatalf("unexpected flush request %s %s", request.Method, request.URL.Path)
		}
		receivedKey = request.Header.Get("Idempotency-Key")
		writer.Header().Set("X-TimeKeeper-Registration-Queue-ID", "RQ-abcdef0123456789")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"task_id":"T-10001","name":"offline task"}`))
	}))
	defer server.Close()
	offer, err := enqueueRegistrationOutbox("P-10000", "task", "", "/api/v1/projects/P-10000/tasks", []byte(`{"name":"offline task"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run([]string{"queue", "flush"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("queue flush code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	if receivedKey != offer.Offer.IdempotencyKey {
		t.Fatalf("flush Idempotency-Key=%q, want %q", receivedKey, offer.Offer.IdempotencyKey)
	}
	reloaded, err := loadRegistrationOutboxEntry(offer.Offer.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State.Status != registrationOutboxRegistered || reloaded.State.CoreQueueID != "RQ-abcdef0123456789" {
		t.Fatalf("flushed receipt=%+v", reloaded.State)
	}
	if !strings.Contains(out.String(), registrationOutboxRegistered) {
		t.Fatalf("queue flush omitted registered outcome: %s", out.String())
	}
}

func TestRegistrationOutboxFlushCreatesTaskSubtaskAndSprintInCore(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "registration-outbox-integration.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Registration outbox integration"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Outbox integration"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.New(database))
	defer server.Close()
	var out, errOut bytes.Buffer
	if code := task([]string{"new", project.ProjectID, category.CategoryID, "Outbox Task", "30m"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("create Task code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	tasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("registered Tasks=%d err=%v", len(tasks), err)
	}
	if code := subtask([]string{"new", tasks[0].TaskID, "Outbox Subtask", "30m"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("create Subtask code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	subtasks, err := database.ListSubtasks(ctx, tasks[0].TaskID)
	if err != nil || len(subtasks) != 1 {
		t.Fatalf("registered Subtasks=%d err=%v", len(subtasks), err)
	}
	if code := sprint([]string{"new", "subtask", subtasks[0].SubtaskID, "Outbox Sprint", "30m"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("create Sprint code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	queue, err := database.ListRegistrationQueue(ctx, project.ProjectID, "", 10)
	if err != nil || len(queue) != 3 {
		t.Fatalf("Core registration queue=%+v err=%v", queue, err)
	}
	wantKinds := []string{"task", "subtask", "sprint"}
	for index, item := range queue {
		if item.Kind != wantKinds[index] || item.Status != model.RegistrationQueueStatusRegistered {
			t.Errorf("Core queue item[%d]=%+v", index, item)
		}
	}
	outbox, err := listRegistrationOutbox()
	if err != nil || len(outbox) != 3 {
		t.Fatalf("local outbox=%+v err=%v", outbox, err)
	}
	for index, item := range outbox {
		if item.State.Status != registrationOutboxRegistered || item.State.CoreQueueID == "" || item.Offer.IdempotencyKey == "" {
			t.Errorf("local receipt[%d]=%+v", index, item)
		}
	}
}

func TestFlushRegistrationOutboxValidatesThenSubmitsWithStableIdempotencyKey(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	var validationCalls, submitCalls int
	var applyKey string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/projects/P-10000/execution-batches/validate":
			validationCalls++
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"valid":true,"counts":{"tasks":1,"subtasks":0,"sprints":0,"total_objects":1},"issues":[]}`))
		case "/api/v1/projects/P-10000/execution-batches":
			submitCalls++
			applyKey = request.Header.Get("Idempotency-Key")
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"tasks":[{"task_id":"T-10001","name":"queued"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	validationPath := "/api/v1/projects/P-10000/execution-batches/validate"
	submitPath := "/api/v1/projects/P-10000/execution-batches"
	offer, err := enqueueRegistrationOutbox("P-10000", "task_batch", validationPath, submitPath, []byte(`{"tasks":[{"name":"queued"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	state, err := flushRegistrationOutboxEntry(offer, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != registrationOutboxRegistered || validationCalls != 1 || submitCalls != 1 {
		t.Fatalf("status=%s validationCalls=%d submitCalls=%d", state.Status, validationCalls, submitCalls)
	}
	if applyKey != offer.Offer.IdempotencyKey {
		t.Fatalf("Idempotency-Key=%q, want stable outbox key %q", applyKey, offer.Offer.IdempotencyKey)
	}
	reloaded, err := loadRegistrationOutboxEntry(offer.Offer.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State.Status != registrationOutboxRegistered || string(reloaded.State.Result) == "" {
		t.Fatalf("registered state was not durable: %+v", reloaded.State)
	}
}

func TestFlushRegistrationOutboxKeepsRejectedSmokeOfferWithoutApplying(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	var applyCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(request.URL.Path, "/validate") {
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"valid":false,"counts":{"tasks":0,"subtasks":0,"sprints":0,"total_objects":0},"issues":[{"path":"tasks[0]","field":"name","code":"required","message":"name is required","suggestion":"Provide a name."}]}`))
			return
		}
		applyCalls++
		writer.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	offer, err := enqueueRegistrationOutbox("P-10000", "execution_batch", "/api/v1/projects/P-10000/execution-batches/validate", "/api/v1/projects/P-10000/execution-batches", []byte(`{"version":"timekeeper-execution-batch/v1"}`))
	if err != nil {
		t.Fatal(err)
	}
	state, err := flushRegistrationOutboxEntry(offer, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != registrationOutboxRejected || applyCalls != 0 || !strings.Contains(state.LastError, "rejected") {
		t.Fatalf("state=%+v applyCalls=%d", state, applyCalls)
	}
}

func TestFlushRegistrationOutboxPollsAcceptedCoreQueueBeforeRetrying(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	var applyCalls, statusCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			applyCalls++
			writer.Header().Set("X-TimeKeeper-Registration-Queue-ID", "RQ-0123456789abcdef")
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte(`{"queue_id":"RQ-0123456789abcdef","project_id":"P-10000","kind":"task","status":"Queued"}`))
			return
		}
		statusCalls++
		if request.URL.Path != "/api/v1/projects/P-10000/registration-queue/RQ-0123456789abcdef" {
			t.Errorf("unexpected queue status path %q", request.URL.Path)
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"queue_id":"RQ-0123456789abcdef","project_id":"P-10000","kind":"task","status":"Registered"}`))
	}))
	defer server.Close()
	offer, err := enqueueRegistrationOutbox("P-10000", "task", "", "/api/v1/projects/P-10000/tasks", []byte(`{"name":"offline"}`))
	if err != nil {
		t.Fatal(err)
	}
	first, err := flushRegistrationOutboxEntry(offer, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != registrationOutboxAwaitingCore || first.CoreQueueID != "RQ-0123456789abcdef" {
		t.Fatalf("accepted state=%+v", first)
	}
	reloaded, err := loadRegistrationOutboxEntry(offer.Offer.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := flushRegistrationOutboxEntry(reloaded, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != registrationOutboxRegistered || applyCalls != 1 || statusCalls != 1 {
		t.Fatalf("final state=%+v applyCalls=%d statusCalls=%d", second, applyCalls, statusCalls)
	}
}

func TestFlushRegistrationOutboxKeepsOfferPendingWhenCoreIsUnavailable(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
	offer, err := enqueueRegistrationOutbox("P-10000", "task", "", "/api/v1/projects/P-10000/tasks", []byte(`{"name":"offline"}`))
	if err != nil {
		t.Fatal(err)
	}
	state, err := flushRegistrationOutboxEntry(offer, "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected a transport failure")
	}
	if state.Status != registrationOutboxPending || state.Attempts != 1 || state.LastError == "" {
		t.Fatalf("failed delivery state=%+v", state)
	}
	reloaded, err := loadRegistrationOutboxEntry(offer.Offer.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Offer.IdempotencyKey != offer.Offer.IdempotencyKey || reloaded.State.Status != registrationOutboxPending {
		t.Fatalf("outbox did not retain retryable offer: %+v", reloaded)
	}
}
