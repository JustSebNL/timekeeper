// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

//go:build sqlite_vec

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/semantic"
)

// IsNativeVectorSearchEnabled reports whether the native sqlite-vec C extension is linked.
func (s *Store) IsNativeVectorSearchEnabled() bool {
	return true
}

// SearchNativeVector queries native sqlite-vec virtual table for KNN vector matches.
func (s *Store) SearchNativeVector(ctx context.Context, embedding []float32, k int) ([]model.VectorSearchResult, error) {
	if k <= 0 {
		k = 10
	}

	encoded := semantic.Encode(embedding)
	query := `SELECT entity_id, entity_type, distance FROM vec_embeddings WHERE embedding MATCH ? ORDER BY distance LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, encoded, k)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return []model.VectorSearchResult{}, nil
		}
		return nil, fmt.Errorf("sqlite-vec search failed: %w", err)
	}
	defer rows.Close()

	results := make([]model.VectorSearchResult, 0)
	for rows.Next() {
		var res model.VectorSearchResult
		if err := rows.Scan(&res.EntityID, &res.EntityType, &res.Distance); err != nil {
			return nil, fmt.Errorf("scan vector result: %w", err)
		}
		res.Score = 1.0 / (1.0 + res.Distance)
		results = append(results, res)
	}

	return results, nil
}
