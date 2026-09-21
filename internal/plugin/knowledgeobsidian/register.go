// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package obsidianplugin registers the Obsidian Memory Galaxy
// knowledge provider with the core plugin registry.
//
// This is the only place that names both core and the provider. Core
// imports this package when the plugin is enabled; it never imports
// the provider directly.
package obsidianplugin

import (
	"time"

	"github.com/JustSebNL/timekeeper/internal/knowledge"
	"github.com/JustSebNL/timekeeper/internal/knowledge/obsidian"
	"github.com/JustSebNL/timekeeper/internal/plugin"
)

// ID is the stable plugin identifier used by configuration and the
// Store catalogue.
const ID = "extension.memory-galaxy"

// DescriptorKey names the configuration entry holding the endpoint
// descriptor path.
const DescriptorKey = "descriptor_path"

func init() {
	plugin.Register(plugin.Descriptor{
		ID:           ID,
		Kind:         plugin.KindKnowledge,
		Name:         "Obsidian Memory Galaxy",
		Version:      "0.1.0",
		Capabilities: []string{"knowledge.search", "knowledge.promote"},
	}, Build)
}

// Build constructs the Obsidian provider from its configuration.
func Build(config map[string]string) (any, error) {
	descriptorPath := config[DescriptorKey]
	if descriptorPath == "" {
		return nil, &knowledge.ProviderError{
			Code:      "plugin_not_configured",
			Message:   "Obsidian endpoint descriptor path is empty",
			Retryable: false,
			Status:    0,
		}
	}
	return obsidian.NewClient(descriptorPath), nil
}

// DefaultTimeout is the provider call timeout core applies.
const DefaultTimeout = 5 * time.Second
