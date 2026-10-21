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

func TestProjectMessageSearchSupportsKindTagsAndDateFilters(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Search Filters"})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := database.CreateProjectMessage(context.Background(), project.ProjectID, model.CreateProjectMessageInput{Kind: model.MessageKindDecision, Body: "Queue policy applies to delivery", Tags: "queue,durable"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProjectMessage(context.Background(), project.ProjectID, model.CreateProjectMessageInput{Kind: model.MessageKindNote, Body: "Queue note about delivery", Tags: "queue,routine"}); err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/messages/search?q=queue&kind=decision&tags=durable&after="+today+"&before="+today+"&limit=1", nil)
	response := httptest.NewRecorder()
	New(database).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []model.ProjectMessageSearchHit `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].Ref != decision.Ref || payload.Items[0].Kind != model.MessageKindDecision {
		t.Fatalf("filtered items=%+v", payload.Items)
	}
}
