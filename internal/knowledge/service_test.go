// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package knowledge_test

import (
	"context"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/knowledge"
)

type providerStub struct {
	deadlineSeen bool
}

func (p *providerStub) Status(ctx context.Context) (knowledge.ProviderStatus, error) {
	_, p.deadlineSeen = ctx.Deadline()
	return knowledge.ProviderStatus{Available: true, Provider: knowledge.ProviderNameObsidian, ProtocolVersion: knowledge.ProtocolVersion}, nil
}
func (p *providerStub) Search(ctx context.Context, query knowledge.KnowledgeQuery) ([]knowledge.KnowledgeResult, error) {
	_, p.deadlineSeen = ctx.Deadline()
	return []knowledge.KnowledgeResult{{ID: "MG-1", Title: query.Query}}, nil
}
func (p *providerStub) Get(ctx context.Context, id string) (knowledge.KnowledgeItem, error) {
	_, p.deadlineSeen = ctx.Deadline()
	return knowledge.KnowledgeItem{}, nil
}
func (p *providerStub) Create(ctx context.Context, candidate knowledge.KnowledgeCandidate, idempotencyKey string) (knowledge.KnowledgeItem, error) {
	_, p.deadlineSeen = ctx.Deadline()
	return knowledge.KnowledgeItem{}, nil
}

func TestServiceValidatesAndBoundsProviderCalls(t *testing.T) {
	stub := &providerStub{}
	service := knowledge.NewService(stub, 250*time.Millisecond)
	status, err := service.Status(context.Background())
	if err != nil || !status.Available || !stub.deadlineSeen {
		t.Fatalf("status=%#v deadline=%v err=%v", status, stub.deadlineSeen, err)
	}
	if _, err := service.Search(context.Background(), knowledge.KnowledgeQuery{Query: "   "}); err == nil {
		t.Fatal("expected empty query rejection")
	}
	results, err := service.Search(context.Background(), knowledge.KnowledgeQuery{Query: " durable context ", Limit: 1})
	if err != nil || len(results) != 1 || results[0].Title != "durable context" {
		t.Fatalf("results=%#v err=%v", results, err)
	}
	candidate := knowledge.KnowledgeCandidate{Title: "A decision", Content: "A bounded decision body.", Type: knowledge.ItemTypeDecision, Visibility: knowledge.VisibilityGalaxy}
	if err := knowledge.ValidateCandidate(candidate); err != nil {
		t.Fatalf("valid candidate rejected: %v", err)
	}
	if err := knowledge.ValidateCandidate(knowledge.KnowledgeCandidate{Title: "bad", Content: "body", Type: "unknown", Visibility: knowledge.VisibilityGalaxy}); err == nil {
		t.Fatal("expected unknown type rejection")
	}
}
