package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

var (
	ErrAdminDisabled   = errors.New("administrator account is disabled")
	ErrAdminIdentity   = errors.New("administrator identity is required")
	ErrAdminPermission = errors.New("administrator permission denied")
	ErrAdminLastAdmin  = errors.New("cannot disable the last enabled administrator")
	adminSlugPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
)

type adminPermissionSeed struct {
	resource    string
	action      string
	description string
}

var adminPermissionSeeds = []adminPermissionSeed{
	{"admin.users", "read", "View user accounts"},
	{"admin.users", "manage", "Enable, disable, and manage user accounts"},
	{"admin.groups", "read", "View security groups"},
	{"admin.groups", "manage", "Create groups and assign members and permissions"},
	{"admin.audit", "read", "View administrator audit events"},
	{"projects", "read", "View projects and execution state"},
	{"projects", "write", "Create and update projects"},
	{"messages", "read", "Read project memory"},
	{"messages", "write", "Write project memory"},
	{"sprints", "execute", "Start, hold, resume, and complete Sprints"},
	{"usage", "read", "View token and session usage"},
	{"diagnostics", "read", "View diagnostic and internal error details"},
	{"visibility.project.private_context", "read", "View private project context"},
	{"visibility.execution.history", "read", "View execution history details"},
	{"visibility.usage.tokens", "read", "View token and session usage details"},
	{"visibility.diagnostics.internal_error", "read", "View internal diagnostic causes"},
	{"visibility.diagnostics.stack_trace", "read", "View bounded diagnostic stack traces"},
}

var adminGroupSeeds = []struct {
	slug        string
	name        string
	description string
	permissions []string
}{
	{"administrators", "Administrators", "Full TimeKeeper administration", []string{"*"}},
	{"operators", "Operators", "Manage projects and execution without administration", []string{"projects.read", "projects.write", "messages.read", "messages.write", "sprints.execute", "usage.read", "visibility.execution.history.read"}},
	{"agents", "Agents", "Agent execution and project memory access", []string{"projects.read", "messages.read", "messages.write", "sprints.execute", "usage.read"}},
	{"viewers", "Viewers", "Read-only workspace access", []string{"projects.read", "messages.read", "usage.read"}},
	{"auditors", "Auditors", "Read-only operational and diagnostic evidence", []string{"projects.read", "messages.read", "usage.read", "diagnostics.read", "admin.audit.read"}},
}

