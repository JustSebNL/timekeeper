// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

//go:build !sqlite_vec

package store

import (
	"context"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// IsNativeVectorSearchEnabled reports whether the native sqlite-vec C extension is linked.
func (s *Store) IsNativeVectorSearchEnabled() bool {
	return false
}

// SearchNativeVector performs vector search using fallback or reports build status.
func (s *Store) SearchNativeVector(ctx context.Context, embedding []float32, k int) ([]model.VectorSearchResult, error) {
	return []model.VectorSearchResult{}, nil
}
