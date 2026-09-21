// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package plugin is the seam between TimeKeeper Core and optional
// functionality.
//
// Core defines what a plugin may be (a Kind) and how it is registered.
// It never imports a plugin. A plugin is compiled in only when it
// registers itself here, and even then core reaches it through an
// interface rather than a concrete type.
//
// The goal is a stable core: installing an item must not require
// editing core packages.
package plugin

import (
	"fmt"
	"sort"
	"sync"
)

// Kind identifies what a plugin provides. Core owns these constants;
// plugins declare which one they implement.
type Kind string

const (
	// KindKnowledge provides a Memory Galaxy / knowledge provider.
	KindKnowledge Kind = "knowledge"
)

// Descriptor is the core-visible description of a registered plugin.
type Descriptor struct {
	ID           string   `json:"id"`
	Kind         Kind     `json:"kind"`
	Name         string   `json:"name,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// Factory builds a plugin instance from its configuration. It returns
// an opaque value; the caller type-asserts to the interface its Kind
// promises (for example knowledge.KnowledgeProvider).
//
// Returning an error disables that plugin rather than failing startup,
// because core must run without any optional plugin.
type Factory func(config map[string]string) (any, error)

type registration struct {
	descriptor Descriptor
	factory    Factory
}

var (
	mu       sync.RWMutex
	registry = map[Kind]map[string]registration{}
)

// Register adds a plugin factory under a kind and id. It is intended
// to be called from a plugin package's init or an explicit
// registration call; core never calls it for a plugin it imports.
//
// Registering the same kind+id twice is a programming error and
// panics, so a duplicate is caught at startup rather than silently
// shadowing a plugin.
func Register(descriptor Descriptor, factory Factory) {
	if descriptor.ID == "" || descriptor.Kind == "" || factory == nil {
		panic("plugin.Register requires an id, a kind and a factory")
	}
	mu.Lock()
	defer mu.Unlock()
	if registry[descriptor.Kind] == nil {
		registry[descriptor.Kind] = map[string]registration{}
	}
	if _, exists := registry[descriptor.Kind][descriptor.ID]; exists {
		panic(fmt.Sprintf("plugin already registered: kind=%s id=%s", descriptor.Kind, descriptor.ID))
	}
	registry[descriptor.Kind][descriptor.ID] = registration{descriptor: descriptor, factory: factory}
}

// Descriptors lists registered plugins of a kind, sorted by id so
// startup logging and any UI listing are stable.
func Descriptors(kind Kind) []Descriptor {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Descriptor, 0, len(registry[kind]))
	for _, reg := range registry[kind] {
		out = append(out, reg.descriptor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Has reports whether a plugin id is registered for a kind.
func Has(kind Kind, id string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := registry[kind][id]
	return ok
}

// Build instantiates a registered plugin. A missing plugin yields a
// typed error, never a panic, so an uninstalled or misconfigured
// optional item degrades into "unavailable" instead of taking core
// down.
func Build(kind Kind, id string, config map[string]string) (any, error) {
	mu.RLock()
	reg, ok := registry[kind][id]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("plugin not installed: kind=%s id=%s", kind, id)
	}
	instance, err := reg.factory(config)
	if err != nil {
		return nil, fmt.Errorf("plugin %s (%s) failed to start: %w", id, kind, err)
	}
	return instance, nil
}

// Available reports whether any plugin of a kind is registered.
func Available(kind Kind) bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(registry[kind]) > 0
}
