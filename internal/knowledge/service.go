// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("knowledge provider unavailable")

// Service is the provider-neutral boundary used by TimeKeeper. It owns
// validation and deadlines; provider implementations own their transport and
// storage details.
type Service struct {
	provider KnowledgeProvider
	timeout  time.Duration
}

func NewService(provider KnowledgeProvider, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = DefaultCallTimeout
	}
	return &Service{provider: provider, timeout: timeout}
}

func (s *Service) Status(ctx context.Context) (ProviderStatus, error) {
	if s == nil || s.provider == nil {
		return ProviderStatus{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.provider.Status(ctx)
}

func (s *Service) Search(ctx context.Context, query KnowledgeQuery) ([]KnowledgeResult, error) {
	if s == nil || s.provider == nil {
		return nil, ErrUnavailable
	}
	query.Query = strings.TrimSpace(query.Query)
	if query.Query == "" || len(query.Query) > MaxQueryLength {
		return nil, fmt.Errorf("knowledge query must contain 1-%d characters", MaxQueryLength)
	}
	if query.Limit <= 0 {
		query.Limit = 8
	}
	if query.Limit > MaxResultLimit {
		return nil, fmt.Errorf("knowledge result limit cannot exceed %d", MaxResultLimit)
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.provider.Search(ctx, query)
}

func (s *Service) Get(ctx context.Context, id string) (KnowledgeItem, error) {
	if s == nil || s.provider == nil {
		return KnowledgeItem{}, ErrUnavailable
	}
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return KnowledgeItem{}, errors.New("knowledge id is required and bounded")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.provider.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, candidate KnowledgeCandidate, idempotencyKey string) (KnowledgeItem, error) {
	if s == nil || s.provider == nil {
		return KnowledgeItem{}, ErrUnavailable
	}
	if err := ValidateCandidate(candidate); err != nil {
		return KnowledgeItem{}, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > MaxPromotionKey {
		return KnowledgeItem{}, fmt.Errorf("idempotency key must contain 1-%d characters", MaxPromotionKey)
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.provider.Create(ctx, candidate, idempotencyKey)
}

func ValidateCandidate(candidate KnowledgeCandidate) error {
	if strings.TrimSpace(candidate.Title) == "" || len([]rune(candidate.Title)) > MaxCandidateTitle {
		return fmt.Errorf("knowledge title must contain 1-%d characters", MaxCandidateTitle)
	}
	if strings.TrimSpace(candidate.Content) == "" || len([]rune(candidate.Content)) > MaxCandidateContent {
		return fmt.Errorf("knowledge content must contain 1-%d characters", MaxCandidateContent)
	}
	if !validItemType(candidate.Type) {
		return fmt.Errorf("unsupported knowledge type %q", candidate.Type)
	}
	if candidate.Visibility != VisibilityPrivate && candidate.Visibility != VisibilityGalaxy {
		return fmt.Errorf("unsupported knowledge visibility %q", candidate.Visibility)
	}
	return nil
}

func validItemType(value ItemType) bool {
	switch value {
	case ItemTypeKnowledge, ItemTypeDecision, ItemTypePattern, ItemTypeFailure,
		ItemTypeInsight, ItemTypeProcedure, ItemTypeReference, ItemTypeRAGSource,
		ItemTypeSkill, ItemTypeHarness, ItemTypeResearch:
		return true
	default:
		return false
	}
}
