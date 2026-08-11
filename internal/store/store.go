// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/lifecycle"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/planning"
	_ "modernc.org/sqlite"
)

// Store owns Time Keeper's authoritative SQLite connection.
type Store struct {
	db *sql.DB
}

// Open opens a SQLite database, enables required safety settings, and applies migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := secureLocalDatabaseFile(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func secureLocalDatabaseFile(path string) error {
	path = strings.TrimSpace(path)
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure SQLite database permissions: %w", err)
	}
	return nil
}

// Close releases the SQLite connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// BackupTo creates a self-contained SQLite snapshot at a new destination path.
// It never overwrites an existing file.
func (s *Store) BackupTo(ctx context.Context, destination string) error {
	if s == nil || s.db == nil {
		return errors.New("backup Time Keeper database: store is closed")
	}
	path, err := filepath.Abs(strings.TrimSpace(destination))
	if err != nil || strings.TrimSpace(destination) == "" {
		return fmt.Errorf("backup destination: provide a valid path")
	}
	if info, statErr := os.Stat(path); statErr == nil {
		if info.IsDir() {
			return fmt.Errorf("backup destination %q is a directory", path)
		}
		return fmt.Errorf("backup destination %q already exists", path)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect backup destination: %w", statErr)
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("backup destination parent: %w", err)
	}
	if !parent.IsDir() {
		return fmt.Errorf("backup destination parent %q is not a directory", filepath.Dir(path))
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("create SQLite backup: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("secure SQLite backup permissions: %w", err)
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		`CREATE TABLE IF NOT EXISTS id_counters (
			name TEXT PRIMARY KEY,
			next_value INTEGER NOT NULL CHECK (next_value >= 10000)
		)`,
		`CREATE TABLE IF NOT EXISTS projects (
			project_id TEXT PRIMARY KEY,
			project_number INTEGER NOT NULL UNIQUE CHECK (project_number >= 10000 AND project_number <= 99999999),
			item_address TEXT NOT NULL UNIQUE,
			project_name TEXT NOT NULL COLLATE NOCASE UNIQUE,
			project_description TEXT NOT NULL DEFAULT '',
			project_goal TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			priority TEXT NOT NULL,
			palette_id INTEGER NOT NULL CHECK (palette_id BETWEEN 1 AND 20),
			reported_completion_pct REAL NOT NULL DEFAULT 0 CHECK (reported_completion_pct BETWEEN 0 AND 100),
			calculated_completion_pct REAL NOT NULL DEFAULT 0 CHECK (calculated_completion_pct BETWEEN 0 AND 100),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		"CREATE INDEX IF NOT EXISTS projects_status_updated_idx ON projects(status, updated_at DESC)",
		`CREATE TABLE IF NOT EXISTS categories (
			category_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
			parent_category_id TEXT REFERENCES categories(category_id) ON DELETE RESTRICT,
			item_address TEXT NOT NULL UNIQUE,
			category_name TEXT NOT NULL COLLATE NOCASE,
			category_description TEXT NOT NULL DEFAULT '',
			category_goal TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			priority TEXT NOT NULL,
			progress_pct REAL NOT NULL DEFAULT 0 CHECK (progress_pct BETWEEN 0 AND 100),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(project_id, category_name)
		)`,
		"CREATE INDEX IF NOT EXISTS categories_project_updated_idx ON categories(project_id, updated_at DESC)",
		`CREATE TABLE IF NOT EXISTS tasks (
			task_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
			category_id TEXT NOT NULL REFERENCES categories(category_id) ON DELETE RESTRICT,
			item_address TEXT NOT NULL UNIQUE,
			task_name TEXT NOT NULL COLLATE NOCASE,
			task_description TEXT NOT NULL DEFAULT '',
			task_goal TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			priority TEXT NOT NULL,
			estimated_duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (estimated_duration_seconds >= 0),
			reported_completion_pct REAL NOT NULL DEFAULT 0 CHECK (reported_completion_pct BETWEEN 0 AND 100),
			calculated_completion_pct REAL NOT NULL DEFAULT 0 CHECK (calculated_completion_pct BETWEEN 0 AND 100),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(category_id, task_name)
		)`,
		"CREATE INDEX IF NOT EXISTS tasks_project_category_updated_idx ON tasks(project_id, category_id, updated_at DESC)",
		`CREATE TABLE IF NOT EXISTS subtasks (
			subtask_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
			category_id TEXT NOT NULL REFERENCES categories(category_id) ON DELETE RESTRICT,
			task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
			item_address TEXT NOT NULL UNIQUE,
			subtask_name TEXT NOT NULL COLLATE NOCASE,
			subtask_description TEXT NOT NULL DEFAULT '',
			subtask_goal TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			priority TEXT NOT NULL,
			estimated_duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (estimated_duration_seconds >= 0),
			reported_completion_pct REAL NOT NULL DEFAULT 0 CHECK (reported_completion_pct BETWEEN 0 AND 100),
			calculated_completion_pct REAL NOT NULL DEFAULT 0 CHECK (calculated_completion_pct BETWEEN 0 AND 100),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(task_id, subtask_name)
		)`,
		"CREATE INDEX IF NOT EXISTS subtasks_task_updated_idx ON subtasks(task_id, updated_at DESC)",
		`CREATE TABLE IF NOT EXISTS sprints (
			sprint_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
			category_id TEXT NOT NULL REFERENCES categories(category_id) ON DELETE RESTRICT,
			task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
			subtask_id TEXT REFERENCES subtasks(subtask_id) ON DELETE RESTRICT,
			item_address TEXT NOT NULL UNIQUE,
			sprint_name TEXT NOT NULL COLLATE NOCASE,
			sprint_description TEXT NOT NULL DEFAULT '',
			sprint_goal TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			priority TEXT NOT NULL,
			estimated_duration_seconds INTEGER NOT NULL CHECK (estimated_duration_seconds >= 0),
			buffer_pct REAL NOT NULL CHECK (buffer_pct BETWEEN 0 AND 100),
			buffer_duration_seconds INTEGER NOT NULL CHECK (buffer_duration_seconds >= 0),
			active_duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (active_duration_seconds >= 0),
			hold_duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (hold_duration_seconds >= 0),
			started_at TEXT,
			ended_at TEXT,
			active_started_at TEXT,
			hold_started_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(task_id, sprint_name)
		)`,
		"CREATE INDEX IF NOT EXISTS sprints_task_updated_idx ON sprints(task_id, updated_at DESC)",
		`CREATE TABLE IF NOT EXISTS time_entries (
			time_entry_id INTEGER PRIMARY KEY AUTOINCREMENT,
			sprint_id TEXT NOT NULL REFERENCES sprints(sprint_id) ON DELETE RESTRICT,
			entry_type TEXT NOT NULL CHECK (entry_type IN ('work', 'hold')),
			started_at TEXT NOT NULL,
			ended_at TEXT NOT NULL,
			duration_seconds INTEGER NOT NULL CHECK (duration_seconds >= 0),
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sprint_time_extensions (
			extension_id INTEGER PRIMARY KEY AUTOINCREMENT,
			sprint_id TEXT NOT NULL REFERENCES sprints(sprint_id) ON DELETE RESTRICT,
			duration_seconds INTEGER NOT NULL CHECK (duration_seconds > 0),
			reason TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 10000),
			created_at TEXT NOT NULL
		)`,
		"CREATE INDEX IF NOT EXISTS sprint_time_extensions_sprint_created_idx ON sprint_time_extensions(sprint_id, created_at, extension_id)",
		`CREATE TABLE IF NOT EXISTS project_events (
			event_id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		"CREATE INDEX IF NOT EXISTS project_events_project_created_idx ON project_events(project_id, created_at DESC, event_id DESC)",
		`CREATE TABLE IF NOT EXISTS project_notes (
			note_id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
			content TEXT NOT NULL CHECK (length(content) BETWEEN 1 AND 10000),
			actor_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		"CREATE INDEX IF NOT EXISTS project_notes_project_created_idx ON project_notes(project_id, created_at DESC, note_id DESC)",
		`CREATE TABLE IF NOT EXISTS llm_pipelines (
			pipeline_id INTEGER PRIMARY KEY AUTOINCREMENT,
			pipeline_name TEXT NOT NULL COLLATE NOCASE UNIQUE,
			provider TEXT NOT NULL,
			base_url TEXT NOT NULL,
			model_name TEXT NOT NULL,
			system_prompt TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS planning_drafts (
			draft_id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
			pipeline_id INTEGER NOT NULL REFERENCES llm_pipelines(pipeline_id) ON DELETE RESTRICT,
			status TEXT NOT NULL CHECK (status IN ('Review', 'Applied', 'Rejected')),
			summary TEXT NOT NULL,
			raw_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			applied_at TEXT
		)`,
		"CREATE INDEX IF NOT EXISTS planning_drafts_project_created_idx ON planning_drafts(project_id, created_at DESC, draft_id DESC)",
		"CREATE INDEX IF NOT EXISTS time_entries_sprint_started_idx ON time_entries(sprint_id, started_at, time_entry_id)",
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}
	if err := s.ensureColumn(ctx, "categories", "parent_category_id", "TEXT REFERENCES categories(category_id) ON DELETE RESTRICT"); err != nil {
		return fmt.Errorf("migrate category parent ownership: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS categories_parent_updated_idx ON categories(parent_category_id, updated_at DESC)"); err != nil {
		return fmt.Errorf("migrate category parent index: %w", err)
	}
	if err := s.ensureColumn(ctx, "sprints", "subtask_id", "TEXT REFERENCES subtasks(subtask_id) ON DELETE RESTRICT"); err != nil {
		return fmt.Errorf("migrate sprints subtask ownership: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS sprints_subtask_updated_idx ON sprints(subtask_id, updated_at DESC)"); err != nil {
		return fmt.Errorf("migrate sprints subtask index: %w", err)
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var columnNumber, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&columnNumber, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition)
	return err
}

// CreateProject persists a new project and assigns its durable public ID.
func (s *Store) CreateProject(ctx context.Context, input model.CreateProjectInput) (model.Project, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return model.Project{}, errors.New("project name is required")
	}
	if input.Priority == "" {
		input.Priority = "Normal"
	}
	if input.PaletteID == 0 {
		input.PaletteID = 1
	}
	if input.PaletteID < 1 || input.PaletteID > 20 {
		return model.Project{}, errors.New("palette_id must be between 1 and 20")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Project{}, fmt.Errorf("begin project creation: %w", err)
	}
	defer tx.Rollback()
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.Project{}, err
	}

	now := time.Now().UTC().Round(0)
	project := model.Project{
		ProjectID:          fmt.Sprintf("P-%d", number),
		ProjectNumber:      number,
		ItemAddress:        fmt.Sprintf("%d", number),
		ProjectName:        input.Name,
		ProjectDescription: strings.TrimSpace(input.Description),
		ProjectGoal:        strings.TrimSpace(input.Goal),
		Status:             "Open",
		Priority:           input.Priority,
		PaletteID:          input.PaletteID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO projects (
		project_id, project_number, item_address, project_name, project_description,
		project_goal, status, priority, palette_id, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		project.ProjectID, project.ProjectNumber, project.ItemAddress, project.ProjectName,
		project.ProjectDescription, project.ProjectGoal, project.Status, project.Priority,
		project.PaletteID, project.CreatedAt.Format(time.RFC3339Nano), project.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return model.Project{}, fmt.Errorf("insert project: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.Project{}, fmt.Errorf("commit project creation: %w", err)
	}
	return project, nil
}

// CreateLLMPipeline persists an optional loopback-only inference connection.
func (s *Store) CreateLLMPipeline(ctx context.Context, input model.CreateLLMPipelineInput) (model.LLMPipeline, error) {
	input.Name, input.Provider, input.BaseURL, input.Model, input.SystemPrompt = strings.TrimSpace(input.Name), strings.TrimSpace(input.Provider), strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"), strings.TrimSpace(input.Model), strings.TrimSpace(input.SystemPrompt)
	if len(input.SystemPrompt) > 10000 {
		return model.LLMPipeline{}, errors.New("pipeline system prompt must be at most 10000 characters")
	}
	if input.Name == "" || input.Model == "" {
		return model.LLMPipeline{}, errors.New("pipeline name and model are required")
	}
	if input.Provider != "ollama" && input.Provider != "openai-compatible" {
		return model.LLMPipeline{}, errors.New("pipeline provider must be ollama or openai-compatible")
	}
	endpoint, err := url.Parse(input.BaseURL)
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return model.LLMPipeline{}, errors.New("pipeline base_url must be a plain http numeric-loopback URL")
	}
	host := net.ParseIP(endpoint.Hostname())
	if host == nil || !host.IsLoopback() {
		return model.LLMPipeline{}, errors.New("pipeline base_url must use a numeric loopback host")
	}
	now := time.Now().UTC().Round(0)
	result, err := s.db.ExecContext(ctx, `INSERT INTO llm_pipelines (pipeline_name, provider, base_url, model_name, system_prompt, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, input.Name, input.Provider, input.BaseURL, input.Model, strings.TrimSpace(input.SystemPrompt), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return model.LLMPipeline{}, fmt.Errorf("create LLM pipeline: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.LLMPipeline{}, fmt.Errorf("read pipeline ID: %w", err)
	}
	return model.LLMPipeline{PipelineID: id, Name: input.Name, Provider: input.Provider, BaseURL: input.BaseURL, Model: input.Model, SystemPrompt: strings.TrimSpace(input.SystemPrompt), CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) GetLLMPipeline(ctx context.Context, pipelineID int64) (model.LLMPipeline, error) {
	var item model.LLMPipeline
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT pipeline_id, pipeline_name, provider, base_url, model_name, system_prompt, created_at, updated_at FROM llm_pipelines WHERE pipeline_id = ?`, pipelineID).Scan(&item.PipelineID, &item.Name, &item.Provider, &item.BaseURL, &item.Model, &item.SystemPrompt, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return model.LLMPipeline{}, fmt.Errorf("%w: LLM pipeline %d", ErrNotFound, pipelineID)
	}
	if err != nil {
		return model.LLMPipeline{}, err
	}
	var parseErr error
	item.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, created)
	if parseErr != nil {
		return model.LLMPipeline{}, parseErr
	}
	item.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updated)
	if parseErr != nil {
		return model.LLMPipeline{}, parseErr
	}
	return item, nil
}

func (s *Store) ListLLMPipelines(ctx context.Context) ([]model.LLMPipeline, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pipeline_id, pipeline_name, provider, base_url, model_name, system_prompt, created_at, updated_at FROM llm_pipelines ORDER BY pipeline_id`)
	if err != nil {
		return nil, fmt.Errorf("list LLM pipelines: %w", err)
	}
	defer rows.Close()
	items := make([]model.LLMPipeline, 0)
	for rows.Next() {
		var item model.LLMPipeline
		var created, updated string
		if err := rows.Scan(&item.PipelineID, &item.Name, &item.Provider, &item.BaseURL, &item.Model, &item.SystemPrompt, &created, &updated); err != nil {
			return nil, err
		}
		var err error
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// CreatePlanningDraft persists a validated, non-mutating proposal for review.
func (s *Store) CreatePlanningDraft(ctx context.Context, projectID string, pipelineID int64, rawJSON string) (model.PlanningDraft, error) {
	rawJSON = strings.TrimSpace(rawJSON)
	parsed, err := planning.ParseDraft([]byte(rawJSON))
	if err != nil {
		return model.PlanningDraft{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.PlanningDraft{}, fmt.Errorf("begin planning draft: %w", err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.PlanningDraft{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.PlanningDraft{}, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM llm_pipelines WHERE pipeline_id = ?", pipelineID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.PlanningDraft{}, fmt.Errorf("%w: LLM pipeline %d", ErrNotFound, pipelineID)
		}
		return model.PlanningDraft{}, err
	}
	now := time.Now().UTC().Round(0)
	result, err := tx.ExecContext(ctx, `INSERT INTO planning_drafts (project_id, pipeline_id, status, summary, raw_json, created_at) VALUES (?, ?, 'Review', ?, ?, ?)`, projectID, pipelineID, parsed.Summary, rawJSON, now.Format(time.RFC3339Nano))
	if err != nil {
		return model.PlanningDraft{}, fmt.Errorf("insert planning draft: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.PlanningDraft{}, err
	}
	if err := recordProjectEvent(ctx, tx, projectID, "planning_draft", fmt.Sprintf("PD-%d", id), "planning_draft_created", "Local LLM planning draft created for review.", now); err != nil {
		return model.PlanningDraft{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.PlanningDraft{}, fmt.Errorf("commit planning draft: %w", err)
	}
	return model.PlanningDraft{DraftID: id, ProjectID: projectID, PipelineID: pipelineID, Status: "Review", Summary: parsed.Summary, RawJSON: rawJSON, CreatedAt: now}, nil
}

func (s *Store) ListPlanningDrafts(ctx context.Context, projectID string) ([]model.PlanningDraft, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT draft_id, project_id, pipeline_id, status, summary, raw_json, created_at, applied_at FROM planning_drafts WHERE project_id = ? ORDER BY created_at DESC, draft_id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.PlanningDraft, 0)
	for rows.Next() {
		var item model.PlanningDraft
		var created string
		var applied sql.NullString
		if err := rows.Scan(&item.DraftID, &item.ProjectID, &item.PipelineID, &item.Status, &item.Summary, &item.RawJSON, &created, &applied); err != nil {
			return nil, err
		}
		var err error
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		if applied.Valid {
			at, err := time.Parse(time.RFC3339Nano, applied.String)
			if err != nil {
				return nil, err
			}
			item.AppliedAt = &at
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ApplyPlanningDraft materializes one reviewed proposal atomically and marks it Applied.
func (s *Store) ApplyPlanningDraft(ctx context.Context, projectID string, draftID int64) (model.ExecutionTree, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ExecutionTree{}, err
	}
	defer tx.Rollback()
	var status, rawJSON, projectAddress string
	if err := tx.QueryRowContext(ctx, `SELECT d.status,d.raw_json,p.item_address FROM planning_drafts d JOIN projects p ON p.project_id=d.project_id WHERE d.draft_id=? AND d.project_id=?`, draftID, projectID).Scan(&status, &rawJSON, &projectAddress); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ExecutionTree{}, fmt.Errorf("%w: planning draft %d", ErrNotFound, draftID)
		}
		return model.ExecutionTree{}, err
	}
	if status != "Review" {
		return model.ExecutionTree{}, errors.New("planning draft is not awaiting review")
	}
	draft, err := planning.ParseDraft([]byte(rawJSON))
	if err != nil {
		return model.ExecutionTree{}, fmt.Errorf("revalidate planning draft: %w", err)
	}
	now := time.Now().UTC().Round(0)
	stampNow := now.Format(time.RFC3339Nano)
	for _, category := range draft.Categories {
		categoryNumber, err := allocateItemNumber(ctx, tx)
		if err != nil {
			return model.ExecutionTree{}, err
		}
		categoryID := fmt.Sprintf("C-%d", categoryNumber)
		categoryAddress := projectAddress + "." + fmt.Sprintf("%d", categoryNumber)
		if _, err = tx.ExecContext(ctx, `INSERT INTO categories (category_id,project_id,item_address,category_name,category_description,category_goal,status,priority,progress_pct,created_at,updated_at) VALUES (?,?,?,?,'','','Open','Normal',0,?,?)`, categoryID, projectID, categoryAddress, category.Name, stampNow, stampNow); err != nil {
			return model.ExecutionTree{}, err
		}
		for _, task := range category.Tasks {
			taskNumber, err := allocateItemNumber(ctx, tx)
			if err != nil {
				return model.ExecutionTree{}, err
			}
			taskID := fmt.Sprintf("T-%d", taskNumber)
			taskAddress := categoryAddress + "." + fmt.Sprintf("%d", taskNumber)
			if _, err = tx.ExecContext(ctx, `INSERT INTO tasks (task_id,project_id,category_id,item_address,task_name,task_description,task_goal,status,priority,estimated_duration_seconds,reported_completion_pct,calculated_completion_pct,created_at,updated_at) VALUES (?,?,?,?,?,'','','Open','Normal',?,0,0,?,?)`, taskID, projectID, categoryID, taskAddress, task.Name, task.EstimatedDurationSeconds, stampNow, stampNow); err != nil {
				return model.ExecutionTree{}, err
			}
			for _, sprint := range task.Sprints {
				if err := insertDraftSprint(ctx, tx, projectID, categoryID, taskID, "", taskAddress, sprint, now); err != nil {
					return model.ExecutionTree{}, err
				}
			}
			for _, subtask := range task.Subtasks {
				subNumber, err := allocateItemNumber(ctx, tx)
				if err != nil {
					return model.ExecutionTree{}, err
				}
				subID := fmt.Sprintf("ST-%d", subNumber)
				subAddress := taskAddress + "." + fmt.Sprintf("%d", subNumber)
				if _, err = tx.ExecContext(ctx, `INSERT INTO subtasks (subtask_id,project_id,category_id,task_id,item_address,subtask_name,subtask_description,subtask_goal,status,priority,estimated_duration_seconds,reported_completion_pct,calculated_completion_pct,created_at,updated_at) VALUES (?,?,?,?,?,?,'','','Open','Normal',?,0,0,?,?)`, subID, projectID, categoryID, taskID, subAddress, subtask.Name, subtask.EstimatedDurationSeconds, stampNow, stampNow); err != nil {
					return model.ExecutionTree{}, err
				}
				for _, sprint := range subtask.Sprints {
					if err := insertDraftSprint(ctx, tx, projectID, categoryID, taskID, subID, subAddress, sprint, now); err != nil {
						return model.ExecutionTree{}, err
					}
				}
			}
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE planning_drafts SET status='Applied', applied_at=? WHERE draft_id=?", stampNow, draftID); err != nil {
		return model.ExecutionTree{}, err
	}
	if err := recordProjectEvent(ctx, tx, projectID, "planning_draft", fmt.Sprintf("PD-%d", draftID), "planning_draft_applied", "Approved local LLM planning draft materialized hierarchy.", now); err != nil {
		return model.ExecutionTree{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.ExecutionTree{}, err
	}
	return s.ProjectExecutionTree(ctx, projectID)
}
func insertDraftSprint(ctx context.Context, tx *sql.Tx, projectID, categoryID, taskID, subtaskID, ownerAddress string, sprint planning.DraftSprint, now time.Time) error {
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("SP-%d", number)
	buffer := sprint.EstimatedDurationSeconds * int64(sprint.BufferPct) / 100
	_, err = tx.ExecContext(ctx, `INSERT INTO sprints (sprint_id,project_id,category_id,task_id,subtask_id,item_address,sprint_name,sprint_description,sprint_goal,status,priority,estimated_duration_seconds,buffer_pct,buffer_duration_seconds,active_duration_seconds,hold_duration_seconds,started_at,ended_at,active_started_at,hold_started_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,'','','Open','Normal',?,?,?,0,0,NULL,NULL,NULL,NULL,?,?)`, id, projectID, categoryID, taskID, nullString(subtaskID), ownerAddress+"."+fmt.Sprintf("%d", number), sprint.Name, sprint.EstimatedDurationSeconds, sprint.BufferPct, buffer, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return err
}
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// UpdateProjectStatus persists a deliberate workflow state for a Project.
func (s *Store) UpdateProjectStatus(ctx context.Context, projectID string, input model.UpdateProjectStatusInput) (model.Project, error) {
	status := strings.TrimSpace(input.Status)
	if status != "Open" && status != "On Hold" && status != "Completed" && status != "Cancelled" {
		return model.Project{}, errors.New("project status must be Open, On Hold, Completed, or Cancelled")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Project{}, fmt.Errorf("begin project status update: %w", err)
	}
	defer tx.Rollback()
	var previous string
	if err := tx.QueryRowContext(ctx, "SELECT status FROM projects WHERE project_id = ?", projectID).Scan(&previous); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Project{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.Project{}, fmt.Errorf("read project status: %w", err)
	}
	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, "UPDATE projects SET status = ?, updated_at = ? WHERE project_id = ?", status, now.Format(time.RFC3339Nano), projectID); err != nil {
		return model.Project{}, fmt.Errorf("update project status: %w", err)
	}
	if previous != status {
		message := fmt.Sprintf("Project status changed from %s to %s.", previous, status)
		if err := recordProjectEvent(ctx, tx, projectID, "project", projectID, "project_status_changed", message, now); err != nil {
			return model.Project{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Project{}, fmt.Errorf("commit project status update: %w", err)
	}
	return scanProject(s.db.QueryRowContext(ctx, `SELECT project_id, project_number, item_address, project_name,
		project_description, project_goal, status, priority, palette_id,
		reported_completion_pct, calculated_completion_pct, created_at, updated_at
		FROM projects WHERE project_id = ?`, projectID))
}

// UpdateProjectMetadata persists editable durable Project context.
func (s *Store) UpdateProjectMetadata(ctx context.Context, projectID string, input model.UpdateProjectMetadataInput) (model.Project, error) {
	goal := strings.TrimSpace(input.Goal)
	description := strings.TrimSpace(input.Description)
	if len(goal) > 1000 || len(description) > 10000 {
		return model.Project{}, errors.New("project goal must be at most 1000 characters and description at most 10000 characters")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Project{}, fmt.Errorf("begin project metadata update: %w", err)
	}
	defer tx.Rollback()
	var previousGoal, previousDescription string
	if err := tx.QueryRowContext(ctx, "SELECT project_goal, project_description FROM projects WHERE project_id = ?", projectID).Scan(&previousGoal, &previousDescription); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Project{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.Project{}, fmt.Errorf("read project metadata: %w", err)
	}
	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, "UPDATE projects SET project_goal = ?, project_description = ?, updated_at = ? WHERE project_id = ?", goal, description, now.Format(time.RFC3339Nano), projectID); err != nil {
		return model.Project{}, fmt.Errorf("update project metadata: %w", err)
	}
	if previousGoal != goal || previousDescription != description {
		if err := recordProjectEvent(ctx, tx, projectID, "project", projectID, "project_metadata_updated", "Project goal or description updated.", now); err != nil {
			return model.Project{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Project{}, fmt.Errorf("commit project metadata update: %w", err)
	}
	return scanProject(s.db.QueryRowContext(ctx, `SELECT project_id, project_number, item_address, project_name,
		project_description, project_goal, status, priority, palette_id,
		reported_completion_pct, calculated_completion_pct, created_at, updated_at
		FROM projects WHERE project_id = ?`, projectID))
}

// ProjectExecutionTree returns a consistent ownership projection for one Project.
func (s *Store) ProjectExecutionTree(ctx context.Context, projectID string) (model.ExecutionTree, error) {
	project, err := scanProject(s.db.QueryRowContext(ctx, `SELECT project_id, project_number, item_address, project_name,
		project_description, project_goal, status, priority, palette_id,
		reported_completion_pct, calculated_completion_pct, created_at, updated_at
		FROM projects WHERE project_id = ?`, projectID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ExecutionTree{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.ExecutionTree{}, fmt.Errorf("read execution tree project: %w", err)
	}
	categories, err := s.ListCategories(ctx, projectID)
	if err != nil {
		return model.ExecutionTree{}, err
	}
	tree := model.ExecutionTree{Project: project, Categories: make([]model.ExecutionCategory, 0)}
	categoryNodes := make(map[string]*model.ExecutionCategory, len(categories))
	for _, category := range categories {
		categoryNodes[category.CategoryID] = &model.ExecutionCategory{Category: category, Categories: make([]model.ExecutionCategory, 0), Tasks: make([]model.ExecutionTask, 0)}
	}
	tasks, err := s.ListTasks(ctx, projectID)
	if err != nil {
		return model.ExecutionTree{}, err
	}
	for _, task := range tasks {
		categoryNode, ok := categoryNodes[task.CategoryID]
		if !ok {
			return model.ExecutionTree{}, fmt.Errorf("execution tree task %s references a missing category", task.TaskID)
		}
		directSprints, err := s.ListSprints(ctx, task.TaskID)
		if err != nil {
			return model.ExecutionTree{}, err
		}
		subtasks, err := s.ListSubtasks(ctx, task.TaskID)
		if err != nil {
			return model.ExecutionTree{}, err
		}
		taskNode := model.ExecutionTask{Task: task, Sprints: directSprints, Subtasks: make([]model.ExecutionSubtask, 0, len(subtasks))}
		for _, subtask := range subtasks {
			sprints, err := s.ListSubtaskSprints(ctx, subtask.SubtaskID)
			if err != nil {
				return model.ExecutionTree{}, err
			}
			taskNode.Subtasks = append(taskNode.Subtasks, model.ExecutionSubtask{Subtask: subtask, Sprints: sprints})
		}
		categoryNode.Tasks = append(categoryNode.Tasks, taskNode)
	}
	var buildCategory func(string) model.ExecutionCategory
	buildCategory = func(categoryID string) model.ExecutionCategory {
		node := *categoryNodes[categoryID]
		node.Categories = make([]model.ExecutionCategory, 0)
		for _, child := range categories {
			if child.ParentCategoryID == categoryID {
				node.Categories = append(node.Categories, buildCategory(child.CategoryID))
			}
		}
		return node
	}
	for _, category := range categories {
		if category.ParentCategoryID == "" {
			tree.Categories = append(tree.Categories, buildCategory(category.CategoryID))
		} else if _, ok := categoryNodes[category.ParentCategoryID]; !ok {
			return model.ExecutionTree{}, fmt.Errorf("execution tree category %s references a missing parent", category.CategoryID)
		}
	}
	model.CalculateExecutionTreeCompletion(&tree)
	return tree, nil
}

// ListProjects returns projects in most-recent activity order.
func (s *Store) ListProjects(ctx context.Context) ([]model.Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT project_id, project_number, item_address, project_name,
		project_description, project_goal, status, priority, palette_id,
		reported_completion_pct, calculated_completion_pct, created_at, updated_at
		FROM projects ORDER BY updated_at DESC, project_number DESC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	projects := make([]model.Project, 0)
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		tree, err := s.ProjectExecutionTree(ctx, project.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("calculate project completion %s: %w", project.ProjectID, err)
		}
		project.CalculatedCompletionPct = tree.Project.CalculatedCompletionPct
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

func scanProject(scanner interface{ Scan(...any) error }) (model.Project, error) {
	var project model.Project
	var createdAt, updatedAt string
	if err := scanner.Scan(&project.ProjectID, &project.ProjectNumber, &project.ItemAddress, &project.ProjectName,
		&project.ProjectDescription, &project.ProjectGoal, &project.Status, &project.Priority, &project.PaletteID,
		&project.ReportedCompletionPct, &project.CalculatedCompletionPct, &createdAt, &updatedAt); err != nil {
		return model.Project{}, fmt.Errorf("scan project: %w", err)
	}
	var err error
	project.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.Project{}, fmt.Errorf("parse project created_at: %w", err)
	}
	project.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return model.Project{}, fmt.Errorf("parse project updated_at: %w", err)
	}
	return project, nil
}

// ProjectOperationalSummary returns durable, currently known Sprint state without inventing a runtime pulse.
func (s *Store) ProjectOperationalSummary(ctx context.Context, projectID string) (model.ProjectOperationalSummary, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ProjectOperationalSummary{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.ProjectOperationalSummary{}, fmt.Errorf("read summary project: %w", err)
	}
	summary := model.ProjectOperationalSummary{ProjectID: projectID}
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN s.status = 'Open' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN s.status = 'Active' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN s.status = 'On Hold' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN s.status = 'Completed' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(s.estimated_duration_seconds), 0),
		COALESCE(SUM(s.buffer_duration_seconds), 0),
		COALESCE(SUM(COALESCE(x.extension_duration_seconds, 0)), 0),
		COALESCE(SUM(s.estimated_duration_seconds + s.buffer_duration_seconds + COALESCE(x.extension_duration_seconds, 0)), 0),
		COALESCE(SUM(s.active_duration_seconds), 0),
		COALESCE(SUM(s.hold_duration_seconds), 0)
		FROM sprints s LEFT JOIN (SELECT sprint_id, SUM(duration_seconds) AS extension_duration_seconds FROM sprint_time_extensions GROUP BY sprint_id) x ON x.sprint_id = s.sprint_id WHERE s.project_id = ?`, projectID).Scan(
		&summary.TotalSprints, &summary.OpenSprints, &summary.ActiveSprints, &summary.HeldSprints,
		&summary.CompletedSprints, &summary.EstimatedDurationSeconds, &summary.BufferDurationSeconds, &summary.ExtensionDurationSeconds,
		&summary.PlannedDurationSeconds, &summary.RecordedWorkSeconds, &summary.RecordedHoldSeconds)
	if err != nil {
		return model.ProjectOperationalSummary{}, fmt.Errorf("read project operational summary: %w", err)
	}
	return summary, nil
}

// ListProjectEvents returns newest-first immutable execution history for one project.
func (s *Store) ListProjectEvents(ctx context.Context, projectID string) ([]model.ProjectEvent, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return nil, fmt.Errorf("read event project: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id, project_id, entity_type, entity_id, event_type, message, created_at
		FROM project_events WHERE project_id = ? ORDER BY created_at DESC, event_id DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project events: %w", err)
	}
	defer rows.Close()
	events := make([]model.ProjectEvent, 0)
	for rows.Next() {
		var event model.ProjectEvent
		var createdAt string
		if err := rows.Scan(&event.EventID, &event.ProjectID, &event.EntityType, &event.EntityID, &event.EventType, &event.Message, &createdAt); err != nil {
			return nil, fmt.Errorf("scan project event: %w", err)
		}
		if event.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse project event created_at: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project events: %w", err)
	}
	return events, nil
}

func recordProjectEvent(ctx context.Context, tx *sql.Tx, projectID, entityType, entityID, eventType, message string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO project_events (project_id, entity_type, entity_id, event_type, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, projectID, entityType, entityID, eventType, message, stamp(now))
	return err
}

// CreateProjectNote persists an attributed, immutable observation on one project.
func (s *Store) CreateProjectNote(ctx context.Context, projectID string, input model.CreateProjectNoteInput, actorID string) (model.ProjectNote, error) {
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" {
		return model.ProjectNote{}, errors.New("note content is required")
	}
	if len(input.Content) > 10000 {
		return model.ProjectNote{}, errors.New("note content must be at most 10000 characters")
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ProjectNote{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.ProjectNote{}, fmt.Errorf("read note project: %w", err)
	}
	note := model.ProjectNote{
		ProjectID: projectID,
		Content:   input.Content,
		ActorID:   strings.TrimSpace(actorID),
		CreatedAt: time.Now().UTC().Round(0),
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO project_notes (project_id, content, actor_id, created_at) VALUES (?, ?, ?, ?)`,
		note.ProjectID, note.Content, note.ActorID, note.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return model.ProjectNote{}, fmt.Errorf("insert project note: %w", err)
	}
	note.NoteID, err = result.LastInsertId()
	if err != nil {
		return model.ProjectNote{}, fmt.Errorf("read project note id: %w", err)
	}
	return note, nil
}

// ListProjectNotes returns newest-first project observations.
func (s *Store) ListProjectNotes(ctx context.Context, projectID string) ([]model.ProjectNote, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return nil, fmt.Errorf("read note project: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT note_id, project_id, content, actor_id, created_at
		FROM project_notes WHERE project_id = ? ORDER BY created_at DESC, note_id DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project notes: %w", err)
	}
	defer rows.Close()
	notes := make([]model.ProjectNote, 0)
	for rows.Next() {
		note, err := scanProjectNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project notes: %w", err)
	}
	return notes, nil
}

func scanProjectNote(scanner interface{ Scan(...any) error }) (model.ProjectNote, error) {
	var note model.ProjectNote
	var createdAt string
	if err := scanner.Scan(&note.NoteID, &note.ProjectID, &note.Content, &note.ActorID, &createdAt); err != nil {
		return model.ProjectNote{}, fmt.Errorf("scan project note: %w", err)
	}
	var err error
	note.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.ProjectNote{}, fmt.Errorf("parse project note created_at: %w", err)
	}
	return note, nil
}

// ErrNotFound identifies a requested tracked entity that does not exist.
var ErrNotFound = errors.New("tracked item not found")

// CreateCategory persists a project-owned category with a global ID and scoped address.
func (s *Store) CreateCategory(ctx context.Context, projectID string, input model.CreateCategoryInput) (model.Category, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.ParentCategoryID = strings.TrimSpace(input.ParentCategoryID)
	if input.Name == "" {
		return model.Category{}, errors.New("category name is required")
	}
	if input.Priority == "" {
		input.Priority = "Normal"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Category{}, fmt.Errorf("begin category creation: %w", err)
	}
	defer tx.Rollback()
	var projectAddress string
	if err := tx.QueryRowContext(ctx, "SELECT item_address FROM projects WHERE project_id = ?", projectID).Scan(&projectAddress); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Category{}, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return model.Category{}, fmt.Errorf("read category project: %w", err)
	}
	categoryAddress := projectAddress
	if input.ParentCategoryID != "" {
		var parentProjectID, parentAddress string
		if err := tx.QueryRowContext(ctx, "SELECT project_id, item_address FROM categories WHERE category_id = ?", input.ParentCategoryID).Scan(&parentProjectID, &parentAddress); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.Category{}, fmt.Errorf("%w: parent category %s", ErrNotFound, input.ParentCategoryID)
			}
			return model.Category{}, fmt.Errorf("read category parent: %w", err)
		}
		if parentProjectID != projectID {
			return model.Category{}, errors.New("parent category must belong to the same project")
		}
		categoryAddress = parentAddress
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.Category{}, err
	}
	now := time.Now().UTC().Round(0)
	category := model.Category{
		CategoryID:       fmt.Sprintf("C-%d", number),
		ProjectID:        projectID,
		ParentCategoryID: input.ParentCategoryID,
		ItemAddress:      categoryAddress + "." + fmt.Sprintf("%d", number),
		Name:             input.Name,
		Description:      strings.TrimSpace(input.Description),
		Goal:             strings.TrimSpace(input.Goal),
		Status:           "Open",
		Priority:         input.Priority,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	parentCategoryID := any(nil)
	if category.ParentCategoryID != "" {
		parentCategoryID = category.ParentCategoryID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO categories (
		category_id, project_id, parent_category_id, item_address, category_name, category_description,
		category_goal, status, priority, progress_pct, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		category.CategoryID, category.ProjectID, parentCategoryID, category.ItemAddress, category.Name, category.Description,
		category.Goal, category.Status, category.Priority, category.ProgressPct,
		category.CreatedAt.Format(time.RFC3339Nano), category.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return model.Category{}, fmt.Errorf("insert category: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.Category{}, fmt.Errorf("commit category creation: %w", err)
	}
	return category, nil
}

// UpdateCategoryMetadata persists editable durable Category context.
func (s *Store) UpdateCategoryMetadata(ctx context.Context, categoryID string, input model.UpdateCategoryMetadataInput) (model.Category, error) {
	goal := strings.TrimSpace(input.Goal)
	description := strings.TrimSpace(input.Description)
	if len(goal) > 1000 || len(description) > 10000 {
		return model.Category{}, errors.New("category goal must be at most 1000 characters and description at most 10000 characters")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Category{}, err
	}
	defer tx.Rollback()
	var projectID, previousGoal, previousDescription string
	if err := tx.QueryRowContext(ctx, "SELECT project_id, category_goal, category_description FROM categories WHERE category_id = ?", categoryID).Scan(&projectID, &previousGoal, &previousDescription); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Category{}, fmt.Errorf("%w: category %s", ErrNotFound, categoryID)
		}
		return model.Category{}, err
	}
	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, "UPDATE categories SET category_goal = ?, category_description = ?, updated_at = ? WHERE category_id = ?", goal, description, now.Format(time.RFC3339Nano), categoryID); err != nil {
		return model.Category{}, err
	}
	if previousGoal != goal || previousDescription != description {
		if err := recordProjectEvent(ctx, tx, projectID, "category", categoryID, "category_metadata_updated", "Category goal or description updated.", now); err != nil {
			return model.Category{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Category{}, err
	}
	return scanCategory(s.db.QueryRowContext(ctx, `SELECT category_id, project_id, parent_category_id, item_address, category_name, category_description, category_goal, status, priority, progress_pct, created_at, updated_at FROM categories WHERE category_id = ?`, categoryID))
}

// UpdateCategoryStatus persists a Category workflow state and immutable Project history.
func (s *Store) UpdateCategoryStatus(ctx context.Context, categoryID string, input model.UpdateCategoryStatusInput) (model.Category, error) {
	status := strings.TrimSpace(input.Status)
	if status != "Open" && status != "On Hold" && status != "Completed" && status != "Cancelled" {
		return model.Category{}, errors.New("category status must be Open, On Hold, Completed, or Cancelled")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Category{}, err
	}
	defer tx.Rollback()
	var projectID, previous string
	if err := tx.QueryRowContext(ctx, "SELECT project_id, status FROM categories WHERE category_id = ?", categoryID).Scan(&projectID, &previous); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Category{}, fmt.Errorf("%w: category %s", ErrNotFound, categoryID)
		}
		return model.Category{}, err
	}
	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, "UPDATE categories SET status = ?, updated_at = ? WHERE category_id = ?", status, now.Format(time.RFC3339Nano), categoryID); err != nil {
		return model.Category{}, err
	}
	if previous != status {
		if err := recordProjectEvent(ctx, tx, projectID, "category", categoryID, "category_status_changed", fmt.Sprintf("Category status changed from %s to %s.", previous, status), now); err != nil {
			return model.Category{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Category{}, err
	}
	return scanCategory(s.db.QueryRowContext(ctx, `SELECT category_id, project_id, parent_category_id, item_address, category_name, category_description, category_goal, status, priority, progress_pct, created_at, updated_at FROM categories WHERE category_id = ?`, categoryID))
}

// ListCategories returns a project's categories in creation order.
func (s *Store) ListCategories(ctx context.Context, projectID string) ([]model.Category, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return nil, fmt.Errorf("read category project: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT category_id, project_id, parent_category_id, item_address, category_name,
		category_description, category_goal, status, priority, progress_pct, created_at, updated_at
		FROM categories WHERE project_id = ? ORDER BY created_at, category_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	categories := make([]model.Category, 0)
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}
	return categories, nil
}

// CreateTask persists a category-owned task with a global ID and hierarchical address.
func (s *Store) CreateTask(ctx context.Context, projectID string, input model.CreateTaskInput) (model.Task, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return model.Task{}, errors.New("task name is required")
	}
	if input.CategoryID == "" {
		return model.Task{}, errors.New("category_id is required")
	}
	if input.EstimatedDurationSeconds < 0 {
		return model.Task{}, errors.New("estimated_duration_seconds must not be negative")
	}
	if input.Priority == "" {
		input.Priority = "Normal"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Task{}, fmt.Errorf("begin task creation: %w", err)
	}
	defer tx.Rollback()
	var categoryAddress string
	if err := tx.QueryRowContext(ctx, "SELECT item_address FROM categories WHERE category_id = ? AND project_id = ?", input.CategoryID, projectID).Scan(&categoryAddress); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Task{}, fmt.Errorf("%w: category %s", ErrNotFound, input.CategoryID)
		}
		return model.Task{}, fmt.Errorf("read task category: %w", err)
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.Task{}, err
	}
	now := time.Now().UTC().Round(0)
	task := model.Task{
		TaskID:                   fmt.Sprintf("T-%d", number),
		ProjectID:                projectID,
		CategoryID:               input.CategoryID,
		ItemAddress:              categoryAddress + "." + fmt.Sprintf("%d", number),
		Name:                     input.Name,
		Description:              strings.TrimSpace(input.Description),
		Goal:                     strings.TrimSpace(input.Goal),
		Status:                   "Open",
		Priority:                 input.Priority,
		EstimatedDurationSeconds: input.EstimatedDurationSeconds,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO tasks (
		task_id, project_id, category_id, item_address, task_name, task_description, task_goal,
		status, priority, estimated_duration_seconds, reported_completion_pct,
		calculated_completion_pct, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.TaskID, task.ProjectID, task.CategoryID, task.ItemAddress, task.Name, task.Description,
		task.Goal, task.Status, task.Priority, task.EstimatedDurationSeconds, task.ReportedCompletionPct,
		task.CalculatedCompletionPct, task.CreatedAt.Format(time.RFC3339Nano), task.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return model.Task{}, fmt.Errorf("insert task: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.Task{}, fmt.Errorf("commit task creation: %w", err)
	}
	return task, nil
}

// ListTasks returns a project's tasks in creation order.
func (s *Store) ListTasks(ctx context.Context, projectID string) ([]model.Task, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		return nil, fmt.Errorf("read task project: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT task_id, project_id, category_id, item_address, task_name,
		task_description, task_goal, status, priority, estimated_duration_seconds,
		reported_completion_pct, calculated_completion_pct, created_at, updated_at
		FROM tasks WHERE project_id = ? ORDER BY created_at, task_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	tasks := make([]model.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
}

// UpdateTaskMetadata persists editable durable Task context.
func (s *Store) UpdateTaskMetadata(ctx context.Context, taskID string, input model.UpdateTaskMetadataInput) (model.Task, error) {
	goal := strings.TrimSpace(input.Goal)
	description := strings.TrimSpace(input.Description)
	if len(goal) > 1000 || len(description) > 10000 {
		return model.Task{}, errors.New("task goal must be at most 1000 characters and description at most 10000 characters")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Task{}, err
	}
	defer tx.Rollback()
	var projectID, previousGoal, previousDescription string
	if err := tx.QueryRowContext(ctx, "SELECT project_id, task_goal, task_description FROM tasks WHERE task_id = ?", taskID).Scan(&projectID, &previousGoal, &previousDescription); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Task{}, fmt.Errorf("%w: task %s", ErrNotFound, taskID)
		}
		return model.Task{}, err
	}
	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, "UPDATE tasks SET task_goal = ?, task_description = ?, updated_at = ? WHERE task_id = ?", goal, description, now.Format(time.RFC3339Nano), taskID); err != nil {
		return model.Task{}, err
	}
	if previousGoal != goal || previousDescription != description {
		if err := recordProjectEvent(ctx, tx, projectID, "task", taskID, "task_metadata_updated", "Task goal or description updated.", now); err != nil {
			return model.Task{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Task{}, err
	}
	return scanTask(s.db.QueryRowContext(ctx, `SELECT task_id, project_id, category_id, item_address, task_name, task_description, task_goal, status, priority, estimated_duration_seconds, reported_completion_pct, calculated_completion_pct, created_at, updated_at FROM tasks WHERE task_id = ?`, taskID))
}

// UpdateTaskStatus persists a Task workflow state and immutable Project history.
func (s *Store) UpdateTaskStatus(ctx context.Context, taskID string, input model.UpdateTaskStatusInput) (model.Task, error) {
	status := strings.TrimSpace(input.Status)
	if status != "Open" && status != "On Hold" && status != "Completed" && status != "Cancelled" {
		return model.Task{}, errors.New("task status must be Open, On Hold, Completed, or Cancelled")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Task{}, err
	}
	defer tx.Rollback()
	var projectID, previous string
	if err := tx.QueryRowContext(ctx, "SELECT project_id, status FROM tasks WHERE task_id = ?", taskID).Scan(&projectID, &previous); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Task{}, fmt.Errorf("%w: task %s", ErrNotFound, taskID)
		}
		return model.Task{}, err
	}
	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, "UPDATE tasks SET status = ?, updated_at = ? WHERE task_id = ?", status, now.Format(time.RFC3339Nano), taskID); err != nil {
		return model.Task{}, err
	}
	if previous != status {
		if err := recordProjectEvent(ctx, tx, projectID, "task", taskID, "task_status_changed", fmt.Sprintf("Task status changed from %s to %s.", previous, status), now); err != nil {
			return model.Task{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Task{}, err
	}
	return scanTask(s.db.QueryRowContext(ctx, `SELECT task_id, project_id, category_id, item_address, task_name, task_description, task_goal, status, priority, estimated_duration_seconds, reported_completion_pct, calculated_completion_pct, created_at, updated_at FROM tasks WHERE task_id = ?`, taskID))
}

func scanTask(scanner interface{ Scan(...any) error }) (model.Task, error) {
	var task model.Task
	var createdAt, updatedAt string
	if err := scanner.Scan(&task.TaskID, &task.ProjectID, &task.CategoryID, &task.ItemAddress, &task.Name,
		&task.Description, &task.Goal, &task.Status, &task.Priority, &task.EstimatedDurationSeconds,
		&task.ReportedCompletionPct, &task.CalculatedCompletionPct, &createdAt, &updatedAt); err != nil {
		return model.Task{}, fmt.Errorf("scan task: %w", err)
	}
	var err error
	task.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.Task{}, fmt.Errorf("parse task created_at: %w", err)
	}
	task.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return model.Task{}, fmt.Errorf("parse task updated_at: %w", err)
	}
	return task, nil
}

// CreateSubtask persists a task-owned executable unit with a global ID and hierarchical address.
func (s *Store) CreateSubtask(ctx context.Context, taskID string, input model.CreateSubtaskInput) (model.Subtask, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return model.Subtask{}, errors.New("subtask name is required")
	}
	if input.EstimatedDurationSeconds < 0 {
		return model.Subtask{}, errors.New("estimated_duration_seconds must not be negative")
	}
	if input.Priority == "" {
		input.Priority = "Normal"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Subtask{}, fmt.Errorf("begin subtask creation: %w", err)
	}
	defer tx.Rollback()
	var projectID, categoryID, taskAddress string
	if err := tx.QueryRowContext(ctx, "SELECT project_id, category_id, item_address FROM tasks WHERE task_id = ?", taskID).Scan(&projectID, &categoryID, &taskAddress); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Subtask{}, fmt.Errorf("%w: task %s", ErrNotFound, taskID)
		}
		return model.Subtask{}, fmt.Errorf("read subtask task: %w", err)
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.Subtask{}, err
	}
	now := time.Now().UTC().Round(0)
	subtask := model.Subtask{
		SubtaskID:                fmt.Sprintf("ST-%d", number),
		ProjectID:                projectID,
		CategoryID:               categoryID,
		TaskID:                   taskID,
		ItemAddress:              taskAddress + "." + fmt.Sprintf("%d", number),
		Name:                     input.Name,
		Description:              strings.TrimSpace(input.Description),
		Goal:                     strings.TrimSpace(input.Goal),
		Status:                   "Open",
		Priority:                 input.Priority,
		EstimatedDurationSeconds: input.EstimatedDurationSeconds,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO subtasks (
		subtask_id, project_id, category_id, task_id, item_address, subtask_name, subtask_description,
		subtask_goal, status, priority, estimated_duration_seconds, reported_completion_pct,
		calculated_completion_pct, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		subtask.SubtaskID, subtask.ProjectID, subtask.CategoryID, subtask.TaskID, subtask.ItemAddress,
		subtask.Name, subtask.Description, subtask.Goal, subtask.Status, subtask.Priority,
		subtask.EstimatedDurationSeconds, subtask.ReportedCompletionPct, subtask.CalculatedCompletionPct,
		subtask.CreatedAt.Format(time.RFC3339Nano), subtask.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return model.Subtask{}, fmt.Errorf("insert subtask: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.Subtask{}, fmt.Errorf("commit subtask creation: %w", err)
	}
	return subtask, nil
}

// ListSubtasks returns a task's subtasks in creation order.
func (s *Store) ListSubtasks(ctx context.Context, taskID string) ([]model.Subtask, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM tasks WHERE task_id = ?", taskID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: task %s", ErrNotFound, taskID)
		}
		return nil, fmt.Errorf("read subtask task: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT subtask_id, project_id, category_id, task_id, item_address,
		subtask_name, subtask_description, subtask_goal, status, priority, estimated_duration_seconds,
		reported_completion_pct, calculated_completion_pct, created_at, updated_at
		FROM subtasks WHERE task_id = ? ORDER BY created_at, subtask_id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list subtasks: %w", err)
	}
	defer rows.Close()
	subtasks := make([]model.Subtask, 0)
	for rows.Next() {
		subtask, err := scanSubtask(rows)
		if err != nil {
			return nil, err
		}
		subtasks = append(subtasks, subtask)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subtasks: %w", err)
	}
	return subtasks, nil
}

// UpdateSubtaskStatus persists a Subtask workflow state and immutable Project history.
func (s *Store) UpdateSubtaskStatus(ctx context.Context, subtaskID string, input model.UpdateSubtaskStatusInput) (model.Subtask, error) {
	status := strings.TrimSpace(input.Status)
	if status != "Open" && status != "On Hold" && status != "Completed" && status != "Cancelled" {
		return model.Subtask{}, errors.New("subtask status must be Open, On Hold, Completed, or Cancelled")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Subtask{}, err
	}
	defer tx.Rollback()
	var projectID, previous string
	if err := tx.QueryRowContext(ctx, "SELECT project_id, status FROM subtasks WHERE subtask_id = ?", subtaskID).Scan(&projectID, &previous); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Subtask{}, fmt.Errorf("%w: subtask %s", ErrNotFound, subtaskID)
		}
		return model.Subtask{}, err
	}
	now := time.Now().UTC().Round(0)
	if _, err := tx.ExecContext(ctx, "UPDATE subtasks SET status = ?, updated_at = ? WHERE subtask_id = ?", status, now.Format(time.RFC3339Nano), subtaskID); err != nil {
		return model.Subtask{}, err
	}
	if previous != status {
		if err := recordProjectEvent(ctx, tx, projectID, "subtask", subtaskID, "subtask_status_changed", fmt.Sprintf("Subtask status changed from %s to %s.", previous, status), now); err != nil {
			return model.Subtask{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Subtask{}, err
	}
	return scanSubtask(s.db.QueryRowContext(ctx, `SELECT subtask_id, project_id, category_id, task_id, item_address, subtask_name, subtask_description, subtask_goal, status, priority, estimated_duration_seconds, reported_completion_pct, calculated_completion_pct, created_at, updated_at FROM subtasks WHERE subtask_id = ?`, subtaskID))
}

func scanSubtask(scanner interface{ Scan(...any) error }) (model.Subtask, error) {
	var subtask model.Subtask
	var createdAt, updatedAt string
	if err := scanner.Scan(&subtask.SubtaskID, &subtask.ProjectID, &subtask.CategoryID, &subtask.TaskID, &subtask.ItemAddress,
		&subtask.Name, &subtask.Description, &subtask.Goal, &subtask.Status, &subtask.Priority,
		&subtask.EstimatedDurationSeconds, &subtask.ReportedCompletionPct, &subtask.CalculatedCompletionPct,
		&createdAt, &updatedAt); err != nil {
		return model.Subtask{}, fmt.Errorf("scan subtask: %w", err)
	}
	var err error
	subtask.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.Subtask{}, fmt.Errorf("parse subtask created_at: %w", err)
	}
	subtask.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return model.Subtask{}, fmt.Errorf("parse subtask updated_at: %w", err)
	}
	return subtask, nil
}

// CreateSprint persists a bounded execution cut directly under a task.
func (s *Store) CreateSprint(ctx context.Context, taskID string, input model.CreateSprintInput) (model.Sprint, error) {
	return s.createSprint(ctx, taskID, "", input)
}

// CreateSubtaskSprint persists a bounded execution cut owned by a subtask.
func (s *Store) CreateSubtaskSprint(ctx context.Context, subtaskID string, input model.CreateSprintInput) (model.Sprint, error) {
	return s.createSprint(ctx, "", subtaskID, input)
}

func (s *Store) createSprint(ctx context.Context, taskID, subtaskID string, input model.CreateSprintInput) (model.Sprint, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return model.Sprint{}, errors.New("sprint name is required")
	}
	if input.EstimatedDurationSeconds <= 0 {
		return model.Sprint{}, errors.New("estimated_duration_seconds must be positive")
	}
	if input.BufferPct < 0 || input.BufferPct > 100 || input.BufferPct != math.Trunc(input.BufferPct) {
		return model.Sprint{}, errors.New("buffer_pct must be a whole number between 0 and 100")
	}
	if input.Priority == "" {
		input.Priority = "Normal"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Sprint{}, fmt.Errorf("begin sprint creation: %w", err)
	}
	defer tx.Rollback()
	var projectID, categoryID, ownerAddress string
	if subtaskID != "" {
		if err := tx.QueryRowContext(ctx, "SELECT project_id, category_id, task_id, item_address FROM subtasks WHERE subtask_id = ?", subtaskID).Scan(&projectID, &categoryID, &taskID, &ownerAddress); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return model.Sprint{}, fmt.Errorf("%w: subtask %s", ErrNotFound, subtaskID)
			}
			return model.Sprint{}, fmt.Errorf("read sprint subtask: %w", err)
		}
	} else if err := tx.QueryRowContext(ctx, "SELECT project_id, category_id, item_address FROM tasks WHERE task_id = ?", taskID).Scan(&projectID, &categoryID, &ownerAddress); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Sprint{}, fmt.Errorf("%w: task %s", ErrNotFound, taskID)
		}
		return model.Sprint{}, fmt.Errorf("read sprint task: %w", err)
	}
	number, err := allocateItemNumber(ctx, tx)
	if err != nil {
		return model.Sprint{}, err
	}
	now := time.Now().UTC().Round(0)
	sprint := model.Sprint{
		SprintID:                 fmt.Sprintf("SP-%d", number),
		ProjectID:                projectID,
		CategoryID:               categoryID,
		TaskID:                   taskID,
		SubtaskID:                subtaskID,
		ItemAddress:              ownerAddress + "." + fmt.Sprintf("%d", number),
		Name:                     input.Name,
		Description:              strings.TrimSpace(input.Description),
		Goal:                     strings.TrimSpace(input.Goal),
		Status:                   "Open",
		Priority:                 input.Priority,
		EstimatedDurationSeconds: input.EstimatedDurationSeconds,
		BufferPct:                input.BufferPct,
		BufferDurationSeconds:    int64(float64(input.EstimatedDurationSeconds) * input.BufferPct / 100),
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	var storedSubtaskID any
	if sprint.SubtaskID != "" {
		storedSubtaskID = sprint.SubtaskID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sprints (
		sprint_id, project_id, category_id, task_id, subtask_id, item_address, sprint_name, sprint_description,
		sprint_goal, status, priority, estimated_duration_seconds, buffer_pct, buffer_duration_seconds,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sprint.SprintID, sprint.ProjectID, sprint.CategoryID, sprint.TaskID, storedSubtaskID, sprint.ItemAddress, sprint.Name,
		sprint.Description, sprint.Goal, sprint.Status, sprint.Priority, sprint.EstimatedDurationSeconds,
		sprint.BufferPct, sprint.BufferDurationSeconds, sprint.CreatedAt.Format(time.RFC3339Nano), sprint.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return model.Sprint{}, fmt.Errorf("insert sprint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.Sprint{}, fmt.Errorf("commit sprint creation: %w", err)
	}
	return sprint, nil
}

// ListSprints returns a task's direct execution cuts in creation order.
func (s *Store) ListSprints(ctx context.Context, taskID string) ([]model.Sprint, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM tasks WHERE task_id = ?", taskID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: task %s", ErrNotFound, taskID)
		}
		return nil, fmt.Errorf("read sprint task: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sprint_id, project_id, category_id, task_id, subtask_id, item_address,
		sprint_name, sprint_description, sprint_goal, status, priority, estimated_duration_seconds,
		buffer_pct, buffer_duration_seconds, active_duration_seconds, hold_duration_seconds,
		started_at, ended_at, created_at, updated_at FROM sprints WHERE task_id = ? AND subtask_id IS NULL ORDER BY created_at, sprint_id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list sprints: %w", err)
	}
	defer rows.Close()
	sprints := make([]model.Sprint, 0)
	for rows.Next() {
		sprint, err := scanSprint(rows)
		if err != nil {
			return nil, err
		}
		sprints = append(sprints, sprint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sprints: %w", err)
	}
	return sprints, nil
}

// ListSubtaskSprints returns a subtask's execution cuts in creation order.
func (s *Store) ListSubtaskSprints(ctx context.Context, subtaskID string) ([]model.Sprint, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM subtasks WHERE subtask_id = ?", subtaskID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: subtask %s", ErrNotFound, subtaskID)
		}
		return nil, fmt.Errorf("read sprint subtask: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sprint_id, project_id, category_id, task_id, subtask_id, item_address,
		sprint_name, sprint_description, sprint_goal, status, priority, estimated_duration_seconds,
		buffer_pct, buffer_duration_seconds, active_duration_seconds, hold_duration_seconds,
		started_at, ended_at, created_at, updated_at FROM sprints WHERE subtask_id = ? ORDER BY created_at, sprint_id`, subtaskID)
	if err != nil {
		return nil, fmt.Errorf("list subtask sprints: %w", err)
	}
	defer rows.Close()
	sprints := make([]model.Sprint, 0)
	for rows.Next() {
		sprint, err := scanSprint(rows)
		if err != nil {
			return nil, err
		}
		sprints = append(sprints, sprint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subtask sprints: %w", err)
	}
	return sprints, nil
}

// TransitionSprint performs one legal lifecycle transition and records completed intervals immutably.
func (s *Store) TransitionSprint(ctx context.Context, sprintID, action, reason string) (model.Sprint, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Sprint{}, fmt.Errorf("begin sprint transition: %w", err)
	}
	defer tx.Rollback()
	state, err := loadSprintState(ctx, tx, sprintID)
	if err != nil {
		return model.Sprint{}, err
	}
	now := time.Now().UTC().Round(0)
	reason = strings.TrimSpace(reason)

	rule, err := lifecycle.SprintTransition(state.Sprint.Status, action)
	if err != nil {
		return model.Sprint{}, err
	}

	switch rule.Action {
	case "start":
		_, err = tx.ExecContext(ctx, `UPDATE sprints SET status = ?, started_at = COALESCE(started_at, ?),
			active_started_at = ?, updated_at = ? WHERE sprint_id = ?`, rule.To, stamp(now), stamp(now), stamp(now), sprintID)
	case "hold":
		if state.ActiveStartedAt == nil {
			return model.Sprint{}, errors.New("active sprint is missing its active interval")
		}
		seconds := elapsedSeconds(*state.ActiveStartedAt, now)
		if err = addTimeEntry(ctx, tx, sprintID, rule.CloseInterval, *state.ActiveStartedAt, now, seconds, reason); err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE sprints SET status = ?, active_duration_seconds = active_duration_seconds + ?,
				active_started_at = NULL, hold_started_at = ?, updated_at = ? WHERE sprint_id = ?`, rule.To, seconds, stamp(now), stamp(now), sprintID)
		}
	case "resume":
		if state.HoldStartedAt == nil {
			return model.Sprint{}, errors.New("held sprint is missing its hold interval")
		}
		seconds := elapsedSeconds(*state.HoldStartedAt, now)
		if err = addTimeEntry(ctx, tx, sprintID, rule.CloseInterval, *state.HoldStartedAt, now, seconds, reason); err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE sprints SET status = ?, hold_duration_seconds = hold_duration_seconds + ?,
				hold_started_at = NULL, active_started_at = ?, updated_at = ? WHERE sprint_id = ?`, rule.To, seconds, stamp(now), stamp(now), sprintID)
		}
	case "complete":
		if state.ActiveStartedAt == nil {
			return model.Sprint{}, errors.New("active sprint is missing its active interval")
		}
		seconds := elapsedSeconds(*state.ActiveStartedAt, now)
		if err = addTimeEntry(ctx, tx, sprintID, rule.CloseInterval, *state.ActiveStartedAt, now, seconds, reason); err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE sprints SET status = ?, active_duration_seconds = active_duration_seconds + ?,
				active_started_at = NULL, ended_at = ?, updated_at = ? WHERE sprint_id = ?`, rule.To, seconds, stamp(now), stamp(now), sprintID)
		}
	default:
		return model.Sprint{}, fmt.Errorf("unsupported declared sprint transition %q", rule.Action)
	}
	if err != nil {
		return model.Sprint{}, fmt.Errorf("update sprint transition: %w", err)
	}
	updated, err := loadSprintState(ctx, tx, sprintID)
	if err != nil {
		return model.Sprint{}, err
	}
	eventType, message := sprintTransitionEvent(rule.Action)
	if err := recordProjectEvent(ctx, tx, updated.Sprint.ProjectID, "sprint", updated.Sprint.SprintID, eventType, message, now); err != nil {
		return model.Sprint{}, fmt.Errorf("record sprint transition event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.Sprint{}, fmt.Errorf("commit sprint transition: %w", err)
	}
	return updated.Sprint, nil
}

