package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func adminRoutes(mux *http.ServeMux, database *store.Store) {
	mux.HandleFunc("GET /api/v1/admin/me", adminMe(database))
	mux.HandleFunc("GET /api/v1/admin/users", adminUsers(database))
	mux.HandleFunc("PATCH /api/v1/admin/users/{userID}", updateAdminUser(database))
	mux.HandleFunc("GET /api/v1/admin/groups", adminGroups(database))
	mux.HandleFunc("POST /api/v1/admin/groups", createAdminGroup(database))
	mux.HandleFunc("PUT /api/v1/admin/groups/{groupID}/members", updateAdminGroupMembers(database))
	mux.HandleFunc("GET /api/v1/admin/permissions", adminPermissions(database))
	mux.HandleFunc("GET /api/v1/admin/audit", adminAudit(database))
}

func adminMe(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := resolveAdminPrincipal(r, database)
		if err != nil {
			writeAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, principal)
	}
}

func adminUsers(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdminPermission(w, r, database, "admin.users.read"); !ok {
			return
		}
		users, err := database.ListAdminUsers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "admin_users_failed", "Could not load administrator users.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": users})
	}
}

func updateAdminUser(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := requireAdminPermission(w, r, database, "admin.users.manage")
		if !ok {
			return
		}
		var input model.UpdateAdminUserInput
		if err := decodeAdminJSON(r, &input); err != nil || input.Enabled == nil {
			writeError(w, http.StatusBadRequest, "invalid_admin_user", "enabled must be provided as a boolean.")
			return
		}
		userID := r.PathValue("userID")
		if err := database.SetAdminUserEnabled(r.Context(), userID, *input.Enabled); err != nil {
			if errors.Is(err, store.ErrAdminLastAdmin) {
				writeError(w, http.StatusConflict, "last_administrator", err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, "admin_user_update_failed", err.Error())
			return
		}
		_ = database.AppendAdminAudit(r.Context(), adminAuditEvent(r, principal.User.UserID, "admin.user.enabled_changed", "user", userID, map[string]any{"enabled": *input.Enabled}))
		writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "enabled": *input.Enabled})
	}
}

func adminGroups(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdminPermission(w, r, database, "admin.groups.read"); !ok {
			return
		}
		groups, err := database.ListAdminGroups(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "admin_groups_failed", "Could not load security groups.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
	}
}

func createAdminGroup(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := requireAdminPermission(w, r, database, "admin.groups.manage")
		if !ok {
			return
		}
		var input model.CreateAdminGroupInput
		if err := decodeAdminJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_admin_group", err.Error())
			return
		}
		group, err := database.CreateAdminGroup(r.Context(), input)
		if err != nil {
			writeError(w, http.StatusBadRequest, "admin_group_create_failed", err.Error())
			return
		}
		_ = database.AppendAdminAudit(r.Context(), adminAuditEvent(r, principal.User.UserID, "admin.group.created", "group", group.GroupID, map[string]any{"slug": group.Slug}))
		writeJSON(w, http.StatusCreated, group)
	}
}

func updateAdminGroupMembers(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := requireAdminPermission(w, r, database, "admin.groups.manage")
		if !ok {
			return
		}
		var input model.UpdateAdminGroupMembersInput
		if err := decodeAdminJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_admin_members", err.Error())
			return
		}
		groupID := r.PathValue("groupID")
		if err := database.SetAdminGroupMembers(r.Context(), groupID, input.UserIDs); err != nil {
			writeError(w, http.StatusBadRequest, "admin_members_update_failed", err.Error())
			return
		}
		_ = database.AppendAdminAudit(r.Context(), adminAuditEvent(r, principal.User.UserID, "admin.group.members_changed", "group", groupID, map[string]any{"user_ids": input.UserIDs}))
		writeJSON(w, http.StatusOK, map[string]any{"group_id": groupID, "user_ids": input.UserIDs})
	}
}

func adminPermissions(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdminPermission(w, r, database, "admin.groups.read"); !ok {
			return
		}
		permissions, err := database.ListAdminPermissions(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "admin_permissions_failed", "Could not load permissions.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"permissions": permissions})
	}
}

func adminAudit(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdminPermission(w, r, database, "admin.audit.read"); !ok {
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		events, err := database.ListAdminAudit(r.Context(), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "admin_audit_failed", "Could not load administrator audit events.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	}
}

func resolveAdminPrincipal(r *http.Request, database *store.Store) (model.AdminPrincipal, error) {
	if strings.TrimSpace(r.Header.Get("X-Agent-ID")) == "" {
		return model.AdminPrincipal{}, store.ErrAdminIdentity
	}
	return database.EnsureAdminPrincipal(r.Context(), r.Header.Get("X-Agent-ID"), r.Header.Get("X-Agent-Name"), r.Header.Get("X-Agent-Type"))
}

func requireAdminPermission(w http.ResponseWriter, r *http.Request, database *store.Store, permission string) (model.AdminPrincipal, bool) {
	principal, err := resolveAdminPrincipal(r, database)
	if err != nil {
		writeAdminError(w, err)
		return model.AdminPrincipal{}, false
	}
	allowed, err := database.HasAdminPermission(r.Context(), principal.User.UserID, permission)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "admin_authorization_failed", "Could not evaluate administrator permissions.")
		return model.AdminPrincipal{}, false
	}
	if !allowed {
		_ = database.AppendAdminAudit(r.Context(), adminAuditEvent(r, principal.User.UserID, "admin.access.denied", "permission", permission, map[string]any{"path": r.URL.Path}))
		writeError(w, http.StatusForbidden, "admin_permission_denied", "Your account is not allowed to perform this administrator action.")
		return model.AdminPrincipal{}, false
	}
	return principal, true
}

func writeAdminError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrAdminIdentity) {
		writeError(w, http.StatusUnauthorized, "admin_identity_required", "An X-Agent-ID identity is required for administrator access.")
		return
	}
	if errors.Is(err, store.ErrAdminDisabled) {
		writeError(w, http.StatusForbidden, "admin_account_disabled", "This account is disabled.")
		return
	}
	writeError(w, http.StatusInternalServerError, "admin_identity_failed", "Could not resolve the administrator identity.")
}

func decodeAdminJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("request must contain one JSON value")
		}
		return err
	}
	return nil
}

func adminAuditEvent(r *http.Request, actor, action, targetType, targetID string, details map[string]any) model.AdminAuditEvent {
	encoded, _ := json.Marshal(details)
	return model.AdminAuditEvent{
		ActorUserID: actor,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Details:     string(encoded),
		RequestID:   r.Header.Get("X-Request-ID"),
		ClientID:    r.Header.Get("X-TimeKeeper-Client"),
		Surface:     strings.TrimSpace(r.Header.Get("X-TimeKeeper-Surface")),
	}
}
