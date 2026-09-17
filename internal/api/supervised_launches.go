package api

import (
	"encoding/json"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"io"
	"net/http"
)

func reserveSupervisedLaunch(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in model.SupervisedLaunchInput
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		if d.Decode(&in) != nil || d.Decode(&struct{}{}) != io.EOF {
			writeError(w, 400, "invalid_launch", "Provide one launch JSON object.")
			return
		}
		v, err := db.ReserveSupervisedLaunch(r.Context(), r.PathValue("sprintID"), in)
		if err != nil {
			writeError(w, 400, "invalid_launch", err.Error())
			return
		}
		writeJSON(w, 200, v)
	}
}
