// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/semantic"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestVectorSearchBuildTagBoundary(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "vector.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	vec := semantic.Embed("task scheduling vector search test")

	results, err := s.SearchNativeVector(ctx, vec, 5)
	if err != nil {
		t.Fatalf("SearchNativeVector returned error: %v", err)
	}

	enabled := s.IsNativeVectorSearchEnabled()
	t.Logf("IsNativeVectorSearchEnabled=%v results_count=%d", enabled, len(results))
}
