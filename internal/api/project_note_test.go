// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectNotesAreDurableAndAttributed(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "HSAM"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	server := api.New(database)
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/notes", bytes.NewBufferString(`{"content":"Checked the retrieval baseline."}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("X-Agent-ID", "hermes:default:worker-7")
	createResponse := httptest.NewRecorder()
	server.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create note status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		NoteID  int64  `json:"note_id"`
		Content string `json:"content"`
		ActorID string `json:"actor_id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("decode created note: %v", err)
	}
	if created.NoteID < 1 || created.Content != "Checked the retrieval baseline." || created.ActorID != "hermes:default:worker-7" {
		t.Fatalf("created note = %#v", created)
	}

	listResponse := httptest.NewRecorder()
	server.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/notes", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list notes status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var listed struct {
		Items []struct {
			NoteID  int64  `json:"note_id"`
			Content string `json:"content"`
			ActorID string `json:"actor_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed notes: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].NoteID != created.NoteID || listed.Items[0].Content != created.Content || listed.Items[0].ActorID != created.ActorID {
		t.Fatalf("listed notes = %#v", listed.Items)
	}
}
