// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package plugin_test

import (
	"context"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/knowledge"
	"github.com/JustSebNL/timekeeper/internal/plugin"
)

// fakeProvider stands in for a plugin that core has never heard of.
// If it can be registered, built and used through the knowledge
// interface, then installing an item genuinely requires no core edit.
type fakeProvider struct {
	id string
}

func (f *fakeProvider) Status(context.Context) (knowledge.ProviderStatus, error) {
	return knowledge.ProviderStatus{Available: true, Provider: f.id, ProtocolVersion: knowledge.ProtocolVersion}, nil
}

func (f *fakeProvider) Search(context.Context, knowledge.KnowledgeQuery) ([]knowledge.KnowledgeResult, error) {
	return nil, nil
}

func (f *fakeProvider) Get(context.Context, string) (knowledge.KnowledgeItem, error) {
	return knowledge.KnowledgeItem{}, nil
}

func (f *fakeProvider) Create(context.Context, knowledge.KnowledgeCandidate, string) (knowledge.KnowledgeItem, error) {
	return knowledge.KnowledgeItem{}, nil
}

func registerFake(t *testing.T, id string) {
	t.Helper()
	plugin.Register(plugin.Descriptor{ID: id, Kind: plugin.KindKnowledge, Name: "fake"}, func(map[string]string) (any, error) {
		return &fakeProvider{id: id}, nil
	})
}

func TestRegistryBuildsUnknownPluginThroughInterface(t *testing.T) {
	const id = "test.fake-provider"
	registerFake(t, id)

	instance, err := plugin.Build(plugin.KindKnowledge, id, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	provider, ok := instance.(knowledge.KnowledgeProvider)
	if !ok {
		t.Fatalf("plugin did not satisfy knowledge.KnowledgeProvider: %T", instance)
	}
	status, err := provider.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Provider != id {
		t.Fatalf("provider = %q, want %q", status.Provider, id)
	}
}

func TestMissingPluginIsTypedNotFatal(t *testing.T) {
	if _, err := plugin.Build(plugin.KindKnowledge, "not.installed", nil); err == nil {
		t.Fatal("expected an error for an uninstalled plugin")
	}
	if plugin.Has(plugin.KindKnowledge, "not.installed") {
		t.Fatal("uninstalled plugin reported as present")
	}
}

func TestDescriptorsListRegisteredPlugins(t *testing.T) {
	const id = "test.listed"
	registerFake(t, id)

	descriptors := plugin.Descriptors(plugin.KindKnowledge)
	found := false
	for _, d := range descriptors {
		if d.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s missing from descriptors: %+v", id, descriptors)
	}
	// Registration is a property of whichever binary imports the
	// plugin package; the registry itself only reports what it holds.
	t.Logf("registered knowledge plugins in this binary: %d", len(descriptors))
}

func TestDuplicateRegistrationPanics(t *testing.T) {
	plugin.Register(plugin.Descriptor{ID: "test.dup", Kind: plugin.KindKnowledge}, func(map[string]string) (any, error) {
		return &fakeProvider{id: "test.dup"}, nil
	})
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration did not panic")
		}
	}()
	plugin.Register(plugin.Descriptor{ID: "test.dup", Kind: plugin.KindKnowledge}, func(map[string]string) (any, error) {
		return nil, nil
	})
}
