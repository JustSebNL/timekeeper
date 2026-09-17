package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/JustSebNL/timekeeper/internal/store"
)

func updateBugReportStatus(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID string `json:"project_id"`
			Status    string `json:"status"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_bug_report_status", "Bug report status input must be valid JSON.")
			return
		}
		report, err := database.UpdateBugReportStatus(r.Context(), input.ProjectID, r.PathValue("reportID"), input.Status)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "bug_report_not_found", "Bug report was not found.")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_bug_report_status", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}
