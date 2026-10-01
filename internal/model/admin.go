package model

import "time"

// AdminUser is a local identity used by the authorization layer. ExternalID
// is the stable identity supplied by the local client or an future provider.
type AdminUser struct {
	UserID      string    `json:"user_id"`
	ExternalID  string    `json:"external_id"`
	DisplayName string    `json:"display_name"`
	Kind        string    `json:"kind"`
	Enabled     bool      `json:"enabled"`
	Groups      []string  `json:"groups"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AdminGroup struct {
	GroupID     string    `json:"group_id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	BuiltIn     bool      `json:"built_in"`
	MemberCount int       `json:"member_count"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AdminPermission struct {
	PermissionID string `json:"permission_id"`
	Resource     string `json:"resource"`
	Action       string `json:"action"`
	Description  string `json:"description"`
}

type AdminAuditEvent struct {
	EventID     int64     `json:"event_id"`
	ActorUserID string    `json:"actor_user_id"`
	Action      string    `json:"action"`
	TargetType  string    `json:"target_type"`
	TargetID    string    `json:"target_id"`
	Details     string    `json:"details"`
	RequestID   string    `json:"request_id,omitempty"`
	ClientID    string    `json:"client_id,omitempty"`
	Surface     string    `json:"surface,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type AdminPrincipal struct {
	User        AdminUser `json:"user"`
	Permissions []string  `json:"permissions"`
	IsBootstrap bool      `json:"is_bootstrap"`
}

type CreateAdminGroupInput struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type UpdateAdminUserInput struct {
	Enabled *bool `json:"enabled"`
}

type UpdateAdminGroupMembersInput struct {
	UserIDs []string `json:"user_ids"`
}
