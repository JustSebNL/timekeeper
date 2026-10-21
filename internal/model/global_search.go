package model

import "time"

const (
	GlobalSearchScopeProjects = "projects"
	GlobalSearchScopeTasks    = "tasks"
	GlobalSearchScopeMessages = "messages"
	DefaultGlobalSearchLimit  = 20
	MaxGlobalSearchLimit      = 200
)

type GlobalSearchInput struct {
	Query     string
	ProjectID string
	Scopes    []string
	Limit     int
	Kind      string
	Tags      []string
	After     *time.Time
	Before    *time.Time
}

type GlobalSearchResult struct {
	Type        string    `json:"type"`
	ProjectID   string    `json:"project_id"`
	ProjectName string    `json:"project_name"`
	ObjectID    string    `json:"object_id"`
	Ref         string    `json:"ref"`
	Title       string    `json:"title"`
	Status      string    `json:"status,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Tags        string    `json:"tags,omitempty"`
	Snippet     string    `json:"snippet"`
	Score       int       `json:"score"`
	Rank        float64   `json:"rank,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type GlobalSearchResponse struct {
	Query     string               `json:"query"`
	ProjectID string               `json:"project_id,omitempty"`
	Scopes    []string             `json:"scopes"`
	Items     []GlobalSearchResult `json:"items"`
	Total     int                  `json:"total"`
	Limit     int                  `json:"limit"`
	HasMore   bool                 `json:"has_more"`
}
