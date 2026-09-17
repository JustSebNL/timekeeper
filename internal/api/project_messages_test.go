// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectMessageAPIProvidesThreadsEditsAndRevisions(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Board API"})
	if err != nil {
		t.Fatal(err)
	}
	server := api.New(database)

	rootResponse := performJSON(server, http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/messages", `{"kind":"observation","body":"The queue boundary is the important seam.","tags":"queue"}`)
	if rootResponse.Code != http.StatusCreated {
		t.Fatalf("root status=%d body=%s", rootResponse.Code, rootResponse.Body.String())
	}
	var root model.ProjectMessage
	if err := json.NewDecoder(rootResponse.Body).Decode(&root); err != nil {
		t.Fatal(err)
	}
	if root.Ref != "M-"+strconv.FormatInt(root.MessageID, 10) || root.Revision != 1 {
		t.Fatalf("root=%#v", root)
	}

	replyResponse := performJSON(server, http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/messages", `{"kind":"answer","body":"Confirmed: the Sprint must be dispatched after persistence.","parent_message_id":`+strconv.FormatInt(root.MessageID, 10)+`}`)
	if replyResponse.Code != http.StatusCreated {
		t.Fatalf("reply status=%d body=%s", replyResponse.Code, replyResponse.Body.String())
	}
	var reply model.ProjectMessage
	if err := json.NewDecoder(replyResponse.Body).Decode(&reply); err != nil {
		t.Fatal(err)
	}
	if reply.ParentMessageID == nil || *reply.ParentMessageID != root.MessageID {
		t.Fatalf("reply=%#v", reply)
	}

	editResponse := performJSON(server, http.MethodPatch, "/api/v1/projects/"+project.ProjectID+"/messages/"+strconv.FormatInt(root.MessageID, 10), `{"kind":"lesson","author":"xatia","body":"Persist first, dispatch second.","tags":"queue,lesson"}`)
	if editResponse.Code != http.StatusOK {
		t.Fatalf("edit status=%d body=%s", editResponse.Code, editResponse.Body.String())
	}
	var edited model.ProjectMessage
	if err := json.NewDecoder(editResponse.Body).Decode(&edited); err != nil {
		t.Fatal(err)
	}
	if edited.Revision != 2 || edited.Kind != model.MessageKindLesson || edited.Body != "Persist first, dispatch second." {
		t.Fatalf("edited=%#v", edited)
	}

	threadResponse := performJSON(server, http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/messages/"+strconv.FormatInt(reply.MessageID, 10)+"/thread", "")
	if threadResponse.Code != http.StatusOK {
		t.Fatalf("thread status=%d body=%s", threadResponse.Code, threadResponse.Body.String())
	}
	var thread struct {
		Items []model.ProjectMessage `json:"items"`
	}
	if err := json.NewDecoder(threadResponse.Body).Decode(&thread); err != nil {
		t.Fatal(err)
	}
	if len(thread.Items) != 2 || thread.Items[0].MessageID != root.MessageID || thread.Items[1].MessageID != reply.MessageID {
		t.Fatalf("thread=%#v", thread.Items)
	}

	revisionResponse := performJSON(server, http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/messages/"+strconv.FormatInt(root.MessageID, 10)+"/revisions", "")
	if revisionResponse.Code != http.StatusOK {
		t.Fatalf("revision status=%d body=%s", revisionResponse.Code, revisionResponse.Body.String())
	}
	var revisions struct {
		Items []model.ProjectMessageRevision `json:"items"`
	}
	if err := json.NewDecoder(revisionResponse.Body).Decode(&revisions); err != nil {
		t.Fatal(err)
	}
	if len(revisions.Items) != 2 || revisions.Items[0].Revision != 2 || revisions.Items[1].Revision != 1 {
		t.Fatalf("revisions=%#v", revisions.Items)
	}
}

func TestProjectMessageAPIHydratesBoundedContext(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Context API"})
	if err != nil {
		t.Fatal(err)
	}
	server := api.New(database)
	rootResponse := performJSON(server, http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/messages", `{"kind":"observation","body":"Alpha queue checkpoint.","tags":"alpha"}`)
	var root model.ProjectMessage
	if rootResponse.Code != http.StatusCreated || json.NewDecoder(rootResponse.Body).Decode(&root) != nil {
		t.Fatalf("root status=%d body=%s", rootResponse.Code, rootResponse.Body.String())
	}
	for _, body := range []string{"Alpha reply one.", "Alpha reply two.", "Alpha related one.", "Alpha related two.", "Alpha related three."} {
		payload := `{"kind":"answer","body":"` + body + `"}`
		if strings.Contains(body, "related") {
			payload = `{"kind":"lesson","body":"` + body + `","tags":"alpha"}`
		} else {
			payload = `{"kind":"answer","body":"` + body + `","parent_message_id":` + strconv.FormatInt(root.MessageID, 10) + `}`
		}
		response := performJSON(server, http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/messages", payload)
		if response.Code != http.StatusCreated {
			t.Fatalf("child status=%d body=%s", response.Code, response.Body.String())
		}
	}
	edit := performJSON(server, http.MethodPatch, "/api/v1/projects/"+project.ProjectID+"/messages/"+strconv.FormatInt(root.MessageID, 10), `{"kind":"decision","body":"Alpha queue checkpoint is mandatory.","tags":"alpha"}`)
	if edit.Code != http.StatusOK {
		t.Fatalf("edit status=%d body=%s", edit.Code, edit.Body.String())
	}
	contextResponse := performJSON(server, http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/messages/"+strconv.FormatInt(root.MessageID, 10)+"/context?thread_limit=1&related_limit=2&revision_limit=1", "")
	if contextResponse.Code != http.StatusOK {
		t.Fatalf("context status=%d body=%s", contextResponse.Code, contextResponse.Body.String())
	}
	var bundle model.ProjectMessageContext
	if err := json.NewDecoder(contextResponse.Body).Decode(&bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Anchor.MessageID != root.MessageID || len(bundle.Thread) != 1 || len(bundle.Related) != 2 || len(bundle.Revisions) != 1 {
		t.Fatalf("bundle=%#v", bundle)
	}
	if !bundle.ThreadTruncated || !bundle.RelatedTruncated || !bundle.RevisionsTruncated || bundle.RelatedQuery != "alpha" || bundle.RelatedMode != "vector" {
		t.Fatalf("bundle truncation/query/mode=%#v", bundle)
	}
	for _, hit := range bundle.Related {
		if hit.MessageID == root.MessageID {
			t.Fatalf("anchor leaked into related results: %#v", bundle.Related)
		}
	}
	invalid := performJSON(server, http.MethodGet, "/api/v1/projects/"+project.ProjectID+"/messages/"+strconv.FormatInt(root.MessageID, 10)+"/context?related_limit=-1", "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestProjectMessageAPIPinsAndUnpinsWithoutRevision(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Pin API"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := database.CreateProjectMessage(ctx, project.ProjectID, model.CreateProjectMessageInput{Body: "Important durable decision."})
	if err != nil {
		t.Fatal(err)
	}
	server := api.New(database)
	path := "/api/v1/projects/" + project.ProjectID + "/messages/" + strconv.FormatInt(message.MessageID, 10) + "/pin"
	pin := performJSON(server, http.MethodPost, path, `{"pinned":true}`)
	if pin.Code != http.StatusOK {
		t.Fatalf("pin status=%d body=%s", pin.Code, pin.Body.String())
	}
	var pinned model.ProjectMessage
	if err := json.NewDecoder(pin.Body).Decode(&pinned); err != nil {
		t.Fatal(err)
	}
	if !pinned.Pinned || pinned.Revision != 1 {
		t.Fatalf("pinned=%#v", pinned)
	}
	unpin := performJSON(server, http.MethodPost, path, `{"pinned":false}`)
	if unpin.Code != http.StatusOK {
		t.Fatalf("unpin status=%d body=%s", unpin.Code, unpin.Body.String())
	}
	var unpinned model.ProjectMessage
	if err := json.NewDecoder(unpin.Body).Decode(&unpinned); err != nil {
		t.Fatal(err)
	}
	if unpinned.Pinned || unpinned.Revision != 1 {
		t.Fatalf("unpinned=%#v", unpinned)
	}
}

func TestProjectMessageAPICreatesAtomicCheckpointBundle(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Checkpoint API"})
	if err != nil {
		t.Fatal(err)
	}
	server := api.New(database)
	response := performJSON(server, http.MethodPost, "/api/v1/projects/"+project.ProjectID+"/messages/checkpoint", `{"author":"agent-a","entries":[{"kind":"decision","body":"Save before dispatch."},{"kind":"lesson","body":"Keep the root M-ref."}]}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("checkpoint status=%d body=%s", response.Code, response.Body.String())
	}
	var bundle model.ProjectMessageCheckpoint
	if err := json.NewDecoder(response.Body).Decode(&bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Root.Ref == "" || len(bundle.Items) != 2 || bundle.Items[1].ParentMessageID == nil || *bundle.Items[1].ParentMessageID != bundle.Root.MessageID {
		t.Fatalf("checkpoint bundle=%#v", bundle)
	}
}

func TestProjectMessageAPIResolvesProjectAlias(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Alias Board", Alias: "hsam"})
	if err != nil {
		t.Fatal(err)
	}
	server := api.New(database)
	response := performJSON(server, http.MethodPost, "/api/v1/projects/hsam/messages", `{"kind":"note","body":"Alias addresses the same board."}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("alias message status=%d body=%s project=%s", response.Code, response.Body.String(), project.ProjectID)
	}
	var message model.ProjectMessage
	if err := json.NewDecoder(response.Body).Decode(&message); err != nil {
		t.Fatal(err)
	}
	if message.ProjectID != project.ProjectID {
		t.Fatalf("alias message project=%#v", message)
	}
}

func TestProjectMessageAPIRejectsCrossProjectReplyAndOversizedBody(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	first, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "First"})
	second, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Second"})
	root, _ := database.CreateProjectMessage(ctx, first.ProjectID, model.CreateProjectMessageInput{Body: "Private first project note."})
	server := api.New(database)
	cross := performJSON(server, http.MethodPost, "/api/v1/projects/"+second.ProjectID+"/messages", `{"body":"Must reject this parent.","parent_message_id":`+strconv.FormatInt(root.MessageID, 10)+`}`)
	if cross.Code != http.StatusBadRequest {
		t.Fatalf("cross-project status=%d body=%s", cross.Code, cross.Body.String())
	}
	large := performJSON(server, http.MethodPost, "/api/v1/projects/"+first.ProjectID+"/messages", `{"body":"`+strings.Repeat("x", model.MaxProjectMessageBodyLength+1)+`"}`)
	if large.Code != http.StatusBadRequest {
		t.Fatalf("large status=%d body=%s", large.Code, large.Body.String())
	}
}

func performJSON(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	request := httptest.NewRequest(method, path, reader)
	if method != http.MethodGet {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
