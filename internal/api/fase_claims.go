package api

import (
	"errors"
	"github.com/JustSebNL/timekeeper/internal/store"
	"net/http"
)

func writeClaimConflict(w http.ResponseWriter, err error) bool {
	var conflict *store.ClaimConflictError
	if !errors.As(err, &conflict) {
		return false
	}
	writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "item_already_claimed", "message": conflict.Error(), "details": map[string]string{"item_id": conflict.ItemID, "agent_id": "FASE-" + conflict.AgentKey, "agent_key": conflict.AgentKey}}})
	return true
}
func faseClaimRoutes(mux *http.ServeMux, s *store.Store) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/execution-claims", func(w http.ResponseWriter, r *http.Request) {
		claims, e := s.ListFASEClaims(r.Context(), r.PathValue("projectID"))
		if e != nil {
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"items": claims})
	})
	mux.HandleFunc("POST /api/v1/execution-claims/{itemID}", func(w http.ResponseWriter, r *http.Request) {
		claim, e := s.ClaimFASEItem(r.Context(), r.PathValue("itemID"))
		if e != nil {
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 200, claim)
	})
	mux.HandleFunc("POST /api/v1/execution-claims/{itemID}/release", func(w http.ResponseWriter, r *http.Request) {
		if e := s.ReleaseFASEClaim(r.Context(), r.PathValue("itemID")); e != nil {
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 200, map[string]bool{"released": true})
	})
}
