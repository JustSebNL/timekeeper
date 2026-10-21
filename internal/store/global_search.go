package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func (s *Store) SearchGlobal(ctx context.Context, input model.GlobalSearchInput) (model.GlobalSearchResponse, error) {
	query := strings.TrimSpace(input.Query)
	tokens := globalSearchTokens(query)
	if len(tokens) == 0 || utf8.RuneCountInString(query) > 200 {
		return model.GlobalSearchResponse{}, fmt.Errorf("%w: query must contain 1 to 200 searchable characters", ErrValidation)
	}
	limit := input.Limit
	if limit <= 0 {
		limit = model.DefaultGlobalSearchLimit
	}
	if limit > model.MaxGlobalSearchLimit {
		limit = model.MaxGlobalSearchLimit
	}
	scopes, err := normalizeGlobalSearchScopes(input.Scopes)
	if err != nil {
		return model.GlobalSearchResponse{}, err
	}
	projects, err := s.listGlobalSearchProjects(ctx, strings.TrimSpace(input.ProjectID))
	if err != nil {
		return model.GlobalSearchResponse{}, fmt.Errorf("list projects for global search: %w", err)
	}
	projectFilter := strings.TrimSpace(input.ProjectID)
	projectID := projectFilter
	projectExists := projectFilter == ""
	for _, project := range projects {
		if project.ProjectID == projectFilter {
			projectExists = true
		}
	}
	if !projectExists {
		return model.GlobalSearchResponse{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
	}

	items := make([]model.GlobalSearchResult, 0)
	projectCount, taskCount := 0, 0
	for _, project := range projects {
		if projectID != "" && project.ProjectID != projectID {
			continue
		}
		if containsScope(scopes, model.GlobalSearchScopeProjects) {
			text := strings.Join([]string{project.ProjectName, project.ProjectDescription, project.ProjectGoal, project.ProjectAlias}, " ")
			if score, ok := globalSearchScore(tokens, project.ProjectName, text, 300); ok {
				projectCount++
				items = append(items, model.GlobalSearchResult{Type: "project", ProjectID: project.ProjectID, ProjectName: project.ProjectName, ObjectID: project.ProjectID, Ref: project.ProjectID, Title: project.ProjectName, Status: project.Status, Snippet: globalSearchSnippet(text, tokens), Score: score, CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt})
			}
		}
		if containsScope(scopes, model.GlobalSearchScopeTasks) {
			tasks, listErr := s.ListTasks(ctx, project.ProjectID)
			if listErr != nil {
				return model.GlobalSearchResponse{}, fmt.Errorf("list tasks in %s for global search: %w", project.ProjectID, listErr)
			}
			for _, task := range tasks {
				text := strings.Join([]string{task.Name, task.Description, task.Goal, task.ItemAddress}, " ")
				if score, ok := globalSearchScore(tokens, task.Name, text, 200); ok {
					taskCount++
					items = append(items, model.GlobalSearchResult{Type: "task", ProjectID: project.ProjectID, ProjectName: project.ProjectName, ObjectID: task.TaskID, Ref: task.TaskID, Title: task.Name, Status: task.Status, Snippet: globalSearchSnippet(text, tokens), Score: score, CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt})
				}
			}
		}
	}
	messageCount := 0
	if containsScope(scopes, model.GlobalSearchScopeMessages) {
		var messageItems []model.GlobalSearchResult
		messageCount, messageItems, err = s.searchGlobalMessages(ctx, input, projectID, limit)
		if err != nil {
			return model.GlobalSearchResponse{}, err
		}
		items = append(items, messageItems...)
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Type == "message" && b.Type == "message" && a.Rank != b.Rank {
			return a.Rank < b.Rank
		}
		if strings.ToLower(a.ProjectName) != strings.ToLower(b.ProjectName) {
			return strings.ToLower(a.ProjectName) < strings.ToLower(b.ProjectName)
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if strings.ToLower(a.Title) != strings.ToLower(b.Title) {
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		}
		return a.ObjectID < b.ObjectID
	})
	total := projectCount + taskCount + messageCount
	hasMore := total > limit
	if len(items) > limit {
		items = items[:limit]
	}
	if items == nil {
		items = []model.GlobalSearchResult{}
	}
	return model.GlobalSearchResponse{Query: query, ProjectID: projectID, Scopes: scopes, Items: items, Total: total, Limit: limit, HasMore: hasMore}, nil
}

func (s *Store) listGlobalSearchProjects(ctx context.Context, projectID string) ([]model.Project, error) {
	query := `SELECT project_id,project_number,item_address,project_name,project_description,project_goal,status,priority,palette_id,
		reported_completion_pct,calculated_completion_pct,created_at,updated_at,project_alias,planned_start_date,target_end_date,
		actual_started_at,completed_at,cancelled_at,lifecycle_phase,schedule_timezone,deadline_warning_days FROM projects`
	args := []any{}
	if projectID != "" {
		query += ` WHERE project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY updated_at DESC,project_number DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query project search candidates: %w", err)
	}
	defer rows.Close()
	projects := make([]model.Project, 0)
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project search candidates: %w", err)
	}
	return projects, nil
}

func normalizeGlobalSearchScopes(input []string) ([]string, error) {
	allowed := map[string]bool{model.GlobalSearchScopeProjects: true, model.GlobalSearchScopeTasks: true, model.GlobalSearchScopeMessages: true}
	selected := map[string]bool{}
	for _, raw := range input {
		scope := strings.ToLower(strings.TrimSpace(raw))
		if scope == "" {
			continue
		}
		if !allowed[scope] {
			return nil, fmt.Errorf("%w: unknown search scope %q", ErrValidation, raw)
		}
		selected[scope] = true
	}
	if len(selected) == 0 {
		selected[model.GlobalSearchScopeProjects] = true
		selected[model.GlobalSearchScopeTasks] = true
		selected[model.GlobalSearchScopeMessages] = true
	}
	out := make([]string, 0, len(selected))
	for scope := range selected {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out, nil
}

func containsScope(scopes []string, target string) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func globalSearchTokens(query string) []string {
	var b strings.Builder
	for _, r := range strings.ToLower(query) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Fields(b.String())
}

func globalSearchScore(tokens []string, title, text string, base int) (int, bool) {
	fieldTokens := make(map[string]struct{})
	for _, token := range globalSearchTokens(text) {
		fieldTokens[token] = struct{}{}
	}
	for _, token := range tokens {
		if _, ok := fieldTokens[token]; !ok {
			return 0, false
		}
	}
	titleTokens := make(map[string]struct{})
	for _, token := range globalSearchTokens(title) {
		titleTokens[token] = struct{}{}
	}
	for _, token := range tokens {
		if _, ok := titleTokens[token]; !ok {
			return base + 10, true
		}
	}
	return base + 20, true
}

func globalSearchSnippet(text string, tokens []string) string {
	const maxRunes = 160
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	start := 0
	lower := strings.ToLower(string(runes))
	for _, token := range tokens {
		if pos := strings.Index(lower, token); pos >= 0 {
			start = utf8.RuneCountInString(lower[:pos])
			break
		}
	}
	start -= 40
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
		start = end - maxRunes
	}
	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}

func (s *Store) searchGlobalMessages(ctx context.Context, input model.GlobalSearchInput, projectID string, limit int) (int, []model.GlobalSearchResult, error) {
	match := sanitizeFtsQuery(input.Query)
	if match == "" {
		return 0, []model.GlobalSearchResult{}, nil
	}
	where := `project_messages_fts MATCH ?`
	args := []any{match}
	if projectID != "" {
		where += ` AND m.project_id = ?`
		args = append(args, projectID)
	}
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if kind != "" {
		if !model.IsValidMessageKind(model.MessageKind(kind)) {
			return 0, nil, fmt.Errorf("%w: invalid message kind %q", ErrValidation, input.Kind)
		}
		where += ` AND lower(m.kind) = ?`
		args = append(args, kind)
	}
	for _, rawTag := range input.Tags {
		tag := strings.ToLower(strings.TrimSpace(rawTag))
		if tag == "" {
			continue
		}
		if strings.ContainsAny(tag, ",;") {
			return 0, nil, fmt.Errorf("%w: each message tag filter must be one tag", ErrValidation)
		}
		where += ` AND instr(',' || lower(replace(replace(m.tags, ' ', ''), ';', ',')) || ',', ',' || ? || ',') > 0`
		args = append(args, tag)
	}
	if input.After != nil && input.Before != nil && input.After.After(*input.Before) {
		return 0, nil, fmt.Errorf("%w: after date must not be later than before date", ErrValidation)
	}
	if input.After != nil {
		where += ` AND m.created_at >= ?`
		args = append(args, input.After.UTC().Format(time.RFC3339Nano))
	}
	if input.Before != nil {
		where += ` AND m.created_at <= ?`
		args = append(args, input.Before.UTC().Format(time.RFC3339Nano))
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_messages_fts f JOIN project_messages m ON m.message_id=f.rowid WHERE `+where, args...).Scan(&total); err != nil {
		return 0, nil, fmt.Errorf("count global message matches: %w", err)
	}
	selectArgs := append(append([]any{}, args...), limit)
	rows, err := s.db.QueryContext(ctx, `SELECT m.message_id,m.project_id,p.project_name,m.kind,m.body,m.tags,m.created_at,m.updated_at,rank
		FROM project_messages_fts f JOIN project_messages m ON m.message_id=f.rowid JOIN projects p ON p.project_id=m.project_id
		WHERE `+where+` ORDER BY rank,m.project_id,m.message_id LIMIT ?`, selectArgs...)
	if err != nil {
		return 0, nil, fmt.Errorf("search global messages: %w", err)
	}
	defer rows.Close()
	items := make([]model.GlobalSearchResult, 0)
	for rows.Next() {
		var item model.GlobalSearchResult
		var id int64
		var kind, tags, body, created, updated string
		if err := rows.Scan(&id, &item.ProjectID, &item.ProjectName, &kind, &body, &tags, &created, &updated, &item.Rank); err != nil {
			return 0, nil, fmt.Errorf("scan global message result: %w", err)
		}
		item.Type = "message"
		item.ObjectID = fmt.Sprintf("%d", id)
		item.Ref = fmt.Sprintf("M-%d", id)
		item.Title = body
		item.Kind = kind
		item.Tags = tags
		item.Snippet = globalSearchSnippet(body, globalSearchTokens(input.Query))
		item.Score = 100
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return 0, nil, fmt.Errorf("parse global message timestamp: %w", err)
		}
		item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
		if err != nil {
			return 0, nil, fmt.Errorf("parse global message updated timestamp: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("iterate global message results: %w", err)
	}
	return total, items, nil
}