func (s *Store) migrateAdmin(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS admin_users (
			user_id TEXT PRIMARY KEY,
			external_id TEXT NOT NULL UNIQUE CHECK (length(external_id) BETWEEN 1 AND 256),
			display_name TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 256),
			kind TEXT NOT NULL CHECK (kind IN ('human', 'agent', 'service')),
			enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_groups (
			group_id TEXT PRIMARY KEY,
			slug TEXT NOT NULL UNIQUE CHECK (length(slug) BETWEEN 2 AND 64),
			name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
			description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
			built_in INTEGER NOT NULL DEFAULT 0 CHECK (built_in IN (0, 1)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_group_members (
			group_id TEXT NOT NULL REFERENCES admin_groups(group_id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES admin_users(user_id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			PRIMARY KEY(group_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS admin_permissions (
			permission_id TEXT PRIMARY KEY,
			resource TEXT NOT NULL,
			action TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			UNIQUE(resource, action)
		)`,
		`CREATE TABLE IF NOT EXISTS admin_group_permissions (
			group_id TEXT NOT NULL REFERENCES admin_groups(group_id) ON DELETE CASCADE,
			permission_id TEXT NOT NULL REFERENCES admin_permissions(permission_id) ON DELETE CASCADE,
			PRIMARY KEY(group_id, permission_id)
		)`,
		`CREATE TABLE IF NOT EXISTS admin_audit_events (
			event_id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_user_id TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL DEFAULT '',
			details_json TEXT NOT NULL DEFAULT '{}',
			request_id TEXT NOT NULL DEFAULT '',
			client_id TEXT NOT NULL DEFAULT '',
			surface TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_sessions (
			session_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES admin_users(user_id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			client_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			revoked_at TEXT NOT NULL DEFAULT ''
		)`,
		"CREATE INDEX IF NOT EXISTS admin_sessions_user_idx ON admin_sessions(user_id, expires_at)",
		"CREATE INDEX IF NOT EXISTS admin_group_members_user_idx ON admin_group_members(user_id, group_id)",
		"CREATE INDEX IF NOT EXISTS admin_audit_created_idx ON admin_audit_events(created_at DESC, event_id DESC)",
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate administrator store: %w", err)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin administrator seed: %w", err)
	}
	defer tx.Rollback()
	for _, seed := range adminPermissionSeeds {
		permissionID, err := allocateAdminNumber(ctx, tx, "admin_permission", "PERM-")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO admin_permissions(permission_id,resource,action,description) VALUES(?,?,?,?)`, permissionID, seed.resource, seed.action, seed.description); err != nil {
			return fmt.Errorf("seed administrator permission: %w", err)
		}
	}
	for _, seed := range adminGroupSeeds {
		groupID, err := allocateAdminNumber(ctx, tx, "admin_group", "GRP-")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO admin_groups(group_id,slug,name,description,built_in,created_at,updated_at) VALUES(?,?,?,?,1,?,?)`, groupID, seed.slug, seed.name, seed.description, stamp(time.Now().UTC()), stamp(time.Now().UTC())); err != nil {
			return fmt.Errorf("seed administrator group: %w", err)
		}
		var actualGroupID string
		if err := tx.QueryRowContext(ctx, `SELECT group_id FROM admin_groups WHERE slug = ?`, seed.slug).Scan(&actualGroupID); err != nil {
			return fmt.Errorf("resolve administrator group %s: %w", seed.slug, err)
		}
		for _, permission := range seed.permissions {
			if permission == "*" {
				rows, queryErr := tx.QueryContext(ctx, `SELECT permission_id FROM admin_permissions`)
				if queryErr != nil {
					return queryErr
				}
				for rows.Next() {
					var permissionID string
					if err := rows.Scan(&permissionID); err != nil {
						rows.Close()
						return err
					}
					if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO admin_group_permissions(group_id,permission_id) VALUES(?,?)`, actualGroupID, permissionID); err != nil {
						rows.Close()
						return err
					}
				}
				rows.Close()
				continue
			}
			parts := strings.SplitN(permission, ".", 2)
			if len(parts) != 2 {
				continue
			}
			resource, action := permission[:strings.LastIndex(permission, ".")], permission[strings.LastIndex(permission, ".")+1:]
			var permissionID string
			if err := tx.QueryRowContext(ctx, `SELECT permission_id FROM admin_permissions WHERE resource = ? AND action = ?`, resource, action).Scan(&permissionID); err != nil {
				return fmt.Errorf("resolve administrator permission %s: %w", permission, err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO admin_group_permissions(group_id,permission_id) VALUES(?,?)`, actualGroupID, permissionID); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit administrator seed: %w", err)
	}
	return nil
}

func allocateAdminNumber(ctx context.Context, tx *sql.Tx, name, prefix string) (string, error) {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO id_counters(name,next_value) VALUES(?,10000)`, name); err != nil {
		return "", fmt.Errorf("initialize %s sequence: %w", name, err)
	}
	var number int64
	if err := tx.QueryRowContext(ctx, `SELECT next_value FROM id_counters WHERE name = ?`, name).Scan(&number); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE id_counters SET next_value = next_value + 1 WHERE name = ?`, name); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", prefix, number), nil
}

func (s *Store) EnsureAdminPrincipal(ctx context.Context, externalID, displayName, kind string) (model.AdminPrincipal, error) {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		externalID = "anonymous"
	}
	if len(externalID) > 256 {
		return model.AdminPrincipal{}, errors.New("administrator identity is too long")
	}
	if kind != "human" && kind != "agent" && kind != "service" {
		kind = "human"
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = externalID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.AdminPrincipal{}, err
	}
	defer tx.Rollback()
	var userID, storedName, storedKind string
	var enabled int
	err = tx.QueryRowContext(ctx, `SELECT user_id,display_name,kind,enabled FROM admin_users WHERE external_id = ?`, externalID).Scan(&userID, &storedName, &storedKind, &enabled)
	bootstrap := false
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&count); err != nil {
			return model.AdminPrincipal{}, err
		}
		userID, err = allocateAdminNumber(ctx, tx, "admin_user", "USR-")
		if err != nil {
			return model.AdminPrincipal{}, err
		}
		storedName, storedKind = strings.TrimSpace(displayName), kind
		if count == 0 {
			bootstrap = true
		}
		now := stamp(time.Now().UTC())
		if _, err := tx.ExecContext(ctx, `INSERT INTO admin_users(user_id,external_id,display_name,kind,enabled,created_at,updated_at) VALUES(?,?,?,?,1,?,?)`, userID, externalID, storedName, storedKind, now, now); err != nil {
			return model.AdminPrincipal{}, err
		}
		enabled = 1
		groupSlug := "viewers"
		if bootstrap {
			groupSlug = "administrators"
		}
		var groupID string
		if err := tx.QueryRowContext(ctx, `SELECT group_id FROM admin_groups WHERE slug = ?`, groupSlug).Scan(&groupID); err != nil {
			return model.AdminPrincipal{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO admin_group_members(group_id,user_id,created_at) VALUES(?,?,?)`, groupID, userID, now); err != nil {
			return model.AdminPrincipal{}, err
		}
	} else if err != nil {
		return model.AdminPrincipal{}, err
	}
	if enabled == 0 {
		return model.AdminPrincipal{}, ErrAdminDisabled
	}
	if err := tx.Commit(); err != nil {
		return model.AdminPrincipal{}, err
	}
	user, err := s.getAdminUser(ctx, userID)
	if err != nil {
		return model.AdminPrincipal{}, err
	}
	permissions, err := s.adminPermissions(ctx, userID)
	if err != nil {
		return model.AdminPrincipal{}, err
	}
	return model.AdminPrincipal{User: user, Permissions: permissions, IsBootstrap: bootstrap}, nil
}

func (s *Store) HasAdminPermission(ctx context.Context, userID, permission string) (bool, error) {
	permissions, err := s.adminPermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, value := range permissions {
		if value == "*" || value == permission {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) ListAdminUsers(ctx context.Context) ([]model.AdminUser, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id,external_id,display_name,kind,enabled,created_at,updated_at FROM admin_users ORDER BY display_name COLLATE NOCASE,user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []model.AdminUser
	for rows.Next() {
		user, err := scanAdminUser(rows)
		if err != nil {
			return nil, err
		}
		user.Groups, err = s.adminGroupsForUser(ctx, user.UserID)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) SetAdminUserEnabled(ctx context.Context, userID string, enabled bool) error {
	if !enabled {
		var admins int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_group_members gm JOIN admin_groups g ON g.group_id = gm.group_id JOIN admin_users u ON u.user_id = gm.user_id WHERE g.slug = 'administrators' AND u.enabled = 1 AND u.user_id <> ?`, userID).Scan(&admins); err != nil {
			return err
		}
		if admins == 0 {
			return ErrAdminLastAdmin
		}
	}
	result, err := s.db.ExecContext(ctx, `UPDATE admin_users SET enabled = ?,updated_at = ? WHERE user_id = ?`, boolInt(enabled), stamp(time.Now().UTC()), userID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListAdminGroups(ctx context.Context) ([]model.AdminGroup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT group_id,slug,name,description,built_in,created_at,updated_at FROM admin_groups ORDER BY built_in DESC,name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []model.AdminGroup
	for rows.Next() {
		var group model.AdminGroup
		var builtIn int
		var created, updated string
		if err := rows.Scan(&group.GroupID, &group.Slug, &group.Name, &group.Description, &builtIn, &created, &updated); err != nil {
			return nil, err
		}
		group.BuiltIn = builtIn == 1
		group.CreatedAt, err = parseAdminTime(created)
		if err != nil {
			return nil, err
		}
		group.UpdatedAt, err = parseAdminTime(updated)
		if err != nil {
			return nil, err
		}
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_group_members WHERE group_id = ?`, group.GroupID).Scan(&group.MemberCount); err != nil {
			return nil, err
		}
		group.Permissions, err = s.adminGroupPermissions(ctx, group.GroupID)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *Store) CreateAdminGroup(ctx context.Context, input model.CreateAdminGroupInput) (model.AdminGroup, error) {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Name = strings.TrimSpace(input.Name)
	if !adminSlugPattern.MatchString(input.Slug) {
		return model.AdminGroup{}, errors.New("group slug must use lowercase letters, numbers, _ or -")
	}
	if input.Name == "" || len(input.Name) > 128 {
		return model.AdminGroup{}, errors.New("group name is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.AdminGroup{}, err
	}
	defer tx.Rollback()
	groupID, err := allocateAdminNumber(ctx, tx, "admin_group", "GRP-")
	if err != nil {
		return model.AdminGroup{}, err
	}
	now := stamp(time.Now().UTC())
	if _, err := tx.ExecContext(ctx, `INSERT INTO admin_groups(group_id,slug,name,description,built_in,created_at,updated_at) VALUES(?,?,?, ?,0,?,?)`, groupID, input.Slug, input.Name, strings.TrimSpace(input.Description), now, now); err != nil {
		return model.AdminGroup{}, fmt.Errorf("create group: %w", err)
	}
	for _, permission := range uniqueStrings(input.Permissions) {
		parts := strings.SplitN(permission, ".", 2)
		if len(parts) != 2 {
			return model.AdminGroup{}, fmt.Errorf("invalid permission %q", permission)
		}
		resource, action := permission[:strings.LastIndex(permission, ".")], permission[strings.LastIndex(permission, ".")+1:]
		var permissionID string
		if err := tx.QueryRowContext(ctx, `SELECT permission_id FROM admin_permissions WHERE resource = ? AND action = ?`, resource, action).Scan(&permissionID); err != nil {
			return model.AdminGroup{}, fmt.Errorf("unknown permission %q", permission)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO admin_group_permissions(group_id,permission_id) VALUES(?,?)`, groupID, permissionID); err != nil {
			return model.AdminGroup{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.AdminGroup{}, err
	}
	groups, err := s.ListAdminGroups(ctx)
	if err != nil {
		return model.AdminGroup{}, err
	}
	for _, group := range groups {
		if group.GroupID == groupID {
			return group, nil
		}
	}
	return model.AdminGroup{}, sql.ErrNoRows
}

func (s *Store) SetAdminGroupMembers(ctx context.Context, groupID string, userIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var groupSlug string
	if err := tx.QueryRowContext(ctx, `SELECT slug FROM admin_groups WHERE group_id = ?`, groupID).Scan(&groupSlug); err != nil {
		return err
	}
	if groupSlug == "administrators" && len(uniqueStrings(userIDs)) == 0 {
		return ErrAdminLastAdmin
	}
	if groupSlug == "administrators" {
		for _, userID := range uniqueStrings(userIDs) {
			var enabled int
			if err := tx.QueryRowContext(ctx, `SELECT enabled FROM admin_users WHERE user_id = ?`, userID).Scan(&enabled); err != nil {
				return err
			}
			if enabled == 0 {
				return ErrAdminLastAdmin
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_group_members WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	now := stamp(time.Now().UTC())
	for _, userID := range uniqueStrings(userIDs) {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users WHERE user_id = ?`, userID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return fmt.Errorf("user %s was not found", userID)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO admin_group_members(group_id,user_id,created_at) VALUES(?,?,?)`, groupID, userID, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListAdminPermissions(ctx context.Context) ([]model.AdminPermission, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT permission_id,resource,action,description FROM admin_permissions ORDER BY resource,action`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var permissions []model.AdminPermission
	for rows.Next() {
		var permission model.AdminPermission
		if err := rows.Scan(&permission.PermissionID, &permission.Resource, &permission.Action, &permission.Description); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func (s *Store) AppendAdminAudit(ctx context.Context, event model.AdminAuditEvent) error {
	if event.Details == "" {
		event.Details = "{}"
	}
	if !json.Valid([]byte(event.Details)) {
		return errors.New("administrator audit details must be valid JSON")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO admin_audit_events(actor_user_id,action,target_type,target_id,details_json,request_id,client_id,surface,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, event.ActorUserID, event.Action, event.TargetType, event.TargetID, event.Details, event.RequestID, event.ClientID, event.Surface, stamp(time.Now().UTC()))
	return err
}

func (s *Store) ListAdminAudit(ctx context.Context, limit int) ([]model.AdminAuditEvent, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id,actor_user_id,action,target_type,target_id,details_json,request_id,client_id,surface,created_at FROM admin_audit_events ORDER BY event_id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []model.AdminAuditEvent
	for rows.Next() {
		var event model.AdminAuditEvent
		var created string
		if err := rows.Scan(&event.EventID, &event.ActorUserID, &event.Action, &event.TargetType, &event.TargetID, &event.Details, &event.RequestID, &event.ClientID, &event.Surface, &created); err != nil {
			return nil, err
		}
		event.CreatedAt, err = parseAdminTime(created)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) getAdminUser(ctx context.Context, userID string) (model.AdminUser, error) {
	row := s.db.QueryRowContext(ctx, `SELECT user_id,external_id,display_name,kind,enabled,created_at,updated_at FROM admin_users WHERE user_id = ?`, userID)
	user, err := scanAdminUser(row)
	if err != nil {
		return model.AdminUser{}, err
	}
	user.Groups, err = s.adminGroupsForUser(ctx, userID)
	return user, err
}

func (s *Store) adminPermissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT p.resource || '.' || p.action FROM admin_permissions p JOIN admin_group_permissions gp ON gp.permission_id = p.permission_id JOIN admin_group_members gm ON gm.group_id = gp.group_id WHERE gm.user_id = ? ORDER BY 1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var permissions []string
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	var isAdmin int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_group_members gm JOIN admin_groups g ON g.group_id = gm.group_id WHERE gm.user_id = ? AND g.slug = 'administrators'`, userID).Scan(&isAdmin); err != nil {
		return nil, err
	}
	if isAdmin > 0 {
		permissions = append(permissions, "*")
	}
	return permissions, rows.Err()
}

func (s *Store) adminGroupsForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.slug FROM admin_groups g JOIN admin_group_members gm ON gm.group_id = g.group_id WHERE gm.user_id = ? ORDER BY g.slug`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []string
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *Store) adminGroupPermissions(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.resource || '.' || p.action FROM admin_permissions p JOIN admin_group_permissions gp ON gp.permission_id = p.permission_id WHERE gp.group_id = ? ORDER BY 1`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var permissions []string
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func scanAdminUser(scanner interface{ Scan(...any) error }) (model.AdminUser, error) {
	var user model.AdminUser
	var enabled int
	var created, updated string
	if err := scanner.Scan(&user.UserID, &user.ExternalID, &user.DisplayName, &user.Kind, &enabled, &created, &updated); err != nil {
		return model.AdminUser{}, err
	}
	var err error
	user.Enabled = enabled == 1
	user.CreatedAt, err = parseAdminTime(created)
	if err != nil {
		return model.AdminUser{}, err
	}
	user.UpdatedAt, err = parseAdminTime(updated)
	if err != nil {
		return model.AdminUser{}, err
	}
	return user, nil
}

func parseAdminTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse administrator timestamp: %w", err)
	}
	return parsed, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
