package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestAdministratorBootstrapAndServerEnforcedGroupAccess(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	handler := api.New(database)

	missingIdentity := adminRequest(handler, http.MethodGet, "/api/v1/admin/me", "", nil)
	if missingIdentity.Code != http.StatusUnauthorized {
		t.Fatalf("missing identity status=%d body=%s", missingIdentity.Code, missingIdentity.Body.String())
	}

	bootstrap := adminRequest(handler, http.MethodGet, "/api/v1/admin/me", "agent-admin", nil)
	if bootstrap.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var principal model.AdminPrincipal
	if err := json.NewDecoder(bootstrap.Body).Decode(&principal); err != nil {
		t.Fatal(err)
	}
	if !principal.IsBootstrap || principal.User.Groups[0] != "administrators" {
		t.Fatalf("unexpected bootstrap principal=%#v", principal)
	}

	second := adminRequest(handler, http.MethodGet, "/api/v1/admin/me", "agent-viewer", nil)
	if second.Code != http.StatusOK {
		t.Fatalf("second principal status=%d body=%s", second.Code, second.Body.String())
	}
	usersDenied := adminRequest(handler, http.MethodGet, "/api/v1/admin/users", "agent-viewer", nil)
	if usersDenied.Code != http.StatusForbidden {
		t.Fatalf("viewer users status=%d body=%s", usersDenied.Code, usersDenied.Body.String())
	}

	groups := adminRequest(handler, http.MethodGet, "/api/v1/admin/groups", "agent-admin", nil)
	if groups.Code != http.StatusOK {
		t.Fatalf("groups status=%d body=%s", groups.Code, groups.Body.String())
	}
	var groupPayload struct {
		Groups []model.AdminGroup `json:"groups"`
	}
	if err := json.NewDecoder(groups.Body).Decode(&groupPayload); err != nil || len(groupPayload.Groups) < 5 {
		t.Fatalf("groups=%#v err=%v", groupPayload, err)
	}

	created := adminRequest(handler, http.MethodPost, "/api/v1/admin/groups", "agent-admin", bytes.NewBufferString(`{"slug":"incident_managers","name":"Incident Managers","permissions":["diagnostics.read","admin.audit.read"]}`))
	if created.Code != http.StatusCreated {
		t.Fatalf("create group status=%d body=%s", created.Code, created.Body.String())
	}

	var usersPayload struct {
		Users []model.AdminUser `json:"users"`
	}
	users := adminRequest(handler, http.MethodGet, "/api/v1/admin/users", "agent-admin", nil)
	if users.Code != http.StatusOK || json.NewDecoder(users.Body).Decode(&usersPayload) != nil {
		t.Fatalf("users status=%d body=%s", users.Code, users.Body.String())
	}
	var viewerID string
	for _, user := range usersPayload.Users {
		if user.ExternalID == "agent-viewer" {
			viewerID = user.UserID
		}
	}
	if viewerID == "" {
		t.Fatal("viewer identity was not persisted")
	}

	disabled := adminRequest(handler, http.MethodPatch, "/api/v1/admin/users/"+viewerID, "agent-admin", bytes.NewBufferString(`{"enabled":false}`))
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	viewerAfterDisable := adminRequest(handler, http.MethodGet, "/api/v1/admin/me", "agent-viewer", nil)
	if viewerAfterDisable.Code != http.StatusForbidden {
		t.Fatalf("disabled principal status=%d body=%s", viewerAfterDisable.Code, viewerAfterDisable.Body.String())
	}
}

func adminRequest(handler http.Handler, method, path, agentID string, body *bytes.Buffer) *httptest.ResponseRecorder {
	var reader *bytes.Buffer
	if body == nil {
		reader = bytes.NewBuffer(nil)
	} else {
		reader = body
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("X-Agent-ID", agentID)
	request.Header.Set("X-Agent-Name", agentID)
	request.Header.Set("X-Agent-Type", "human")
	request.Header.Set("X-TimeKeeper-Client", "admin-test")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
