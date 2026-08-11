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

func TestCategoryCreatedThroughAPIHasProjectScopedItemAddress(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "HSAM"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	handler := api.New(database)

	create := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/categories", bytes.NewBufferString(`{"name":"Memory","goal":"Own memory subsystems"}`))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create category status = %d, body = %s", created.Code, created.Body.String())
	}

	var category struct {
		CategoryID  string `json:"category_id"`
		ItemAddress string `json:"item_address"`
		Name        string `json:"name"`
	}
	if err := json.NewDecoder(created.Body).Decode(&category); err != nil {
		t.Fatalf("decode category: %v", err)
	}
	if category.CategoryID != "C-10001" || category.ItemAddress != "10000.10001" || category.Name != "Memory" {
		t.Fatalf("unexpected category: %#v", category)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/categories", nil)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK {
		t.Fatalf("list category status = %d, body = %s", listed.Code, listed.Body.String())
	}
	var payload struct {
		Items []struct {
			CategoryID string `json:"category_id"`
			Name       string `json:"name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listed.Body).Decode(&payload); err != nil {
		t.Fatalf("decode category list: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].CategoryID != "C-10001" || payload.Items[0].Name != "Memory" {
		t.Fatalf("unexpected categories: %#v", payload.Items)
	}
}

func TestListCategoriesForUnknownProjectReturnsStructuredNotFound(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	response := httptest.NewRecorder()
	api.New(database).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects/P-99999/categories", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown project list status = %d, body = %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"code":"project_not_found"`)) {
		t.Fatalf("unknown project response = %s", response.Body.String())
	}
}
