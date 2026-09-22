package api_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func pluginTestStore(t *testing.T) *store.Store {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestPluginStoreListsAndServesChecksummedArtifacts(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "cactus.needle")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := []byte("plugin artifact\n")
	artifactPath := filepath.Join(pluginDir, "needle.tar.gz")
	if err := os.WriteFile(artifactPath, artifact, 0o644); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(artifact)
	manifest := `{"schema_version":"timekeeper-plugin-manifest/v1","id":"cactus.needle","version":"0.1.0","name":"Needle","runtime":"python3","entrypoint":"plugin.py","protocol":"edge-hook-jsonrpc/v1","artifact":"needle.tar.gz","sha256":"` + hex.EncodeToString(hash[:]) + `"}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := api.NewWithHistory(pluginTestStore(t), api.RuntimeStatus{PluginStoreRoot: root}, nil, nil)
	indexResponse := httptest.NewRecorder()
	handler.ServeHTTP(indexResponse, httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil))
	if indexResponse.Code != http.StatusOK || !strings.Contains(indexResponse.Body.String(), "cactus.needle") {
		t.Fatalf("index status=%d body=%s", indexResponse.Code, indexResponse.Body.String())
	}

	artifactResponse := httptest.NewRecorder()
	handler.ServeHTTP(artifactResponse, httptest.NewRequest(http.MethodGet, "/api/v1/plugins/cactus.needle/artifact", nil))
	if artifactResponse.Code != http.StatusOK || artifactResponse.Body.String() != string(artifact) {
		t.Fatalf("artifact status=%d body=%q", artifactResponse.Code, artifactResponse.Body.String())
	}
}

func TestPluginStoreIsDisabledWithoutRoot(t *testing.T) {
	handler := api.New(pluginTestStore(t))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "plugin_store_unconfigured") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
