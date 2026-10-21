package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

var (
	ErrRegistrationQueueItemNotFound   = errors.New("registration queue item not found")
	ErrRegistrationIdempotencyConflict = errors.New("registration idempotency key was reused with a different payload")
)

const registrationQueueColumns = `queue_seq, queue_id, project_id, kind, idempotency_key, payload_hash, payload_json, status, counts_json, validation_json, result_json, attempts, last_error, created_at, updated_at, processed_at`

// EnqueueRegistration durably stores an offer before any Task/Subtask/Sprint
// rows are registered. Reusing an idempotency key with the same payload returns
// the existing item; reusing it with different content is a conflict.
func (s *Store) EnqueueRegistration(ctx context.Context, projectID, kind, idempotencyKey string, input model.ExecutionBatchInput) (model.RegistrationQueueItem, bool, error) {
	projectID = strings.TrimSpace(projectID)
	kind = strings.TrimSpace(kind)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if projectID == "" {
		return model.RegistrationQueueItem{}, false, errors.New("registration queue Project ID is required")
	}
	if !validRegistrationQueueKind(kind) {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("unsupported registration queue kind %q", kind)
	}
	if len(idempotencyKey) > 160 {
		return model.RegistrationQueueItem{}, false, errors.New("registration idempotency key must be at most 160 characters")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("encode registration payload: %w", err)
	}
	payloadDigest := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(payloadDigest[:])
	counts := model.ValidateExecutionBatchDocument(input).Counts
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("encode registration counts: %w", err)
	}
	queueID, err := newRegistrationQueueID()
	if err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("create registration queue ID: %w", err)
	}
	now := time.Now().UTC().Round(0)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("begin registration enqueue: %w", err)
	}
	defer tx.Rollback()
	if idempotencyKey != "" {
		item, err := scanRegistrationQueue(tx.QueryRowContext(ctx, `SELECT `+registrationQueueColumns+` FROM registration_queue WHERE project_id = ? AND idempotency_key = ?`, projectID, idempotencyKey), true, true)
		if err == nil {
			if item.PayloadHash != payloadHash {
				return model.RegistrationQueueItem{}, false, ErrRegistrationIdempotencyConflict
			}
			return item, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return model.RegistrationQueueItem{}, false, fmt.Errorf("find registration idempotency key: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO registration_queue (
		queue_id, project_id, kind, idempotency_key, payload_hash, payload_json, status,
		counts_json, attempts, last_error, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, '', ?, ?)`, queueID, projectID, kind, idempotencyKey, payloadHash, string(payload), model.RegistrationQueueStatusQueued, string(countsJSON), stamp(now), stamp(now))
	if err != nil {
		if idempotencyKey != "" && strings.Contains(strings.ToLower(err.Error()), "unique") {
			return model.RegistrationQueueItem{}, false, ErrRegistrationIdempotencyConflict
		}
		return model.RegistrationQueueItem{}, false, fmt.Errorf("persist registration offer: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("commit registration offer: %w", err)
	}
	item := model.RegistrationQueueItem{
		QueueID:        queueID,
		ProjectID:      projectID,
		Kind:           kind,
		Status:         model.RegistrationQueueStatusQueued,
		IdempotencyKey: idempotencyKey,
		PayloadHash:    payloadHash,
		Payload:        &input,
		Counts:         counts,
		Attempts:       0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return item, false, nil
}

func validRegistrationQueueKind(kind string) bool {
	switch kind {
	case model.RegistrationQueueKindTask, model.RegistrationQueueKindSubtask, model.RegistrationQueueKindSprint, model.RegistrationQueueKindTaskBatch, model.RegistrationQueueKindExecutionBatch:
		return true
	default:
		return false
	}
}

func newRegistrationQueueID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "RQ-" + hex.EncodeToString(random[:]), nil
}

// ProcessNextRegistrationQueueItem registers the oldest queued offer. Queue
// status and all created entities commit in one SQLite transaction; a crash
// rolls both back, leaving the offer queued for recovery.
func (s *Store) ProcessNextRegistrationQueueItem(ctx context.Context) (model.RegistrationQueueItem, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("begin registration queue processing: %w", err)
	}
	defer tx.Rollback()
	if err := lockFASEWrite(ctx, tx); err != nil {
		return model.RegistrationQueueItem{}, false, err
	}
	item, err := scanRegistrationQueue(tx.QueryRowContext(ctx, `SELECT `+registrationQueueColumns+` FROM registration_queue WHERE status = ? ORDER BY queue_seq LIMIT 1`, model.RegistrationQueueStatusQueued), true, false)
	if errors.Is(err, sql.ErrNoRows) {
		return model.RegistrationQueueItem{}, false, nil
	}
	if err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("read next registration offer: %w", err)
	}
	if item.Payload == nil {
		return model.RegistrationQueueItem{}, false, errors.New("queued registration payload was not loaded")
	}
	input := *item.Payload
	result, validation, err := createExecutionBatchTx(ctx, tx, item.ProjectID, input)
	if err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("process queued registration %s: %w", item.QueueID, err)
	}
	now := time.Now().UTC().Round(0)
	validationJSON, err := json.Marshal(validation)
	if err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("encode registration validation: %w", err)
	}
	status := model.RegistrationQueueStatusRejected
	resultJSON := ""
	lastError := "registration rejected by Core validation"
	if validation.Valid {
		status = model.RegistrationQueueStatusRegistered
		resultBytes, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return model.RegistrationQueueItem{}, false, fmt.Errorf("encode registered result: %w", marshalErr)
		}
		resultJSON = string(resultBytes)
		lastError = ""
	}
	update, err := tx.ExecContext(ctx, `UPDATE registration_queue SET
		status = ?, validation_json = ?, result_json = ?, attempts = attempts + 1,
		last_error = ?, updated_at = ?, processed_at = ? WHERE queue_id = ? AND status = ?`,
		status, string(validationJSON), resultJSON, lastError, stamp(now), stamp(now), item.QueueID, model.RegistrationQueueStatusQueued)
	if err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("update registration queue status: %w", err)
	}
	if affected, err := update.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return model.RegistrationQueueItem{}, false, fmt.Errorf("check registration queue update: %w", err)
		}
		return model.RegistrationQueueItem{}, false, errors.New("registration queue item was claimed concurrently")
	}
	if err := tx.Commit(); err != nil {
		return model.RegistrationQueueItem{}, false, fmt.Errorf("commit registration queue processing: %w", err)
	}
	item.Status = status
	item.Validation = &validation
	item.Attempts++
	item.LastError = lastError
	item.UpdatedAt = now
	item.ProcessedAt = &now
	if validation.Valid {
		item.Result = &result
	}
	return item, true, nil
}

// ProcessRegistrationQueueItem drains earlier offers first and returns the
// requested item's durable state.
func (s *Store) ProcessRegistrationQueueItem(ctx context.Context, queueID string) (model.RegistrationQueueItem, error) {
	item, err := s.GetRegistrationQueueItem(ctx, "", queueID)
	if err != nil {
		return model.RegistrationQueueItem{}, err
	}
	if item.Status != model.RegistrationQueueStatusQueued {
		return item, nil
	}
	for range 1000 {
		processed, found, err := s.ProcessNextRegistrationQueueItem(ctx)
		if err != nil {
			return model.RegistrationQueueItem{}, err
		}
		if !found {
			break
		}
		if processed.QueueID == queueID {
			return processed, nil
		}
	}
	return s.GetRegistrationQueueItem(ctx, "", queueID)
}

// ProcessPendingRegistrationQueue drains at most limit pending offers in FIFO order.
func (s *Store) ProcessPendingRegistrationQueue(ctx context.Context, limit int) ([]model.RegistrationQueueItem, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	processed := make([]model.RegistrationQueueItem, 0)
	for range limit {
		item, found, err := s.ProcessNextRegistrationQueueItem(ctx)
		if err != nil {
			return processed, err
		}
		if !found {
			break
		}
		processed = append(processed, item)
	}
	return processed, nil
}

func (s *Store) ListRegistrationQueue(ctx context.Context, projectID, status string, limit int) ([]model.RegistrationQueueItem, error) {
	items, _, err := s.ListRegistrationQueuePage(ctx, projectID, status, limit, 0)
	return items, err
}

// ListRegistrationQueuePage returns a FIFO page and reports whether another
// page exists. Offset is bounded so API callers cannot request unbounded scans.
func (s *Store) ListRegistrationQueuePage(ctx context.Context, projectID, status string, limit, offset int) ([]model.RegistrationQueueItem, bool, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, false, errors.New("registration queue Project ID is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 || offset > 1_000_000 {
		return nil, false, errors.New("registration queue offset must be between 0 and 1000000")
	}
	query := `SELECT ` + registrationQueueColumns + ` FROM registration_queue WHERE project_id = ?`
	args := []any{projectID}
	if strings.TrimSpace(status) != "" {
		query += ` AND status = ?`
		args = append(args, strings.TrimSpace(status))
	}
	query += ` ORDER BY queue_seq LIMIT ? OFFSET ?`
	args = append(args, limit+1, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list registration queue: %w", err)
	}
	defer rows.Close()
	items := make([]model.RegistrationQueueItem, 0, limit)
	hasMore := false
	for rows.Next() {
		item, err := scanRegistrationQueue(rows, false, false)
		if err != nil {
			return nil, false, fmt.Errorf("scan registration queue item: %w", err)
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate registration queue: %w", err)
	}
	return items, hasMore, nil
}

func (s *Store) GetRegistrationQueueItem(ctx context.Context, projectID, queueID string) (model.RegistrationQueueItem, error) {
	query := `SELECT ` + registrationQueueColumns + ` FROM registration_queue WHERE queue_id = ?`
	args := []any{strings.TrimSpace(queueID)}
	if strings.TrimSpace(projectID) != "" {
		query += ` AND project_id = ?`
		args = append(args, strings.TrimSpace(projectID))
	}
	item, err := scanRegistrationQueue(s.db.QueryRowContext(ctx, query, args...), true, true)
	if errors.Is(err, sql.ErrNoRows) {
		return model.RegistrationQueueItem{}, fmt.Errorf("%w: %s", ErrRegistrationQueueItemNotFound, queueID)
	}
	if err != nil {
		return model.RegistrationQueueItem{}, fmt.Errorf("read registration queue item: %w", err)
	}
	return item, nil
}

// GetRegistrationQueueItemByIdempotencyKey reads an existing offer before
// routing Apply revalidates current candidates. This lets an identical replay
// return its durable result even though the successful registration changed
// the candidate-state hash.
func (s *Store) GetRegistrationQueueItemByIdempotencyKey(ctx context.Context, projectID, idempotencyKey string) (model.RegistrationQueueItem, error) {
	projectID = strings.TrimSpace(projectID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if projectID == "" || idempotencyKey == "" {
		return model.RegistrationQueueItem{}, ErrRegistrationQueueItemNotFound
	}
	item, err := scanRegistrationQueue(s.db.QueryRowContext(ctx, `SELECT `+registrationQueueColumns+`
		FROM registration_queue WHERE project_id = ? AND idempotency_key = ?`, projectID, idempotencyKey), true, true)
	if errors.Is(err, sql.ErrNoRows) {
		return model.RegistrationQueueItem{}, fmt.Errorf("%w: idempotency key", ErrRegistrationQueueItemNotFound)
	}
	if err != nil {
		return model.RegistrationQueueItem{}, fmt.Errorf("read registration by idempotency key: %w", err)
	}
	return item, nil
}

func scanRegistrationQueue(scanner interface{ Scan(...any) error }, includePayload, includeResult bool) (model.RegistrationQueueItem, error) {
	var item model.RegistrationQueueItem
	var payloadJSON, countsJSON, validationJSON, resultJSON, createdAt, updatedAt string
	var processedAt sql.NullString
	if err := scanner.Scan(&item.QueueSeq, &item.QueueID, &item.ProjectID, &item.Kind, &item.IdempotencyKey, &item.PayloadHash, &payloadJSON, &item.Status, &countsJSON, &validationJSON, &resultJSON, &item.Attempts, &item.LastError, &createdAt, &updatedAt, &processedAt); err != nil {
		return model.RegistrationQueueItem{}, err
	}
	var err error
	if err = json.Unmarshal([]byte(countsJSON), &item.Counts); err != nil {
		return model.RegistrationQueueItem{}, fmt.Errorf("decode registration counts: %w", err)
	}
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model.RegistrationQueueItem{}, fmt.Errorf("parse registration created_at: %w", err)
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return model.RegistrationQueueItem{}, fmt.Errorf("parse registration updated_at: %w", err)
	}
	if processedAt.Valid {
		processed, parseErr := time.Parse(time.RFC3339Nano, processedAt.String)
		if parseErr != nil {
			return model.RegistrationQueueItem{}, fmt.Errorf("parse registration processed_at: %w", parseErr)
		}
		item.ProcessedAt = &processed
	}
	if includePayload {
		var payload model.ExecutionBatchInput
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return model.RegistrationQueueItem{}, fmt.Errorf("decode queued payload: %w", err)
		}
		item.Payload = &payload
	}
	if validationJSON != "" {
		var validation model.BatchValidationResult
		if err := json.Unmarshal([]byte(validationJSON), &validation); err != nil {
			return model.RegistrationQueueItem{}, fmt.Errorf("decode queued validation: %w", err)
		}
		item.Validation = &validation
	}
	if includeResult && resultJSON != "" {
		var result model.ExecutionBatchResult
		if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
			return model.RegistrationQueueItem{}, fmt.Errorf("decode queued result: %w", err)
		}
		item.Result = &result
	}
	return item, nil
}
