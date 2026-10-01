package api

import (
	"encoding/json"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"html/template"
	"io"
	"net/http"
	"time"
)

func faseIdentityRoutes(mux *http.ServeMux, s *store.Store) {
	faseClaimRoutes(mux, s)
	mux.HandleFunc("POST /api/v1/agent-identities/resume", func(w http.ResponseWriter, r *http.Request) {
		var in model.FASEResumeInput
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil || in.AgentID == "" || in.SessionID == "" {
			writeError(w, 400, "invalid_fase_resume", "A FASE agent and activity session are required.")
			return
		}
		id, err := s.ResolveFASEActor(r.Context(), in.AgentID, in.SessionID, time.Now())
		if err != nil {
			activityError(w, err)
			return
		}
		writeJSON(w, 200, id)
	})
	mux.HandleFunc("POST /api/v1/agent-identities/resolve", func(w http.ResponseWriter, r *http.Request) {
		var in model.FASEIdentityInput
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			writeError(w, 400, "invalid_fase_identity", "A bounded identity object is required.")
			return
		}
		if err := d.Decode(&struct{}{}); err != io.EOF {
			writeError(w, 400, "invalid_fase_identity", "Exactly one identity object is required.")
			return
		}
		id, err := s.ResolveFASEIdentityAt(r.Context(), in, time.Now())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		w.Header().Set("X-FASE-ID", id.FASEID)
		writeJSON(w, 200, id)
	})
	mux.HandleFunc("GET /api/v1/agent-profiles/{identity}", func(w http.ResponseWriter, r *http.Request) {
		profile, err := s.FASEProfile(r.Context(), r.PathValue("identity"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 200, profile)
	})
	mux.HandleFunc("GET /api/v1/agent-profiles/{identity}/view", func(w http.ResponseWriter, r *http.Request) {
		profile, err := s.FASEProfile(r.Context(), r.PathValue("identity"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
		_ = faseProfileTemplate.Execute(w, profile)
	})
}

var faseProfileTemplate = template.Must(template.New("fase-profile").Funcs(template.FuncMap{"known": func(s string) string {
	if s == "" {
		return "Unknown"
	}
	return s
}}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Agent Profile · {{.Identity.AgentKey}}</title>
<style>body{font:16px system-ui;max-width:850px;margin:3rem auto;padding:0 1rem;background:#10151e;color:#e7edf8}a{color:#88bcff}dt{color:#aab7cd;margin-top:1rem}dd{margin:.25rem 0}li{margin:.5rem 0}</style></head><body>
<a href="/">TimeKeeper</a><h1>Agent Profile</h1><h2>{{.Identity.FASEID}}</h2>
<dl><dt>Permanent fingerprint-bound key</dt><dd>{{.Identity.AgentKey}}</dd><dt>Fingerprint</dt><dd>{{.Identity.MaskedFingerprint}}</dd>
<dt>Identity scope</dt><dd>{{.Identity.IdentityScope}}</dd><dt>Runtime</dt><dd>{{.Identity.Runtime}} · {{known .Identity.Metadata.RuntimeVersion}}</dd>
<dt>Provider</dt><dd>{{known .Identity.Metadata.Provider}}</dd><dt>Language model</dt><dd>{{known .Identity.Metadata.Model}}</dd><dt>Model version</dt><dd>{{known .Identity.Metadata.ModelVersion}}</dd>
<dt>Metadata provenance</dt><dd>{{.Identity.MetadataSource}} — unknown fields are not inferred.</dd></dl>
<h2>FASE-ID history</h2><p>Only periods with recorded use are listed. Each ID belongs to this same permanent identity.</p><ul>{{range .PreviousIDs}}<li><a href="/api/v1/agent-profiles/{{.}}/view">{{.}}</a></li>{{end}}</ul>
<h2>Current state and execution claims</h2>
{{with .WorkState}}<p>{{.State}} · {{.ActiveSprintID}} · {{.Reason}}</p>{{else}}<p>No activity recorded.</p>{{end}}
<ul>{{range .Claims}}<li>{{.ItemID}} — {{.FASEID}}</li>{{else}}<li>No execution claims.</li>{{end}}</ul>
<h2>Recorded work for {{.Identity.FASEID}}</h2>
<p>Profile metadata above is current and reported. Each event below retains its original metadata snapshot.</p>
<ol>{{range .Events}}<li>{{.CreatedAt}} · {{.EntityID}} · {{.Message}}{{with .Actor}} — <a href="/api/v1/agent-profiles/{{.FASEID}}/view">{{.FASEID}}</a> · {{known .Metadata.Model}} ({{.MetadataSource}}){{end}}</li>{{else}}<li>No project events for this month.</li>{{end}}</ol>
{{if .EventsTruncated}}<p>Showing the newest 100 events. Use the project events API for the complete project history.</p>{{end}}
<p><a href="/api/v1/agents/{{.Identity.AgentID}}/work-state">Current work state (JSON)</a> · <a href="/api/v1/agents/{{.Identity.AgentID}}/nudges/history">Guardian history (JSON)</a></p>
</body></html>`))
