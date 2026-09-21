// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestKnowledgeLinkPromotionIsIdempotentAndSupportsPendingRows(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer database.Close()
	ctx := context.Background()

	first, duplicate, err := database.CreateKnowledgeLink(ctx, model.CreateKnowledgeLinkInput{
		SourceType:   model.KnowledgeSourceTrail,
		SourceID:     "TR-1",
		PromotionKey: "trail:TR-1:promotion:1",
	})
	if err != nil || duplicate || first.LinkID == 0 || first.PromotionStatus != model.KnowledgePromotionPending {
		t.Fatalf("first link = %#v duplicate=%v err=%v", first, duplicate, err)
	}
	second, duplicate, err := database.CreateKnowledgeLink(ctx, model.CreateKnowledgeLinkInput{
		SourceType:   model.KnowledgeSourceTrail,
		SourceID:     "TR-1",
		PromotionKey: "trail:TR-1:promotion:1",
	})
	if err != nil || !duplicate || second.LinkID != first.LinkID {
		t.Fatalf("retry = %#v duplicate=%v err=%v", second, duplicate, err)
	}

	other, duplicate, err := database.CreateKnowledgeLink(ctx, model.CreateKnowledgeLinkInput{
		SourceType:   model.KnowledgeSourceTrail,
		SourceID:     "TR-2",
		PromotionKey: "trail:TR-2:promotion:1",
	})
	if err != nil || duplicate || other.LinkID == first.LinkID {
		t.Fatalf("second pending link = %#v duplicate=%v err=%v", other, duplicate, err)
	}

	updated, err := database.UpdateKnowledgeLink(ctx, first.LinkID, model.KnowledgePromotionCreated, "MG-1", "")
	if err != nil || updated.GalaxyID != "MG-1" || updated.PromotionStatus != model.KnowledgePromotionCreated {
		t.Fatalf("updated link = %#v err=%v", updated, err)
	}
	readBack, err := database.GetKnowledgeLinkByPromotion(ctx, model.KnowledgeSourceTrail, "TR-1", "trail:TR-1:promotion:1")
	if err != nil || readBack.LinkID != first.LinkID || readBack.GalaxyID != "MG-1" {
		t.Fatalf("read back = %#v err=%v", readBack, err)
	}
	if _, err := database.GetKnowledgeLink(ctx, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing link error = %v", err)
	}
}

func TestKnowledgeLinkRejectsDuplicateGalaxyID(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer database.Close()
	ctx := context.Background()
	_, _, err = database.CreateKnowledgeLink(ctx, model.CreateKnowledgeLinkInput{SourceType: "trail", SourceID: "TR-1", GalaxyID: "MG-1", PromotionKey: "key-1", Status: model.KnowledgePromotionCreated})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, _, err = database.CreateKnowledgeLink(ctx, model.CreateKnowledgeLinkInput{SourceType: "trail", SourceID: "TR-2", GalaxyID: "MG-1", PromotionKey: "key-2", Status: model.KnowledgePromotionCreated}); err == nil {
		t.Fatal("expected duplicate Galaxy ID error")
	}
}
