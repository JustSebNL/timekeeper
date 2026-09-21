// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// CreateKnowledgeLink inserts a promotion mapping once. Repeating the same
// source/promotion key returns the original mapping with duplicate=true.
func (s *Store) CreateKnowledgeLink(ctx context.Context, input model.CreateKnowledgeLinkInput) (model.KnowledgeLink, bool, error) {
	if s == nil || s.db == nil {
		return model.KnowledgeLink{}, false, errors.New("knowledge link store is closed")
	}
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.GalaxyID = strings.TrimSpace(input.GalaxyID)
	input.PromotionKey = strings.TrimSpace(input.PromotionKey)
	input.Status = strings.TrimSpace(input.Status)
	input.LastError = strings.TrimSpace(input.LastError)
	if input.SourceType == "" || input.SourceID == "" || input.PromotionKey == "" {
		return model.KnowledgeLink{}, false, errors.New("knowledge link source and promotion key are required")
	}
	if input.Status == "" {
		input.Status = model.KnowledgePromotionPending
	}
	if !validKnowledgePromotionStatus(input.Status) {
		return model.KnowledgeLink{}, false, fmt.Errorf("invalid knowledge promotion status %q", input.Status)
	}
	if len(input.PromotionKey) > 512 || len(input.SourceID) > 256 || len(input.GalaxyID) > 128 || len(input.LastError) > 2000 {
		return model.KnowledgeLink{}, false, errors.New("knowledge link field exceeds its limit")
	}
	now := time.Now().UTC().Round(0).Format(time.RFC3339Nano)
	var galaxyValue any = input.GalaxyID
	if galaxyValue == "" {
		galaxyValue = nil
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO knowledge_links
		(source_type, source_id, galaxy_id, promotion_key, promotion_status, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_type, source_id, promotion_key) DO NOTHING`,
		input.SourceType, input.SourceID, galaxyValue, input.PromotionKey, input.Status, input.LastError, now, now)
	if err != nil {
		return model.KnowledgeLink{}, false, fmt.Errorf("insert knowledge link: %w", err)
	}
	created := false
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil {
		created = affected == 1
	}
	link, err := s.getKnowledgeLink(ctx, `WHERE source_type = ? AND source_id = ? AND promotion_key = ?`, input.SourceType, input.SourceID, input.PromotionKey)
	if err != nil {
		return model.KnowledgeLink{}, false, err
	}
	return link, !created, nil
}

func (s *Store) GetKnowledgeLink(ctx context.Context, linkID int64) (model.KnowledgeLink, error) {
	if linkID <= 0 {
		return model.KnowledgeLink{}, fmt.Errorf("invalid knowledge link id %d", linkID)
	}
	return s.getKnowledgeLink(ctx, `WHERE link_id = ?`, linkID)
}

func (s *Store) GetKnowledgeLinkByPromotion(ctx context.Context, sourceType, sourceID, promotionKey string) (model.KnowledgeLink, error) {
	return s.getKnowledgeLink(ctx, `WHERE source_type = ? AND source_id = ? AND promotion_key = ?`, strings.TrimSpace(sourceType), strings.TrimSpace(sourceID), strings.TrimSpace(promotionKey))
}

// UpdateKnowledgeLink records provider convergence without changing source
// identity or promotion key. It is intentionally narrow to preserve retry
// reconciliation semantics.
func (s *Store) UpdateKnowledgeLink(ctx context.Context, linkID int64, status, galaxyID, lastError string) (model.KnowledgeLink, error) {
	status = strings.TrimSpace(status)
	galaxyID = strings.TrimSpace(galaxyID)
	lastError = strings.TrimSpace(lastError)
	if linkID <= 0 || !validKnowledgePromotionStatus(status) {
		return model.KnowledgeLink{}, errors.New("invalid knowledge link update")
	}
	if len(galaxyID) > 128 || len(lastError) > 2000 {
		return model.KnowledgeLink{}, errors.New("knowledge link update exceeds its limit")
	}
	var galaxyValue any = galaxyID
	if galaxyValue == "" {
		galaxyValue = nil
	}
	result, err := s.db.ExecContext(ctx, `UPDATE knowledge_links SET galaxy_id = ?, promotion_status = ?, last_error = ?, updated_at = ? WHERE link_id = ?`, galaxyValue, status, lastError, time.Now().UTC().Round(0).Format(time.RFC3339Nano), linkID)
	if err != nil {
		return model.KnowledgeLink{}, fmt.Errorf("update knowledge link: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return model.KnowledgeLink{}, fmt.Errorf("%w: knowledge link %d", ErrNotFound, linkID)
	}
	return s.GetKnowledgeLink(ctx, linkID)
}

func (s *Store) getKnowledgeLink(ctx context.Context, where string, args ...any) (model.KnowledgeLink, error) {
	var link model.KnowledgeLink
	var galaxyID sql.NullString
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT link_id, source_type, source_id, galaxy_id, promotion_key, promotion_status, last_error, created_at, updated_at FROM knowledge_links `+where, args...).Scan(
		&link.LinkID, &link.SourceType, &link.SourceID, &galaxyID, &link.PromotionKey, &link.PromotionStatus, &link.LastError, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.KnowledgeLink{}, fmt.Errorf("%w: knowledge link", ErrNotFound)
		}
		return model.KnowledgeLink{}, fmt.Errorf("get knowledge link: %w", err)
	}
	if galaxyID.Valid {
		link.GalaxyID = galaxyID.String
	}
	var parseErr error
	link.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt)
	if parseErr != nil {
		return model.KnowledgeLink{}, fmt.Errorf("parse knowledge link created_at: %w", parseErr)
	}
	link.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updatedAt)
	if parseErr != nil {
		return model.KnowledgeLink{}, fmt.Errorf("parse knowledge link updated_at: %w", parseErr)
	}
	return link, nil
}

func validKnowledgePromotionStatus(status string) bool {
	switch status {
	case model.KnowledgePromotionPending, model.KnowledgePromotionCreated,
		model.KnowledgePromotionFailedRetryable, model.KnowledgePromotionFailedPermanent,
		model.KnowledgePromotionConflict:
		return true
	default:
		return false
	}
}
