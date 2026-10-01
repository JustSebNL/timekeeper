package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func activityError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrAgentSessionConflict) {
		writeError(w, 409, "agent_session_conflict", err.Error())
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "agent_not_found", err.Error())
		return
	}
	writeError(w, 400, "invalid_agent_activity", err.Error())
}

func agentActivityRoutes(mux *http.ServeMux, s *store.Store) {
	mux.HandleFunc("GET /api/v1/agents/work-states", func(w http.ResponseWriter, r *http.Request) {
		states, err := s.ListAgentWorkStatesAt(r.Context(), time.Now())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": states})
	})
	mux.HandleFunc("GET /api/v1/agents/{agentID}/work-state", func(w http.ResponseWriter, r *http.Request) {
		state, err := s.AgentWorkStateAt(r.Context(), r.PathValue("agentID"), time.Now())
		if err != nil {
			activityError(w, err)
			return
		}
		writeJSON(w, 200, state)
	})
	mux.HandleFunc("POST /api/v1/agents/{agentID}/activity", func(w http.ResponseWriter, r *http.Request) {
		var in model.AgentActivityInput
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			writeError(w, 400, "invalid_agent_activity", err.Error())
			return
		}
		if id := r.Header.Get("X-Agent-ID"); id != "" && id != r.PathValue("agentID") {
			writeError(w, 409, "agent_identity_mismatch", "Activity must belong to the requesting agent.")
			return
		}
		if session := r.Header.Get("X-Agent-Session-ID"); session != "" {
			if in.SessionID != "" && in.SessionID != session {
				writeError(w, 409, "agent_session_conflict", "Session metadata does not match.")
				return
			}
			in.SessionID = session
		}
		state, err := s.RecordAgentActivityAt(r.Context(), r.PathValue("agentID"), in, time.Now())
		if err != nil {
			activityError(w, err)
			return
		}
		writeJSON(w, 200, state)
	})
}

type activityResponse struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (w *activityResponse) Header() http.Header { return w.header }
func (w *activityResponse) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
}
func (w *activityResponse) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	return w.body.Write(b)
}

// projectIDFromRequest resolves the addressed project. Routing places path
// values on a request clone that is only visible to the matched handler, and
// activity tracking runs around the mux, so fall back to the URL when the
// routed value is not populated yet.
func projectIDFromRequest(r *http.Request) string {
	if value := strings.TrimSpace(r.PathValue("projectID")); value != "" {
		return value
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, part := range parts {
		if part == "projects" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// Identity is attribution, not authentication. A session id is required for
// state-aware enrollment; legacy callers without one retain legacy behaviour.
func trackAgentActivity(s *store.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, session := strings.TrimSpace(r.Header.Get("X-Agent-ID")), strings.TrimSpace(r.Header.Get("X-Agent-Session-ID"))
		if strings.HasPrefix(id, "FASE-") {
			actor, err := s.ResolveFASEActor(r.Context(), id, session, time.Now())
			if err != nil {
				activityError(w, err)
				return
			}
			r = r.WithContext(model.WithFASEActor(r.Context(), actor))
		}
		if id == "" || session == "" {
			next.ServeHTTP(w, r)
			return
		}
		input := model.AgentActivityInput{SessionID: session, Runtime: r.Header.Get("X-Agent-Runtime"), Event: "contact"}
		state, err := s.RecordAgentActivityAt(r.Context(), id, input, time.Now())
		if err != nil {
			activityError(w, err)
			return
		}
		// Reads and agent-control traffic establish contact, never material work.
		if r.Method == http.MethodGet || r.Method == http.MethodHead || strings.HasPrefix(r.URL.Path, "/api/v1/agents/") {
			next.ServeHTTP(w, r)
			return
		}
		rec := &activityResponse{header: make(http.Header)}
		next.ServeHTTP(rec, r)
		if rec.code >= 200 && rec.code < 300 {
			path := r.URL.Path
			if (strings.HasPrefix(path, "/api/v1/sprints/") && (strings.HasSuffix(path, "/start") || strings.HasSuffix(path, "/resume"))) || strings.HasSuffix(path, "/sprints/claim-next") {
				var sprint model.Sprint
				if json.Unmarshal(rec.body.Bytes(), &sprint) == nil && sprint.SprintID != "" {
					input.Event = "work"
					input.SprintID = sprint.SprintID
				}
			} else if (state.State == "working" || state.State == "idle" || state.State == "waiting") && state.WorkExpected && state.ProjectID != "" && projectIDFromRequest(r) == state.ProjectID && (strings.HasSuffix(path, "/notes") || strings.HasSuffix(path, "/messages") || strings.HasSuffix(path, "/messages/checkpoint")) {
				input.Event = "progress"
				input.SprintID = state.ActiveSprintID
				input.ProjectID = state.ProjectID
			}
			if input.Event != "contact" {
				if _, err = s.RecordAgentActivityAt(r.Context(), id, input, time.Now()); err != nil {
					writeError(w, 500, "agent_tracking_failed", "Work mutation succeeded but activity tracking failed; inspect state before retrying.")
					return
				}
			}
		}
		for key, values := range rec.header {
			w.Header()[key] = values
		}
		if rec.code == 0 {
			rec.code = 200
		}
		w.WriteHeader(rec.code)
		_, _ = w.Write(rec.body.Bytes())
	})
}