func sprintTransitionEvent(action string) (string, string) {
	switch action {
	case "start":
		return "sprint_started", "Sprint started."
	case "hold":
		return "sprint_held", "Sprint placed on hold."
	case "resume":
		return "sprint_resumed", "Sprint resumed."
	case "complete":
		return "sprint_completed", "Sprint completed."
	default:
		return "sprint_transitioned", "Sprint transitioned."
	}
}

// ListTimeEntries returns immutable completed work and hold intervals for a sprint.
// AddSprintTimeExtension records a justified immutable addition without changing an original estimate.
func (s *Store) AddSprintTimeExtension(ctx context.Context, sprintID string, input model.CreateSprintTimeExtensionInput) (model.SprintTimeExtension, error) {
	reason := strings.TrimSpace(input.Reason)
	if input.DurationSeconds <= 0 || input.DurationSeconds > int64(10*365*24*time.Hour/time.Second) || reason == "" || len(reason) > 10000 {
		return model.SprintTimeExtension{}, errors.New("extension requires a positive bounded duration and a reason of at most 10000 characters")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.SprintTimeExtension{}, err
	}
	defer tx.Rollback()
	var projectID string
	if err := tx.QueryRowContext(ctx, "SELECT project_id FROM sprints WHERE sprint_id = ?", sprintID).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.SprintTimeExtension{}, fmt.Errorf("%w: sprint %s", ErrNotFound, sprintID)
		}
		return model.SprintTimeExtension{}, err
	}
	now := time.Now().UTC().Round(0)
	result, err := tx.ExecContext(ctx, "INSERT INTO sprint_time_extensions (sprint_id, duration_seconds, reason, created_at) VALUES (?, ?, ?, ?)", sprintID, input.DurationSeconds, reason, now.Format(time.RFC3339Nano))
	if err != nil {
		return model.SprintTimeExtension{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.SprintTimeExtension{}, err
	}
	if err := recordProjectEvent(ctx, tx, projectID, "sprint", sprintID, "sprint_time_extended", fmt.Sprintf("Sprint extended by %d seconds: %s", input.DurationSeconds, reason), now); err != nil {
		return model.SprintTimeExtension{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.SprintTimeExtension{}, err
	}
	return model.SprintTimeExtension{ExtensionID: id, SprintID: sprintID, DurationSeconds: input.DurationSeconds, Reason: reason, CreatedAt: now}, nil
}

// ListSprintTimeExtensions returns immutable Sprint extension evidence in chronological order.
func (s *Store) ListSprintTimeExtensions(ctx context.Context, sprintID string) ([]model.SprintTimeExtension, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM sprints WHERE sprint_id = ?", sprintID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: sprint %s", ErrNotFound, sprintID)
		}
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT extension_id, sprint_id, duration_seconds, reason, created_at FROM sprint_time_extensions WHERE sprint_id = ? ORDER BY created_at, extension_id", sprintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.SprintTimeExtension, 0)
	for rows.Next() {
		var item model.SprintTimeExtension
		var created string
		if err := rows.Scan(&item.ExtensionID, &item.SprintID, &item.DurationSeconds, &item.Reason, &created); err != nil {
			return nil, err
		}
		if item.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListTimeEntries(ctx context.Context, sprintID string) ([]model.TimeEntry, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM sprints WHERE sprint_id = ?", sprintID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: sprint %s", ErrNotFound, sprintID)
		}
		return nil, fmt.Errorf("read time entry sprint: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT time_entry_id, sprint_id, entry_type, started_at, ended_at,
		duration_seconds, reason, created_at FROM time_entries WHERE sprint_id = ? ORDER BY started_at, time_entry_id`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("list time entries: %w", err)
	}
	defer rows.Close()
	entries := make([]model.TimeEntry, 0)
	for rows.Next() {
		var entry model.TimeEntry
		var startedAt, endedAt, createdAt string
		if err := rows.Scan(&entry.TimeEntryID, &entry.SprintID, &entry.EntryType, &startedAt, &endedAt,
			&entry.DurationSeconds, &entry.Reason, &createdAt); err != nil {
			return nil, fmt.Errorf("scan time entry: %w", err)
		}
		var err error
		if entry.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt); err != nil {
			return nil, fmt.Errorf("parse time entry started_at: %w", err)
		}
		if entry.EndedAt, err = time.Parse(time.RFC3339Nano, endedAt); err != nil {
			return nil, fmt.Errorf("parse time entry ended_at: %w", err)
		}
		if entry.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse time entry created_at: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate time entries: %w", err)
	}
	return entries, nil
}

type sprintState struct {
	Sprint          model.Sprint
	ActiveStartedAt *time.Time
	HoldStartedAt   *time.Time
}

func loadSprintState(ctx context.Context, tx *sql.Tx, sprintID string) (sprintState, error) {
	var state sprintState
	var startedAt, endedAt, activeStartedAt, holdStartedAt sql.NullString
	var subtaskID sql.NullString
	var createdAt, updatedAt string
	err := tx.QueryRowContext(ctx, `SELECT sprint_id, project_id, category_id, task_id, subtask_id, item_address,
		sprint_name, sprint_description, sprint_goal, status, priority, estimated_duration_seconds,
		buffer_pct, buffer_duration_seconds, active_duration_seconds, hold_duration_seconds,
		started_at, ended_at, active_started_at, hold_started_at, created_at, updated_at
		FROM sprints WHERE sprint_id = ?`, sprintID).Scan(&state.Sprint.SprintID, &state.Sprint.ProjectID,
		&state.Sprint.CategoryID, &state.Sprint.TaskID, &subtaskID, &state.Sprint.ItemAddress, &state.Sprint.Name,
		&state.Sprint.Description, &state.Sprint.Goal, &state.Sprint.Status, &state.Sprint.Priority,
		&state.Sprint.EstimatedDurationSeconds, &state.Sprint.BufferPct, &state.Sprint.BufferDurationSeconds,
		&state.Sprint.ActiveDurationSeconds, &state.Sprint.HoldDurationSeconds, &startedAt, &endedAt,
		&activeStartedAt, &holdStartedAt, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sprintState{}, fmt.Errorf("%w: sprint %s", ErrNotFound, sprintID)
		}
		return sprintState{}, fmt.Errorf("read sprint: %w", err)
	}
	if subtaskID.Valid {
		state.Sprint.SubtaskID = subtaskID.String
	}
	var errParse error
	if state.Sprint.StartedAt, errParse = parseOptionalTime(startedAt); errParse != nil {
		return sprintState{}, errParse
	}
	if state.Sprint.EndedAt, errParse = parseOptionalTime(endedAt); errParse != nil {
		return sprintState{}, errParse
	}
	if state.ActiveStartedAt, errParse = parseOptionalTime(activeStartedAt); errParse != nil {
		return sprintState{}, errParse
	}
	if state.HoldStartedAt, errParse = parseOptionalTime(holdStartedAt); errParse != nil {
		return sprintState{}, errParse
	}
	if state.Sprint.CreatedAt, errParse = time.Parse(time.RFC3339Nano, createdAt); errParse != nil {
		return sprintState{}, fmt.Errorf("parse sprint created_at: %w", errParse)
	}
	if state.Sprint.UpdatedAt, errParse = time.Parse(time.RFC3339Nano, updatedAt); errParse != nil {
		return sprintState{}, fmt.Errorf("parse sprint updated_at: %w", errParse)
	}
	return state, nil
}

func scanSprint(scanner interface{ Scan(...any) error }) (model.Sprint, error) {
	var sprint model.Sprint
	var startedAt, endedAt, subtaskID sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(&sprint.SprintID, &sprint.ProjectID, &sprint.CategoryID, &sprint.TaskID, &subtaskID, &sprint.ItemAddress,
		&sprint.Name, &sprint.Description, &sprint.Goal, &sprint.Status, &sprint.Priority, &sprint.EstimatedDurationSeconds,
		&sprint.BufferPct, &sprint.BufferDurationSeconds, &sprint.ActiveDurationSeconds, &sprint.HoldDurationSeconds,
		&startedAt, &endedAt, &createdAt, &updatedAt); err != nil {
		return model.Sprint{}, fmt.Errorf("scan sprint: %w", err)
	}
	if subtaskID.Valid {
		sprint.SubtaskID = subtaskID.String
	}
	var err error
	if sprint.StartedAt, err = parseOptionalTime(startedAt); err != nil {
		return model.Sprint{}, err
	}
	if sprint.EndedAt, err = parseOptionalTime(endedAt); err != nil {
		return model.Sprint{}, err
	}
	if sprint.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return model.Sprint{}, fmt.Errorf("parse sprint created_at: %w", err)
	}
	if sprint.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return model.Sprint{}, fmt.Errorf("parse sprint updated_at: %w", err)
	}
	return sprint, nil
}

func addTimeEntry(ctx context.Context, tx *sql.Tx, sprintID, entryType string, startedAt, endedAt time.Time, duration int64, reason string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO time_entries (sprint_id, entry_type, started_at, ended_at, duration_seconds, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, sprintID, entryType, stamp(startedAt), stamp(endedAt), duration, reason, stamp(endedAt))
	return err
}

func elapsedSeconds(startedAt, endedAt time.Time) int64 {
	seconds := int64(endedAt.Sub(startedAt).Seconds())
	if seconds < 0 {
		return 0
	}
	return seconds
}

func parseOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, fmt.Errorf("parse sprint timestamp: %w", err)
	}
	return &parsed, nil
}

func stamp(value time.Time) string { return value.UTC().Round(0).Format(time.RFC3339Nano) }

func allocateItemNumber(ctx context.Context, tx *sql.Tx) (int64, error) {
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO id_counters(name, next_value) VALUES ('item', 10000)"); err != nil {
		return 0, fmt.Errorf("initialize item sequence: %w", err)
	}
	var number int64
	if err := tx.QueryRowContext(ctx, "SELECT next_value FROM id_counters WHERE name = 'item'").Scan(&number); err != nil {
		return 0, fmt.Errorf("read item sequence: %w", err)
	}
	if number > 99999999 {
		return 0, errors.New("item ID space exhausted")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE id_counters SET next_value = next_value + 1 WHERE name = 'item'"); err != nil {
		return 0, fmt.Errorf("advance item sequence: %w", err)
	}
	return number, nil
}

func scanCategory(scanner interface{ Scan(...any) error }) (model.Category, error) {
	var category model.Category
	var parentCategoryID sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(&category.CategoryID, &category.ProjectID, &parentCategoryID, &category.ItemAddress, &category.Name,
		&category.Description, &category.Goal, &category.Status, &category.Priority, &category.ProgressPct,
		&createdAt, &updatedAt); err != nil {
		return model.Category{}, fmt.Errorf("scan category: %w", err)
	}
	category.ParentCategoryID = parentCategoryID.String
	var err error
	category.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.Category{}, fmt.Errorf("parse category created_at: %w", err)
	}
	category.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return model.Category{}, fmt.Errorf("parse category updated_at: %w", err)
	}
	return category, nil
}
