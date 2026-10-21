package desktop

import (
	"context"
	"fmt"
)

const PluginCatalogSchemaVersion = "timekeeper-plugin-index/v1"

// PluginCatalogEntry represents a single plugin declaration exposed by Core.
type PluginCatalogEntry struct {
	SchemaVersion string            `json:"schema_version"`
	ID            string            `json:"id"`
	Version       string            `json:"version"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Runtime       string            `json:"runtime"`
	Entrypoint    string            `json:"entrypoint"`
	Protocol      string            `json:"protocol"`
	Capabilities  []string          `json:"capabilities,omitempty"`
	Requires      []string          `json:"requires,omitempty"`
	Permissions   map[string]bool   `json:"permissions,omitempty"`
	Artifact      string            `json:"artifact,omitempty"`
	SHA256        string            `json:"sha256,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// PluginCatalog is the read-only catalog of plugins returned by Core.
type PluginCatalog struct {
	SchemaVersion string               `json:"schema_version"`
	GeneratedAt   string               `json:"generated_at"`
	Plugins       []PluginCatalogEntry `json:"plugins"`
}

// PluginCatalog reads the plugin catalog from the Core API.
func (c *CoreClient) PluginCatalog(ctx context.Context) (PluginCatalog, error) {
	var catalog PluginCatalog
	if err := c.get(ctx, "/api/v1/plugins", &catalog); err != nil {
		return PluginCatalog{}, err
	}
	if catalog.SchemaVersion != PluginCatalogSchemaVersion {
		return PluginCatalog{}, fmt.Errorf("unsupported plugin catalog schema: %q", catalog.SchemaVersion)
	}
	if catalog.Plugins == nil {
		catalog.Plugins = []PluginCatalogEntry{}
	}
	return catalog, nil
}
