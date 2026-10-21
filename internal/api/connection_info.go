package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/observer"
	"github.com/JustSebNL/timekeeper/internal/store"
	"github.com/JustSebNL/timekeeper/internal/version"
)

const apiContractVersion = "v1"

type connectionInfoResponse struct {
	Service      string   `json:"service"`
	InstanceID   string   `json:"instance_id"`
	Version      string   `json:"version"`
	Commit       string   `json:"commit,omitempty"`
	BuildDate    string   `json:"build_date,omitempty"`
	GoVersion    string   `json:"go_version,omitempty"`
	APIVersion   string   `json:"api_version"`
	Capabilities []string `json:"capabilities"`
	FriendlyURLs []string `json:"friendly_urls"`
}

func connectionInfo(database *store.Store, runtime RuntimeStatus, knowledgeService KnowledgeService, fileHistoryService FileHistoryService, obs observer.Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceID, err := database.InstanceID(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		build := version.Get()
		friendlyURLs := append([]string(nil), runtime.FriendlyURLs...)
		if friendlyURLs == nil {
			friendlyURLs = []string{}
		}
		writeJSON(w, http.StatusOK, connectionInfoResponse{
			Service:      "TimeKeeper",
			InstanceID:   instanceID,
			Version:      build.Version,
			Commit:       build.Commit,
			BuildDate:    build.Date,
			GoVersion:    build.GoVersion,
			APIVersion:   apiContractVersion,
			Capabilities: connectionCapabilities(runtime, knowledgeService, fileHistoryService, obs),
			FriendlyURLs: friendlyURLs,
		})
	}
}

func connectionCapabilities(runtime RuntimeStatus, knowledgeService KnowledgeService, fileHistoryService FileHistoryService, obs observer.Observer) []string {
	capabilities := []string{
		"projects.list",
		"projects.execution_tree",
		"tasks.lifecycle",
		"tasks.batch.validate",
		"execution_batches.validate",
		"project.events",
		"routing.candidates.read",
		"routing.category.evaluate",
		"routing.proposal.evaluate",
		"routing.proposal.apply",
	}
	if runtime.PulseGuardianEnabled {
		capabilities = append(capabilities, "pulse.guardian")
	}
	if knowledgeService != nil {
		capabilities = append(capabilities, "knowledge.galaxy")
	}
	if fileHistoryService != nil {
		capabilities = append(capabilities, "files.history")
	}
	if strings.TrimSpace(runtime.PluginStoreRoot) != "" {
		capabilities = append(capabilities, "plugins.store")
	}
	if obs != nil {
		capabilities = append(capabilities, "events.journal")
	}
	sort.Strings(capabilities)
	return capabilities
}
