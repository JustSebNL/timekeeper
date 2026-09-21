package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	pluginIndexSchema    = "timekeeper-plugin-index/v1"
	pluginManifestSchema = "timekeeper-plugin-manifest/v1"
	maxPluginManifest    = 128 << 10
)

var pluginIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,127}$`)
var pluginSHA256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type pluginManifest struct {
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

type pluginIndex struct {
	SchemaVersion string           `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	Plugins       []pluginManifest `json:"plugins"`
}

func pluginStoreRoutes(mux *http.ServeMux, root string) {
	mux.HandleFunc("GET /api/v1/plugins", listPluginManifests(root))
	mux.HandleFunc("GET /api/v1/plugins/{pluginID}", getPluginManifest(root))
	mux.HandleFunc("GET /api/v1/plugins/{pluginID}/artifact", getPluginArtifact(root))
}

func listPluginManifests(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if strings.TrimSpace(root) == "" {
			writeError(w, http.StatusServiceUnavailable, "plugin_store_unconfigured", "The plugin store is not configured.")
			return
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, pluginIndex{SchemaVersion: pluginIndexSchema, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Plugins: []pluginManifest{}})
				return
			}
			writeError(w, http.StatusInternalServerError, "plugin_store_unavailable", "The plugin store could not be read.")
			return
		}
		manifests := make([]pluginManifest, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() || !pluginIDPattern.MatchString(entry.Name()) {
				continue
			}
			manifest, err := readPluginManifest(root, entry.Name())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "plugin_manifest_invalid", "The plugin store contains an invalid manifest.")
				return
			}
			manifests = append(manifests, manifest)
		}
		sort.Slice(manifests, func(i, j int) bool { return manifests[i].ID < manifests[j].ID })
		writeJSON(w, http.StatusOK, pluginIndex{
			SchemaVersion: pluginIndexSchema,
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			Plugins:       manifests,
		})
	}
}

func getPluginManifest(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		manifest, err := readPluginManifest(root, r.PathValue("pluginID"))
		if err != nil {
			writeError(w, http.StatusNotFound, "plugin_not_found", "The requested plugin was not found.")
			return
		}
		writeJSON(w, http.StatusOK, manifest)
	}
}

func getPluginArtifact(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		manifest, err := readPluginManifest(root, r.PathValue("pluginID"))
		if err != nil || manifest.Artifact == "" {
			writeError(w, http.StatusNotFound, "plugin_artifact_not_found", "The requested plugin artifact was not found.")
			return
		}
		artifactPath, err := safePluginPath(root, manifest.ID, manifest.Artifact)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "plugin_artifact_invalid", "The plugin artifact path is invalid.")
			return
		}
		file, err := os.Open(artifactPath)
		if err != nil {
			writeError(w, http.StatusNotFound, "plugin_artifact_not_found", "The requested plugin artifact was not found.")
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.IsDir() {
			writeError(w, http.StatusNotFound, "plugin_artifact_not_found", "The requested plugin artifact was not found.")
			return
		}
		if manifest.SHA256 != "" {
			hash := sha256.New()
			if _, err := io.Copy(hash, file); err != nil {
				writeError(w, http.StatusInternalServerError, "plugin_artifact_read_failed", "The plugin artifact could not be read.")
				return
			}
			if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), manifest.SHA256) {
				writeError(w, http.StatusInternalServerError, "plugin_artifact_checksum_failed", "The plugin artifact checksum does not match its manifest.")
				return
			}
			if _, err := file.Seek(0, 0); err != nil {
				writeError(w, http.StatusInternalServerError, "plugin_artifact_read_failed", "The plugin artifact could not be rewound.")
				return
			}
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(manifest.Artifact)))
		http.ServeContent(w, r, filepath.Base(manifest.Artifact), info.ModTime(), file)
	}
}

func readPluginManifest(root, pluginID string) (pluginManifest, error) {
	if !pluginIDPattern.MatchString(pluginID) {
		return pluginManifest{}, fmt.Errorf("invalid plugin ID")
	}
	manifestPath, err := safePluginPath(root, pluginID, "plugin.json")
	if err != nil {
		return pluginManifest{}, err
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return pluginManifest{}, err
	}
	defer file.Close()
	var manifest pluginManifest
	decoder := json.NewDecoder(io.LimitReader(file, maxPluginManifest))
	if err := decoder.Decode(&manifest); err != nil {
		return pluginManifest{}, err
	}
	if manifest.SchemaVersion != pluginManifestSchema || manifest.ID != pluginID || manifest.Version == "" || manifest.Runtime == "" || manifest.Entrypoint == "" {
		return pluginManifest{}, fmt.Errorf("invalid plugin manifest")
	}
	if manifest.Artifact == "" || !pluginSHA256Pattern.MatchString(strings.ToLower(manifest.SHA256)) {
		return pluginManifest{}, fmt.Errorf("artifact and lowercase sha256 are required")
	}
	if _, err := safePluginPath(root, pluginID, manifest.Artifact); err != nil {
		return pluginManifest{}, err
	}
	return manifest, nil
}

func safePluginPath(root, pluginID, relative string) (string, error) {
	if !pluginIDPattern.MatchString(pluginID) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("unsafe plugin path")
	}
	base, err := filepath.Abs(filepath.Join(root, pluginID))
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Join(base, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("plugin path escapes plugin directory")
	}
	return candidate, nil
}
