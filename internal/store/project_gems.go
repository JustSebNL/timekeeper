// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const projectGemColumns = `gem_id, project_id, parent_gem_id, task_id, sprint_id, category, subcategory, name, content, notes, tags, source_ref, revision, pinned, deleted_at, created_by, created_at, updated_at`

type projectGemScanner interface { Scan(dest ...any) error }

func validateProjectGemFields(category, subcategory, name, content, tags, sourceRef, actor string) error {
	category = strings.TrimSpace(category)
	subcategory = strings.TrimSpace(subcategory)
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	if category == "" || strings.ContainsAny(category, "\r\n") || utf8.RuneCountInString(category) > model.MaxProjectGemCategoryLength {
		return fmt.Errorf("%w: gem category is required, one line, and at most %d characters", ErrValidation, model.MaxProjectGemCategoryLength)
	}
	if strings.ContainsAny(subcategory, "\r\n") || utf8.RuneCountInString(subcategory) > model.MaxProjectGemCategoryLength {
		return fmt.Errorf("%w: gem subcategory must be one line and at most %d characters", ErrValidation, model.MaxProjectGemCategoryLength)
	}
	if name == "" || strings.ContainsAny(name, "\r\n") || utf8.RuneCountInString(name) > model.MaxProjectGemNameLength {
		return fmt.Errorf("%w: gem name must be one line and at most %d characters", ErrValidation, model.MaxProjectGemNameLength)
	}
	if content == "" || utf8.RuneCountInString(content) > model.MaxProjectGemContentLength {
		return fmt.Errorf("%w: gem content is required and must be at most %d characters", ErrValidation, model.MaxProjectGemContentLength)
	}
	if utf8.RuneCountInString(tags) > model.MaxProjectGemTagsLength {
		return fmt.Errorf("%w: gem tags must be at most %d characters", ErrValidation, model.MaxProjectGemTagsLength)
	}
	if utf8.RuneCountInString(sourceRef) > model.MaxProjectGemSourceRefLength {
		return fmt.Errorf("%w: gem source_ref must be at most %d characters", ErrValidation, model.MaxProjectGemSourceRefLength)
	}
	if utf8.RuneCountInString(actor) > model.MaxProjectGemAuthorLength {
		return fmt.Errorf("%w: gem actor must be at most %d characters", ErrValidation, model.MaxProjectGemAuthorLength)
	}
	return nil
}

func scanProjectGem(scanner projectGemScanner) (model.ProjectGem, error) {
	var gem model.ProjectGem
	var parentID sql.NullInt64
	var pinned int
	var deletedAt sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(&gem.GemID, &gem.ProjectID, &parentID, &gem.TaskID, &gem.SprintID, &gem.Category, &gem.Subcategory,
		&gem.Name, &gem.Content, &gem.Notes, &gem.Tags, &gem.SourceRef, &gem.Revision, &pinned,
		&deletedAt, &gem.CreatedBy, &createdAt, &updatedAt); err != nil {
		return model.ProjectGem{}, err
	}
	if parentID.Valid { value := parentID.Int64; gem.ParentGemID = &value }
	gem.Pinned = pinned != 0
	gem.Ref = fmt.Sprintf("G-%d", gem.GemID)
	var err error
	if gem.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return model.ProjectGem{}, fmt.Errorf("parse gem created_at: %w", err)
	}
	if updatedAt == "" { updatedAt = createdAt }
	if gem.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return model.ProjectGem{}, fmt.Errorf("parse gem updated_at: %w", err)
	}
	if deletedAt.Valid && deletedAt.String != "" {
		value, parseErr := time.Parse(time.RFC3339Nano, deletedAt.String)
		if parseErr != nil { return model.ProjectGem{}, fmt.Errorf("parse gem deleted_at: %w", parseErr) }
		gem.DeletedAt = &value
	}
	return gem, nil
}

func (s *Store) projectExistsForGemTx(ctx context.Context, tx *sql.Tx, projectID string) error {
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return fmt.Errorf("%w: project %s", ErrNotFound, projectID) }
		return fmt.Errorf("read gem project: %w", err)
	}
	return nil
}

