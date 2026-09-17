package api

import (
	"encoding/json"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"io"
	"net/http"
)

func codingOutcomes(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			v, err := db.CodingComparisons(r.Context(), r.PathValue("projectID"))
			if err != nil {
				writeStoreError(w, err)
				return
			}
			writeJSON(w, 200, v)
			return
		}
		var in model.CodingOutcomeInput
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			writeError(w, 400, "invalid_outcome", "Provide one valid coding outcome JSON object.")
			return
		}
		if d.Decode(&struct{}{}) != io.EOF {
			writeError(w, 400, "invalid_outcome", "Provide one JSON object.")
			return
		}
		v, duplicate, err := db.RecordCodingOutcome(r.Context(), r.PathValue("projectID"), in)
		if err != nil {
			writeError(w, 400, "invalid_outcome", err.Error())
			return
		}
		code := 201
		if duplicate {
			code = 200
		}
		writeJSON(w, code, v)
	}
}
