package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestGlobalSearchAPIReturnsAttributedResultsAndTruncation(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer database.Close()
	ctx := context.Background()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Queue Atlas"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Search"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Queue Worker"}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProjectMessage(ctx, project.ProjectID, model.CreateProjectMessageInput{Kind: model.MessageKindDecision, Body: "Queue offers must persist.", Tags: "durable"}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=queue&limit=2", nil)
	recorder := httptest.NewRecorder()
	New(database).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response model.GlobalSearchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Total != 3 || response.Limit != 2 || !response.HasMore || len(response.Items) != 2 {
		t.Fatalf("response summary=%+v", response)
	}
	for _, item := range response.Items {
		if item.ProjectID != project.ProjectID || item.ProjectName != "Queue Atlas" || item.ObjectID == "" || item.Ref == "" {
			t.Errorf("result attribution missing: %+v", item)
		}
	}
	filteredRequest := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=queue&scope=tasks&project="+project.ProjectID, nil)
	filteredRecorder := httptest.NewRecorder()
	New(database).ServeHTTP(filteredRecorder, filteredRequest)
	var filtered model.GlobalSearchResponse
	if filteredRecorder.Code != http.StatusOK || json.Unmarshal(filteredRecorder.Body.Bytes(), &filtered) != nil || filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].Type != "task" {
		t.Fatalf("filtered search status=%d response=%s parsed=%+v", filteredRecorder.Code, filteredRecorder.Body.String(), filtered)
	}
	invalidRequest := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=queue&scope=unknown", nil)
	invalidRecorder := httptest.NewRecorder()
	New(database).ServeHTTP(invalidRecorder, invalidRequest)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid scope status=%d body=%s", invalidRecorder.Code, invalidRecorder.Body.String())
	}
	today := time.Now().UTC().Format("2006-01-02")
	filteredMessageRequest := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=queue&scope=messages&project="+project.ProjectID+"&kind=decision&tags=durable&after="+today+"&before="+today, nil)
	filteredMessageRecorder := httptest.NewRecorder()
	New(database).ServeHTTP(filteredMessageRecorder, filteredMessageRequest)
	var filteredMessage model.GlobalSearchResponse
	if filteredMessageRecorder.Code != http.StatusOK || json.Unmarshal(filteredMessageRecorder.Body.Bytes(), &filteredMessage) != nil || filteredMessage.Total != 1 || len(filteredMessage.Items) != 1 || filteredMessage.Items[0].Ref != "M-1" {
		t.Fatalf("filtered message search status=%d response=%s parsed=%+v", filteredMessageRecorder.Code, filteredMessageRecorder.Body.String(), filteredMessage)
	}
}