func (s *Store) CreateProjectGem(ctx context.Context, projectID string, input model.CreateProjectGemInput) (model.ProjectGem, error) {
	projectID = strings.TrimSpace(projectID)
	input.Category = strings.TrimSpace(input.Category)
	input.Subcategory = strings.TrimSpace(input.Subcategory)
	input.Name = strings.TrimSpace(input.Name)
	input.Content = strings.TrimSpace(input.Content)
	input.Tags = strings.TrimSpace(input.Tags)
	input.SourceRef = strings.TrimSpace(input.SourceRef)
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	if input.ParentGemID != nil && *input.ParentGemID < 1 {
		return model.ProjectGem{}, fmt.Errorf("%w: parent_gem_id must be positive", ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return model.ProjectGem{}, fmt.Errorf("begin gem create: %w", err) }
	defer tx.Rollback()
	if err := s.projectExistsForGemTx(ctx, tx, projectID); err != nil { return model.ProjectGem{}, err }
	if input.ParentGemID != nil {
		parent, err := getProjectGemTx(ctx, tx, projectID, *input.ParentGemID, false)
		if err != nil { return model.ProjectGem{}, fmt.Errorf("read parent gem: %w", err) }
		if input.Category == "" { input.Category, input.Subcategory = parent.Category, parent.Subcategory }
	}
	if err := validateProjectGemFields(input.Category, input.Subcategory, input.Name, input.Content, input.Tags, input.SourceRef, input.CreatedBy); err != nil {
		return model.ProjectGem{}, err
	}
	now := time.Now().UTC().Round(0)
	var parent any
	if input.ParentGemID != nil { parent = *input.ParentGemID }
	result, err := tx.ExecContext(ctx, `INSERT INTO project_gems (
		project_id, parent_gem_id, task_id, sprint_id, category, subcategory, name, content, notes, tags, source_ref,
		revision, pinned, deleted_at, created_by, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, NULL, ?, ?, ?)`,
		projectID, parent, strings.TrimSpace(input.TaskID), strings.TrimSpace(input.SprintID), input.Category, input.Subcategory, input.Name, input.Content, input.Notes,
		input.Tags, input.SourceRef, input.CreatedBy, stamp(now), stamp(now))
	if err != nil { return model.ProjectGem{}, fmt.Errorf("insert project gem: %w", err) }
	gemID, err := result.LastInsertId()
	if err != nil { return model.ProjectGem{}, fmt.Errorf("read project gem id: %w", err) }
	gem := model.ProjectGem{GemID: gemID, Ref: fmt.Sprintf("G-%d", gemID), ProjectID: projectID,
		ParentGemID: input.ParentGemID, TaskID: strings.TrimSpace(input.TaskID), SprintID: strings.TrimSpace(input.SprintID),
		Category: input.Category, Subcategory: input.Subcategory, Name: input.Name, Content: input.Content,
		Notes: input.Notes, Tags: input.Tags, SourceRef: input.SourceRef, Revision: 1,
		CreatedBy: input.CreatedBy, CreatedAt: now, UpdatedAt: now}
	if err := insertProjectGemRevision(ctx, tx, gem, input.CreatedBy); err != nil { return model.ProjectGem{}, err }
	eventType := "gem_created"
	if gem.ParentGemID != nil { eventType = "gem_attached" }
	if err := recordProjectEvent(ctx, tx, projectID, "gem", gem.Ref, eventType, gem.Ref+" stored in the Treasure Chest", now); err != nil {
		return model.ProjectGem{}, fmt.Errorf("record gem event: %w", err)
	}
	if err := tx.Commit(); err != nil { return model.ProjectGem{}, fmt.Errorf("commit project gem: %w", err) }
	return gem, nil
}

func insertProjectGemRevision(ctx context.Context, tx *sql.Tx, gem model.ProjectGem, editedBy string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO project_gem_revisions (
		project_id, gem_id, revision, category, subcategory, name, content, tags, source_ref, edited_by, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, gem.ProjectID, gem.GemID, gem.Revision,
		gem.Category, gem.Subcategory, gem.Name, gem.Content, gem.Tags, gem.SourceRef, editedBy, stamp(gem.UpdatedAt))
	if err != nil { return fmt.Errorf("insert project gem revision: %w", err) }
	return nil
}

func getProjectGemTx(ctx context.Context, tx *sql.Tx, projectID string, gemID int64, includeDeleted bool) (model.ProjectGem, error) {
	query := `SELECT ` + projectGemColumns + ` FROM project_gems WHERE project_id = ? AND gem_id = ?`
	if !includeDeleted { query += ` AND deleted_at IS NULL` }
	gem, err := scanProjectGem(tx.QueryRowContext(ctx, query, projectID, gemID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) { return model.ProjectGem{}, fmt.Errorf("%w: gem G-%d", ErrNotFound, gemID) }
		return model.ProjectGem{}, fmt.Errorf("read project gem: %w", err)
	}
	return gem, nil
}

func (s *Store) GetProjectGem(ctx context.Context, projectID string, gemID int64, includeDeleted bool) (model.ProjectGem, error) {
	if gemID < 1 { return model.ProjectGem{}, fmt.Errorf("%w: gem id must be positive", ErrValidation) }
	query := `SELECT ` + projectGemColumns + ` FROM project_gems WHERE project_id = ? AND gem_id = ?`
	if !includeDeleted { query += ` AND deleted_at IS NULL` }
	gem, err := scanProjectGem(s.db.QueryRowContext(ctx, query, projectID, gemID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) { return model.ProjectGem{}, fmt.Errorf("%w: gem G-%d", ErrNotFound, gemID) }
		return model.ProjectGem{}, fmt.Errorf("get project gem: %w", err)
	}
	return gem, nil
}

func (s *Store) ListProjectGems(ctx context.Context, projectID string, filter model.ProjectGemFilter) (model.ProjectGemList, error) {
	projectID = strings.TrimSpace(projectID)
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Category = strings.TrimSpace(filter.Category)
	filter.Subcategory = strings.TrimSpace(filter.Subcategory)
	limit := filter.Limit
	if limit <= 0 { limit = model.DefaultProjectGemListLimit }
	if limit > model.MaxProjectGemListLimit { limit = model.MaxProjectGemListLimit }
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return model.ProjectGemList{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID) }
		return model.ProjectGemList{}, fmt.Errorf("read gem list project: %w", err)
	}
	where := `project_id = ?`
	args := []any{projectID}
	if !filter.IncludeDeleted { where += ` AND deleted_at IS NULL` }
	if filter.Category != "" { where += ` AND category = ? COLLATE NOCASE`; args = append(args, filter.Category) }
	if filter.Subcategory != "" { where += ` AND subcategory = ? COLLATE NOCASE`; args = append(args, filter.Subcategory) }
	if filter.TaskID != "" { where += ` AND task_id = ?`; args = append(args, strings.TrimSpace(filter.TaskID)) }
	if filter.SprintID != "" { where += ` AND sprint_id = ?`; args = append(args, strings.TrimSpace(filter.SprintID)) }
	if filter.ParentGemID != nil { where += ` AND parent_gem_id = ?`; args = append(args, *filter.ParentGemID) }
	if filter.PinnedOnly { where += ` AND pinned = 1` }
	if filter.Query != "" {
		if utf8.RuneCountInString(filter.Query) > 512 {
			return model.ProjectGemList{}, fmt.Errorf("%w: gem query must be at most 512 characters", ErrValidation)
		}
		terms := strings.Fields(filter.Query)
		for _, term := range terms {
			where += ` AND (instr(lower(name),lower(?))>0 OR instr(lower(content),lower(?))>0 OR instr(lower(notes),lower(?))>0 OR instr(lower(category),lower(?))>0 OR instr(lower(subcategory),lower(?))>0 OR instr(lower(tags),lower(?))>0 OR instr(lower(source_ref),lower(?))>0)`
			for i := 0; i < 7; i++ {
				args = append(args, term)
			}
		}
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_gems WHERE `+where, args...).Scan(&total); err != nil {
		return model.ProjectGemList{}, fmt.Errorf("count project gems: %w", err)
	}
	query := `SELECT ` + projectGemColumns + ` FROM project_gems WHERE ` + where + ` ORDER BY pinned DESC, category COLLATE NOCASE, subcategory COLLATE NOCASE, updated_at DESC, gem_id DESC LIMIT ?`
	queryArgs := append(append([]any{}, args...), limit)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil { return model.ProjectGemList{}, fmt.Errorf("list project gems: %w", err) }
	defer rows.Close()
	items := make([]model.ProjectGem, 0)
	for rows.Next() {
		gem, err := scanProjectGem(rows)
		if err != nil { return model.ProjectGemList{}, fmt.Errorf("scan project gem: %w", err) }
		items = append(items, gem)
	}
	if err := rows.Err(); err != nil { return model.ProjectGemList{}, fmt.Errorf("iterate project gems: %w", err) }
	return model.ProjectGemList{Items: items, Total: total, Limit: limit, HasMore: total > len(items)}, nil
}

func (s *Store) UpdateProjectGem(ctx context.Context, projectID string, gemID int64, input model.UpdateProjectGemInput) (model.ProjectGem, error) {
	input.Category = strings.TrimSpace(input.Category)
	input.Subcategory = strings.TrimSpace(input.Subcategory)
	input.Name = strings.TrimSpace(input.Name)
	input.Content = strings.TrimSpace(input.Content)
	input.Tags = strings.TrimSpace(input.Tags)
	input.SourceRef = strings.TrimSpace(input.SourceRef)
	input.EditedBy = strings.TrimSpace(input.EditedBy)
	if err := validateProjectGemFields(input.Category, input.Subcategory, input.Name, input.Content, input.Tags, input.SourceRef, input.EditedBy); err != nil { return model.ProjectGem{}, err }
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return model.ProjectGem{}, fmt.Errorf("begin gem edit: %w", err) }
	defer tx.Rollback()
	current, err := getProjectGemTx(ctx, tx, projectID, gemID, false)
	if err != nil { return model.ProjectGem{}, err }
	now := time.Now().UTC().Round(0)
	updated := current
	updated.Category, updated.Subcategory = input.Category, input.Subcategory
	updated.Name, updated.Content = input.Name, input.Content
	updated.Tags, updated.SourceRef = input.Tags, input.SourceRef
	updated.Notes = input.Notes
	updated.TaskID = strings.TrimSpace(input.TaskID)
	updated.SprintID = strings.TrimSpace(input.SprintID)
	updated.Revision++
	updated.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, `UPDATE project_gems SET category=?, subcategory=?, name=?, content=?, notes=?, tags=?, source_ref=?, task_id=?, sprint_id=?, revision=?, updated_at=? WHERE project_id=? AND gem_id=?`,
		updated.Category, updated.Subcategory, updated.Name, updated.Content, updated.Notes, updated.Tags, updated.SourceRef, updated.TaskID, updated.SprintID,
		updated.Revision, stamp(now), projectID, gemID); err != nil { return model.ProjectGem{}, fmt.Errorf("update project gem: %w", err) }
	if err := insertProjectGemRevision(ctx, tx, updated, input.EditedBy); err != nil { return model.ProjectGem{}, err }
	if err := recordProjectEvent(ctx, tx, projectID, "gem", updated.Ref, "gem_edited", updated.Ref+" edited to revision "+fmt.Sprint(updated.Revision), now); err != nil {
		return model.ProjectGem{}, fmt.Errorf("record gem edit event: %w", err)
	}
	if err := tx.Commit(); err != nil { return model.ProjectGem{}, fmt.Errorf("commit project gem edit: %w", err) }
	return updated, nil
}

func (s *Store) SetProjectGemPinned(ctx context.Context, projectID string, gemID int64, pinned bool) (model.ProjectGem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return model.ProjectGem{}, fmt.Errorf("begin gem pin: %w", err) }
	defer tx.Rollback()
	gem, err := getProjectGemTx(ctx, tx, projectID, gemID, false)
	if err != nil { return model.ProjectGem{}, err }
	if gem.Pinned == pinned {
		if err := tx.Commit(); err != nil { return model.ProjectGem{}, fmt.Errorf("commit unchanged gem pin: %w", err) }
		return gem, nil
	}
	now := time.Now().UTC().Round(0)
	value, eventType := 0, "gem_unpinned"
	if pinned { value, eventType = 1, "gem_pinned" }
	if _, err := tx.ExecContext(ctx, "UPDATE project_gems SET pinned=?, updated_at=? WHERE project_id=? AND gem_id=?", value, stamp(now), projectID, gemID); err != nil {
		return model.ProjectGem{}, fmt.Errorf("update gem pin: %w", err)
	}
	gem.Pinned, gem.UpdatedAt = pinned, now
	if err := recordProjectEvent(ctx, tx, projectID, "gem", gem.Ref, eventType, gem.Ref+" pin state changed", now); err != nil { return model.ProjectGem{}, fmt.Errorf("record gem pin event: %w", err) }
	if err := tx.Commit(); err != nil { return model.ProjectGem{}, fmt.Errorf("commit gem pin: %w", err) }
	return gem, nil
}

func (s *Store) DeleteProjectGem(ctx context.Context, projectID string, gemID int64) (model.ProjectGem, error) {
	return s.setProjectGemDeleted(ctx, projectID, gemID, true)
}

func (s *Store) RestoreProjectGem(ctx context.Context, projectID string, gemID int64) (model.ProjectGem, error) {
	return s.setProjectGemDeleted(ctx, projectID, gemID, false)
}

func (s *Store) setProjectGemDeleted(ctx context.Context, projectID string, gemID int64, deleted bool) (model.ProjectGem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return model.ProjectGem{}, fmt.Errorf("begin gem delete state: %w", err) }
	defer tx.Rollback()
	gem, err := getProjectGemTx(ctx, tx, projectID, gemID, true)
	if err != nil { return model.ProjectGem{}, err }
	if deleted == (gem.DeletedAt != nil) {
		if err := tx.Commit(); err != nil { return model.ProjectGem{}, fmt.Errorf("commit unchanged gem delete state: %w", err) }
		return gem, nil
	}
	now := time.Now().UTC().Round(0)
	var deletedAt any
	eventType, message := "gem_restored", gem.Ref+" restored to the Treasure Chest"
	if deleted { deletedAt, eventType, message = stamp(now), "gem_deleted", gem.Ref+" hidden from the Treasure Chest" }
	if _, err := tx.ExecContext(ctx, "UPDATE project_gems SET deleted_at=?, updated_at=? WHERE project_id=? AND gem_id=?", deletedAt, stamp(now), projectID, gemID); err != nil {
		return model.ProjectGem{}, fmt.Errorf("update gem delete state: %w", err)
	}
	if deleted { gem.DeletedAt = &now } else { gem.DeletedAt = nil }
	gem.UpdatedAt = now
	if err := recordProjectEvent(ctx, tx, projectID, "gem", gem.Ref, eventType, message, now); err != nil { return model.ProjectGem{}, fmt.Errorf("record gem delete event: %w", err) }
	if err := tx.Commit(); err != nil { return model.ProjectGem{}, fmt.Errorf("commit gem delete state: %w", err) }
	return gem, nil
}

func (s *Store) ListProjectGemCategories(ctx context.Context, projectID string) ([]model.ProjectGemCategory, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return nil, fmt.Errorf("%w: project %s", ErrNotFound, projectID) }
		return nil, fmt.Errorf("read gem categories project: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT category, subcategory FROM project_gems WHERE project_id=? AND deleted_at IS NULL ORDER BY category COLLATE NOCASE, subcategory COLLATE NOCASE`, projectID)
	if err != nil { return nil, fmt.Errorf("list project gem categories: %w", err) }
	defer rows.Close()
	indexByCategory := make(map[string]int)
	categories := make([]model.ProjectGemCategory, 0)
	for rows.Next() {
		var category, subcategory string
		if err := rows.Scan(&category, &subcategory); err != nil { return nil, fmt.Errorf("scan project gem category: %w", err) }
		index, ok := indexByCategory[category]
		if !ok { index = len(categories); indexByCategory[category] = index; categories = append(categories, model.ProjectGemCategory{Category: category, Subcategories: []string{}}) }
		if subcategory != "" { categories[index].Subcategories = append(categories[index].Subcategories, subcategory) }
	}
	if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate project gem categories: %w", err) }
	return categories, nil
}

func (s *Store) ListProjectGemRevisions(ctx context.Context, projectID string, gemID int64) ([]model.ProjectGemRevision, error) {
	if _, err := s.GetProjectGem(ctx, projectID, gemID, true); err != nil { return nil, err }
	rows, err := s.db.QueryContext(ctx, `SELECT revision_id, gem_id, project_id, revision, category, subcategory, name, content, tags, source_ref, edited_by, created_at
		FROM project_gem_revisions WHERE project_id=? AND gem_id=? ORDER BY revision DESC`, projectID, gemID)
	if err != nil { return nil, fmt.Errorf("list project gem revisions: %w", err) }
	defer rows.Close()
	revisions := make([]model.ProjectGemRevision, 0)
	for rows.Next() {
		var revision model.ProjectGemRevision
		var createdAt string
		if err := rows.Scan(&revision.RevisionID, &revision.GemID, &revision.ProjectID, &revision.Revision, &revision.Category, &revision.Subcategory,
			&revision.Name, &revision.Content, &revision.Tags, &revision.SourceRef, &revision.EditedBy, &createdAt); err != nil { return nil, fmt.Errorf("scan project gem revision: %w", err) }
		if revision.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil { return nil, fmt.Errorf("parse gem revision created_at: %w", err) }
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}
