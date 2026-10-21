// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestTreasureChestHelpDoesNotResolveProject(t *testing.T) {
	var out, errOut strings.Builder
	if code := cli.Run([]string{"tc", "--help"}, &out, &errOut, "http://127.0.0.1:1"); code != 0 {
		t.Fatalf("help code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	for _, marker := range []string{"add <project>", "attach <project>", "delete|restore", "categories <project>"} {
		if !strings.Contains(out.String(), marker) { t.Fatalf("help missing %q: %q", marker, out.String()) }
	}
	if errOut.Len() != 0 { t.Fatalf("help wrote stderr=%q", errOut.String()) }
}

func TestTreasureChestAddPostsDedicatedGemRow(t *testing.T) {
	var posted model.CreateProjectGemInput
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/P-1/treasure-chest/gems" { http.NotFound(w, r); return }
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil { t.Fatalf("decode create input: %v", err) }
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(model.ProjectGem{GemID: 42, Ref: "G-42", ProjectID: "P-1", Category: posted.Category, Subcategory: posted.Subcategory, Name: posted.Name, Content: posted.Content, Revision: 1})
	}))
	defer server.Close()
	var out, errOut strings.Builder
	args := []string{"tc", "add", "P-1", "Research", "Retry budget", "--subcategory", "Backoff", "Monotonic deadlines reduce retry storms."}
	if code := cli.Run(args, &out, &errOut, server.URL); code != 0 { t.Fatalf("add code=%d out=%q err=%q", code, out.String(), errOut.String()) }
	if posted.Category != "Research" || posted.Subcategory != "Backoff" || posted.Name != "Retry budget" || posted.Content != "Monotonic deadlines reduce retry storms." {
		t.Fatalf("dedicated gem payload=%#v", posted)
	}
	if !strings.Contains(out.String(), "G-42") { t.Fatalf("gem reference missing: %q", out.String()) }
}

func TestTreasureChestAttachUsesDedicatedParentAndInheritedCategory(t *testing.T) {
	var posted model.CreateProjectGemInput
	parent := model.ProjectGem{GemID: 10, Ref: "G-10", ProjectID: "P-1", Category: "Patterns", Subcategory: "Backoff", Name: "Parent", Content: "Root insight.", Revision: 1}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/P-1/treasure-chest/gems/10":
			_ = json.NewEncoder(w).Encode(parent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/P-1/treasure-chest/gems":
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil { t.Fatalf("decode attachment input: %v", err) }
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(model.ProjectGem{GemID: 11, Ref: "G-11", ProjectID: "P-1", ParentGemID: posted.ParentGemID, Category: posted.Category, Subcategory: posted.Subcategory, Name: posted.Name, Content: posted.Content, Revision: 1})
		default: http.NotFound(w, r)
		}
	}))
	defer server.Close()
	var out, errOut strings.Builder
	if code := cli.Run([]string{"tc", "attach", "P-1", "G-10", "Counterexample", "Fixed delays synchronize clients."}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("attach code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	if posted.ParentGemID == nil || *posted.ParentGemID != 10 || posted.Category != "Patterns" || posted.Subcategory != "Backoff" {
		t.Fatalf("attached gem payload=%#v", posted)
	}
}

func TestTreasureChestEditUsesDedicatedGemEndpoint(t *testing.T) {
	var posted model.UpdateProjectGemInput
	current := model.ProjectGem{GemID: 10, Ref: "G-10", ProjectID: "P-1", Category: "Patterns", Name: "Old", Content: "Old content.", Revision: 1}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/P-1/treasure-chest/gems/10":
			_ = json.NewEncoder(w).Encode(current)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/projects/P-1/treasure-chest/gems/10":
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil { t.Fatalf("decode update input: %v", err) }
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(model.ProjectGem{GemID: 10, Ref: "G-10", ProjectID: "P-1", Category: posted.Category, Name: posted.Name, Content: posted.Content, Revision: 2})
		default: http.NotFound(w, r)
		}
	}))
	defer server.Close()
	var out, errOut strings.Builder
	args := []string{"tc", "edit", "P-1", "G-10", "--name", "Better title", "--content", "Improved content."}
	if code := cli.Run(args, &out, &errOut, server.URL); code != 0 { t.Fatalf("edit code=%d out=%q err=%q", code, out.String(), errOut.String()) }
	if posted.Name != "Better title" || posted.Content != "Improved content." || posted.Category != "Patterns" { t.Fatalf("update input=%#v", posted) }
}

func TestTreasureChestLookupUsesDedicatedSearchEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects/P-1/treasure-chest/gems/search" || r.URL.Query().Get("q") != "retry" || r.URL.Query().Get("category") != "Patterns" {
			http.NotFound(w, r); return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.ProjectGemList{Items: []model.ProjectGem{
			{GemID: 11, Ref: "G-11", ProjectID: "P-1", Category: "Patterns", Name: "Pinned retry", Content: "Important.", Pinned: true},
			{GemID: 10, Ref: "G-10", ProjectID: "P-1", Category: "Patterns", Name: "Ordinary retry", Content: "Insight."},
		}, Total: 2, Limit: 50})
	}))
	defer server.Close()
	var out, errOut strings.Builder
	if code := cli.Run([]string{"tc", "lookup", "P-1", "retry", "--category", "Patterns"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("lookup code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	var result model.ProjectGemList
	if err := json.Unmarshal([]byte(out.String()), &result); err != nil { t.Fatalf("decode lookup: %v", err) }
	if len(result.Items) != 2 || result.Items[0].Ref != "G-11" || !result.Items[0].Pinned { t.Fatalf("lookup result=%#v", result.Items) }
}

func TestTreasureChestShowCategoriesPinDeleteAndRestoreUseDedicatedRoutes(t *testing.T) {
	parentID := int64(10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/P-1/treasure-chest/gems/10":
			_ = json.NewEncoder(w).Encode(model.ProjectGem{GemID: 10, Ref: "G-10", ProjectID: "P-1", Category: "Patterns", Name: "Retry", Content: "Insight."})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/P-1/treasure-chest/gems" && r.URL.Query().Get("parent_gem_id") == "10":
			_ = json.NewEncoder(w).Encode(model.ProjectGemList{Items: []model.ProjectGem{{GemID: 11, Ref: "G-11", ProjectID: "P-1", ParentGemID: &parentID, Category: "Patterns", Name: "Child", Content: "More."}}, Total: 1, Limit: 50})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/P-1/treasure-chest/categories":
			_, _ = w.Write([]byte(`{"items":[{"category":"Patterns","subcategories":[]}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pin"):
			_, _ = w.Write([]byte(`{"gem_id":10,"ref":"G-10","project_id":"P-1","category":"Patterns","name":"Retry","content":"Insight.","pinned":true}`))
		case r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/delete") || strings.HasSuffix(r.URL.Path, "/restore")):
			_, _ = w.Write([]byte(`{"gem_id":10,"ref":"G-10","project_id":"P-1","category":"Patterns","name":"Retry","content":"Insight."}`))
		default: http.NotFound(w, r)
		}
	}))
	defer server.Close()
	for _, args := range [][]string{{"tc", "show", "P-1", "G-10"}, {"tc", "categories", "P-1"}, {"tc", "pin", "P-1", "G-10"}, {"tc", "delete", "P-1", "G-10"}, {"tc", "restore", "P-1", "G-10"}} {
		var out, errOut strings.Builder
		if code := cli.Run(args, &out, &errOut, server.URL); code != 0 { t.Fatalf("command %v code=%d out=%q err=%q", args, code, out.String(), errOut.String()) }
	}
}
