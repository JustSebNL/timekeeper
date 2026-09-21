// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"testing"

	"github.com/JustSebNL/timekeeper/internal/plugin"
)

// The server binary imports the registration package for its side
// effect, so the plugin must be visible in the real binary. This
// guards the wiring: if someone drops the import, this fails.
func TestKnowledgePluginRegisteredInServerBinary(t *testing.T) {
	if !plugin.Has(plugin.KindKnowledge, "extension.memory-galaxy") {
		t.Fatal("extension.memory-galaxy is not registered in the server binary")
	}
	if !plugin.Available(plugin.KindKnowledge) {
		t.Fatal("no knowledge plugins available")
	}
}
