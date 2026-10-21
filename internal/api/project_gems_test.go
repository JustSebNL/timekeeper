// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectTreasureChestAPIStoresAndRetrievesDedicatedGems(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Dedicated Treasure Chest"})
	if err != nil { t.Fatal(err) }
	server := api.New(database)
	base := "/api/v1/projects/" + project.ProjectID + "/treasure-chest"

	createdResponse := performJSON(server, http.MethodPost, base+"/gems", `{"category":"Research","subcategory":"Retrieval","name":"Retry budget","content":"Monotonic deadlines reduce retry storms."}`)
	if createdResponse.Code != http.StatusCreated { t.Fatalf("create status=%d body=%s", createdResponse.Code, createdResponse.Body.String()) }
	var root model.ProjectGem
	if err := json.NewDecoder(createdResponse.Body).Decode(&root); err != nil { t.Fatal(err) }
	if root.Ref == "" || root.GemID < 1 || root.Category != "Research" || root.Subcategory != "Retrieval" || root.Revision != 1 {
		t.Fatalf("created gem=%#v", root)
	}

	childResponse := performJSON(server, http.MethodPost, base+"/gems", `{"parent_gem_id":`+strconv.FormatInt(root.GemID, 10)+`,"name":"Counterexample","content":"Fixed delays can synchronize clients."}`)
	if childResponse.Code != http.StatusCreated { t.Fatalf("attach status=%d body=%s", childResponse.Code, childResponse.Body.String()) }
	var child model.ProjectGem
	if err := json.NewDecoder(childResponse.Body).Decode(&child); err != nil { t.Fatal(err) }
	if child.ParentGemID == nil || *child.ParentGemID != root.GemID || child.Category != root.Category || child.Subcategory != root.Subcategory {
		t.Fatalf("attached gem=%#v parent=%#v", child, root)
	}

	categoriesResponse := performJSON(server, http.MethodGet, base+"/categories", "")
	if categoriesResponse.Code != http.StatusOK { t.Fatalf("categories status=%d body=%s", categoriesResponse.Code, categoriesResponse.Body.String()) }
	var categories struct { Items []model.ProjectGemCategory `json:"items"` }
	if err := json.NewDecoder(categoriesResponse.Body).Decode(&categories); err != nil { t.Fatal(err) }
	if len(categories.Items) != 1 || categories.Items[0].Category != "Research" || len(categories.Items[0].Subcategories) != 1 || categories.Items[0].Subcategories[0] != "Retrieval" {
		t.Fatalf("auto-created category path=%#v", categories.Items)
	}

	pinResponse := performJSON(server, http.MethodPost, base+"/gems/"+strconv.FormatInt(root.GemID, 10)+"/pin", `{"pinned":true}`)
	if pinResponse.Code != http.StatusOK { t.Fatalf("pin status=%d body=%s", pinResponse.Code, pinResponse.Body.String()) }
	updateResponse := performJSON(server, http.MethodPatch, base+"/gems/"+strconv.FormatInt(root.GemID, 10), `{"category":"Research","subcategory":"Retrieval","name":"Retry budget v2","content":"Edited insight.","tags":"queue","source_ref":"repo/docs/retry.md"}`)
	if updateResponse.Code != http.StatusOK { t.Fatalf("edit status=%d body=%s", updateResponse.Code, updateResponse.Body.String()) }
	var updated model.ProjectGem
	if err := json.NewDecoder(updateResponse.Body).Decode(&updated); err != nil { t.Fatal(err) }
	if updated.Revision != 2 || !updated.Pinned || updated.Name != "Retry budget v2" { t.Fatalf("updated gem=%#v", updated) }

	revisionsResponse := performJSON(server, http.MethodGet, base+"/gems/"+strconv.FormatInt(root.GemID, 10)+"/revisions", "")
	if revisionsResponse.Code != http.StatusOK { t.Fatalf("revisions status=%d body=%s", revisionsResponse.Code, revisionsResponse.Body.String()) }
	var revisions struct { Items []model.ProjectGemRevision `json:"items"` }
	if err := json.NewDecoder(revisionsResponse.Body).Decode(&revisions); err != nil { t.Fatal(err) }
	if len(revisions.Items) != 2 || revisions.Items[0].Revision != 2 || revisions.Items[1].Revision != 1 { t.Fatalf("gem revisions=%#v", revisions.Items) }

	deleteResponse := performJSON(server, http.MethodPost, base+"/gems/"+strconv.FormatInt(root.GemID, 10)+"/delete", "")
	if deleteResponse.Code != http.StatusOK { t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String()) }
	missing := performJSON(server, http.MethodGet, base+"/gems/"+strconv.FormatInt(root.GemID, 10), "")
	if missing.Code != http.StatusNotFound { t.Fatalf("deleted gem status=%d body=%s", missing.Code, missing.Body.String()) }
	restoreResponse := performJSON(server, http.MethodPost, base+"/gems/"+strconv.FormatInt(root.GemID, 10)+"/restore", "")
	if restoreResponse.Code != http.StatusOK { t.Fatalf("restore status=%d body=%s", restoreResponse.Code, restoreResponse.Body.String()) }
}

func TestTreasureChestRoutesAppearInAPIHelp(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = database.Close() })
	response := performJSON(api.New(database), http.MethodGet, "/api/help", "")
	if response.Code != http.StatusOK { t.Fatalf("API help status=%d body=%s", response.Code, response.Body.String()) }
	for _, route := range []string{"/api/v1/projects/{projectID}/treasure-chest/gems", "/api/v1/projects/{projectID}/treasure-chest/categories", "/api/v1/projects/{projectID}/treasure-chest/gems/{gemID}/revisions"} {
		if !strings.Contains(response.Body.String(), route) { t.Fatalf("API help missing route %q", route) }
	}
}
