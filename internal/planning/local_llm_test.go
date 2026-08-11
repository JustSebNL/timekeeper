// Copyright (c) 2026 Seb. All rights reserved.

package planning_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/planning"
)

func TestGenerateDraftUsesOllamaChatWithoutApplyingWork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			t.Fatalf("content-type=%q", r.Header.Get("Content-Type"))
		}
		_, _ = w.Write([]byte(`{"message":{"content":"{\"tasks\":[\"Review scope\"]}"}}`))
	}))
	t.Cleanup(server.Close)
	text, err := planning.NewClient(nil).GenerateDraft(context.Background(), model.LLMPipeline{Provider: "ollama", BaseURL: server.URL, Model: "qwen3:4b", SystemPrompt: "Return JSON only."}, "Plan this Project.")
	if err != nil || text != `{"tasks":["Review scope"]}` {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestGenerateDraftUsesOpenAICompatibleChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"draft"}}]}`))
	}))
	t.Cleanup(server.Close)
	text, err := planning.NewClient(nil).GenerateDraft(context.Background(), model.LLMPipeline{Provider: "openai-compatible", BaseURL: server.URL, Model: "local"}, "Plan this Project.")
	if err != nil || text != "draft" {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestGenerateDraftRejectsRedirectsBeforeTheyLeakProjectContext(t *testing.T) {
	var redirectedRequests atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedRequests.Add(1)
		payload, _ := io.ReadAll(r.Body)
		if strings.Contains(string(payload), "confidential Project context") {
			t.Errorf("redirect target received planning context")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"draft"}}]}`))
	}))
	t.Cleanup(redirectTarget.Close)
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/v1/chat/completions", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(localServer.Close)

	_, err := planning.NewClient(nil).GenerateDraft(context.Background(), model.LLMPipeline{Provider: "openai-compatible", BaseURL: localServer.URL, Model: "local"}, "confidential Project context")
	if err == nil {
		t.Fatal("expected redirect to be rejected")
	}
	if got := redirectedRequests.Load(); got != 0 {
		t.Fatalf("redirect target received %d request(s)", got)
	}
}
