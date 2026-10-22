// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/store"
)

var refPattern = regexp.MustCompile(`^(P|T|SP|G)-(\d+)$`)

func resolveReference(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := strings.TrimSpace(r.URL.Query().Get("ref"))
		projectID := strings.TrimSpace(r.URL.Query().Get("project"))
		if ref == "" {
			writeError(w, http.StatusBadRequest, "missing_reference", "ref query parameter is required.")
			return
		}
		matches := refPattern.FindStringSubmatch(ref)
		if matches == nil {
			writeError(w, http.StatusBadRequest, "invalid_reference", "Reference must match P-#, T-#, SP-#, or G-#.")
			return
		}
		prefix := matches[1]
		switch prefix {
		case "P":
			writeJSON(w, http.StatusOK, map[string]any{"ref": ref, "type": "project", "path": "/projects/" + ref})
		case "T":
			writeJSON(w, http.StatusOK, map[string]any{"ref": ref, "type": "task", "path": "/tasks/" + ref, "project_id": projectID})
		case "SP":
			writeJSON(w, http.StatusOK, map[string]any{"ref": ref, "type": "sprint", "path": "/sprints/" + ref, "project_id": projectID})
		case "G":
			writeJSON(w, http.StatusOK, map[string]any{"ref": ref, "type": "gem", "path": "/gems/" + ref, "project_id": projectID})
		default:
			writeError(w, http.StatusBadRequest, "unknown_reference_type", fmt.Sprintf("Unknown reference prefix %q.", prefix))
		}
	}
}
