// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/knowledge"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

type knowledgeStub struct{}

func (knowledgeStub) Status(context.Context) (knowledge.ProviderStatus, error) {
	return knowledge.ProviderStatus{Available: true, Provider: knowledge.ProviderNameObsidian, ProtocolVersion: knowledge.ProtocolVersion}, nil
}
func (knowledgeStub) Search(_ context.Context, query knowledge.KnowledgeQuery) ([]knowledge.KnowledgeResult, error) {
	return []knowledge.KnowledgeResult{{ID: "MG-1", Title: query.Query, Type: knowledge.ItemTypePattern, ReviewStatus: knowledge.ReviewStatusVerified, Visibility: knowledge.VisibilityGalaxy}}, nil
}
func (knowledgeStub) Get(context.Context, string) (knowledge.KnowledgeItem, error) {
	return knowledge.KnowledgeItem{KnowledgeResult: knowledge.KnowledgeResult{ID: "MG-1", Title: "Pattern", Type: knowledge.ItemTypePattern, ReviewStatus: knowledge.ReviewStatusVerified, Visibility: knowledge.VisibilityGalaxy}, Content: "bounded content"}, nil
}
func (knowledgeStub) Create(context.Context, knowledge.KnowledgeCandidate, string) (knowledge.KnowledgeItem, error) {
	return knowledge.KnowledgeItem{KnowledgeResult: knowledge.KnowledgeResult{ID: "MG-2", Title: "Promoted", Type: knowledge.ItemTypeKnowledge, ReviewStatus: knowledge.ReviewStatusInbox, Visibility: knowledge.VisibilityPrivate}}, nil
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestKnowledgeAPIIsUnavailableWithoutProvider(t *testing.T) {
	handler := api.New(newStore(t))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/status", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "provider_unavailable") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestKnowledgeAPIUsesProviderNeutralContract(t *testing.T) {
	handler := api.NewWithKnowledge(newStore(t), api.RuntimeStatus{}, knowledgeStub{})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/status", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), knowledge.ProtocolVersion) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/search", strings.NewReader(`{"query":"durable context","limit":1}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "durable context") {
		t.Fatalf("search=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/MG-1", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var item knowledge.KnowledgeItem
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &item) != nil || item.Content != "bounded content" {
		t.Fatalf("get=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPromoteProjectMessagePersistsLinkAndIsIdempotent(t *testing.T) {
	database := newStore(t)
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Promotion project"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := database.CreateProjectMessage(context.Background(), project.ProjectID, model.CreateProjectMessageInput{Kind: model.MessageKindDecision, Body: "Keep the durable boundary."})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewWithKnowledge(database, api.RuntimeStatus{}, knowledgeStub{})
	body := `{"redaction_status":"verified","type":"decision","visibility":"galaxy"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/messages/"+strconv.FormatInt(message.MessageID, 10)+"/promote", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), "MG-2") {
		t.Fatalf("first promotion=%d body=%s", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/messages/"+strconv.FormatInt(message.MessageID, 10)+"/promote", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"duplicate":true`) {
		t.Fatalf("retry promotion=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPromoteProjectMessageRequiresRedactionAndBlocksSensitiveContent(t *testing.T) {
	database := newStore(t)
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "Sensitive promotion project"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := database.CreateProjectMessage(context.Background(), project.ProjectID, model.CreateProjectMessageInput{Kind: model.MessageKindNote, Body: "Authorization: Bearer secret-token"})
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewWithKnowledge(database, api.RuntimeStatus{}, knowledgeStub{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/messages/"+strconv.FormatInt(message.MessageID, 10)+"/promote", strings.NewReader(`{"redaction_status":"verified"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "sensitive_content") {
		t.Fatalf("sensitive promotion=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
