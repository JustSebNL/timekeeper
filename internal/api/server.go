// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/JustSebNL/timekeeper/internal/lifecycle"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/planning"
	"github.com/JustSebNL/timekeeper/internal/store"
)

// RuntimeStatus reports opt-in local runtime capabilities. It is informational:
// the Store remains authoritative for durable work and recovery evidence.
type RuntimeStatus struct {
	PulseGuardianEnabled         bool  `json:"pulse_guardian_enabled"`
	PulseGuardianIntervalSeconds int64 `json:"pulse_guardian_interval_seconds"`
	// RecoveryPolicy is the fixed, allowlist-described recovery behaviour the
	// local receiver performs. It is evidence, not an executable capability.
	RecoveryPolicy string `json:"recovery_policy,omitempty"`
}

// New returns the API with no process-local recovery worker assumed.
func New(database *store.Store) http.Handler {
	return NewWithRuntime(database, RuntimeStatus{})
}

// NewWithRuntime exposes process-local runtime configuration without coupling the
// Store to server flags or a framework-specific process supervisor.
func NewWithRuntime(database *store.Store, runtime RuntimeStatus) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /api/v1/llm-pipelines", listLLMPipelines(database))
	mux.HandleFunc("POST /api/v1/llm-pipelines", createLLMPipeline(database))
	mux.HandleFunc("GET /api/v1/pulse", pulse(database))
	mux.HandleFunc("GET /api/v1/guardian/status", guardianStatus(database, runtime))
	mux.HandleFunc("POST /api/v1/agents/{agentID}/progress", reportAgentProgress(database))
	mux.HandleFunc("GET /api/v1/agents/{agentID}/nudges/history", listPulseNudgeHistory(database))
	mux.HandleFunc("GET /api/v1/agents/{agentID}/nudges", listPendingPulseNudges(database))
	mux.HandleFunc("POST /api/v1/agents/{agentID}/nudges/{nudgeID}/ack", acknowledgePulseNudge(database))
	mux.HandleFunc("GET /api/v1/projects", listProjects(database))
	mux.HandleFunc("POST /api/v1/projects", createProject(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/metadata", updateProjectMetadata(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/status", updateProjectStatus(database))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/export", projectExport(database))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/execution-tree", projectExecutionTree(database))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/operational-summary", projectOperationalSummary(database))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/attention", projectAttention(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/sprints/claim-next", claimNextSprint(database))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/events", listProjectEvents(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/planning-drafts/{draftID}/apply", applyPlanningDraft(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/planning-drafts/generate", generatePlanningDraft(database))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/planning-drafts", listPlanningDrafts(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/planning-drafts", createPlanningDraft(database))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/notes", listProjectNotes(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/notes", createProjectNote(database))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/categories", listCategories(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/categories", createCategory(database))
	mux.HandleFunc("POST /api/v1/categories/{categoryID}/metadata", updateCategoryMetadata(database))
	mux.HandleFunc("POST /api/v1/categories/{categoryID}/status", updateCategoryStatus(database))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/tasks", listTasks(database))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/tasks", createTask(database))
	mux.HandleFunc("GET /api/v1/tasks/{taskID}/subtasks", listSubtasks(database))
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/subtasks", createSubtask(database))
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/metadata", updateTaskMetadata(database))
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/status", updateTaskStatus(database))
	mux.HandleFunc("GET /api/v1/tasks/{taskID}/sprints", listSprints(database))
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/sprints", createSprint(database))
	mux.HandleFunc("POST /api/v1/subtasks/{subtaskID}/status", updateSubtaskStatus(database))
	mux.HandleFunc("GET /api/v1/subtasks/{subtaskID}/sprints", listSubtaskSprints(database))
	mux.HandleFunc("POST /api/v1/subtasks/{subtaskID}/sprints", createSubtaskSprint(database))
	mux.HandleFunc("POST /api/v1/sprints/{sprintID}/start", transitionSprint(database, "start"))
	mux.HandleFunc("POST /api/v1/sprints/{sprintID}/hold", transitionSprint(database, "hold"))
	mux.HandleFunc("POST /api/v1/sprints/{sprintID}/resume", transitionSprint(database, "resume"))
	mux.HandleFunc("POST /api/v1/sprints/{sprintID}/complete", transitionSprint(database, "complete"))
	mux.HandleFunc("POST /api/v1/sprints/{sprintID}/hold-reason", updateSprintHoldReason(database))
	mux.HandleFunc("POST /api/v1/sprints/{sprintID}/cancel", transitionSprint(database, "cancel"))
	mux.HandleFunc("GET /api/v1/sprints/{sprintID}/retrieval-attempts", listSprintRetrievalAttempts(database))
	mux.HandleFunc("POST /api/v1/sprints/{sprintID}/retrieval-attempts", recordSprintRetrievalAttempt(database))
	mux.HandleFunc("GET /api/v1/sprints/{sprintID}/extensions", listSprintTimeExtensions(database))
	mux.HandleFunc("POST /api/v1/sprints/{sprintID}/extensions", addSprintTimeExtension(database))
	mux.HandleFunc("GET /api/v1/sprints/{sprintID}/time-entries", listTimeEntries(database))
	return recoverJSON(requireJSONForMutations(mux))
}

func requireJSONForMutations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "json_content_type_required", "Mutating API requests must use application/json.")
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
			if err != nil {
				writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Mutating API request bodies must be at most 1 MiB.")
				return
			}
			if len(bytes.TrimSpace(body)) > 0 {
				decoder := json.NewDecoder(bytes.NewReader(body))
				var value json.RawMessage
				if err := decoder.Decode(&value); err != nil {
					writeError(w, http.StatusBadRequest, "invalid_json", "Mutating API requests must contain valid JSON.")
					return
				}
				if err := decoder.Decode(&struct{}{}); err != io.EOF {
					writeError(w, http.StatusBadRequest, "invalid_json", "Mutating API requests must contain one JSON value.")
					return
				}
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		next.ServeHTTP(w, r)
	})
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func guardianStatus(database *store.Store, runtime RuntimeStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		registered, err := database.RegisteredGuardianURLs(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "guardian_status_failed", "Could not read registered Guardian callbacks.")
			return
		}
		status := map[string]any{
			"pulse_guardian_enabled":          runtime.PulseGuardianEnabled,
			"pulse_guardian_interval_seconds": runtime.PulseGuardianIntervalSeconds,
			"recovery_policy":                 runtime.RecoveryPolicy,
			"registered_callbacks":            registered,
		}
		writeJSON(w, http.StatusOK, status)
	}
}

// pulse returns local, read-only attention items. Delivery and scheduling remain the caller's responsibility.
func pulse(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := database.PulseAt(r.Context(), time.Now().UTC().Round(0))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "pulse_failed", "Could not calculate the local Pulse.")
			return
		}
		writeJSON(w, http.StatusOK, value)
	}
}

func reportAgentProgress(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.AgentPulseProgressInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_agent_progress", "Agent progress input must be valid JSON.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_agent_progress", "Agent progress input must contain one JSON object.")
			return
		}
		progress, err := database.ReportAgentProgress(r.Context(), r.PathValue("agentID"), input)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_agent_progress", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, progress)
	}
}

func listPendingPulseNudges(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := database.ListPendingPulseNudges(r.Context(), r.PathValue("agentID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_agent_id", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func listPulseNudgeHistory(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := database.ListPulseNudgeHistory(r.Context(), r.PathValue("agentID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_agent_id", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func acknowledgePulseNudge(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		var input map[string]json.RawMessage
		if err := decoder.Decode(&input); err != nil || input == nil || len(input) != 0 {
			writeError(w, http.StatusBadRequest, "invalid_pulse_nudge_acknowledgement", "Pulse nudge acknowledgement must be an empty JSON object.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_pulse_nudge_acknowledgement", "Pulse nudge acknowledgement must contain one JSON object.")
			return
		}
		nudgeID, err := strconv.ParseInt(r.PathValue("nudgeID"), 10, 64)
		if err != nil || nudgeID < 1 {
			writeError(w, http.StatusBadRequest, "invalid_pulse_nudge", "Nudge ID must be a positive integer.")
			return
		}
		nudge, err := database.AcknowledgePulseNudge(r.Context(), nudgeID, r.PathValue("agentID"))
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "pulse_nudge_not_found", "Pulse nudge was not found.")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_pulse_nudge_acknowledgement", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, nudge)
	}
}

func listLLMPipelines(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := database.ListLLMPipelines(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list_llm_pipelines_failed", "Could not list LLM pipelines.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func createLLMPipeline(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateLLMPipelineInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_llm_pipeline", "LLM pipeline input must be valid JSON.")
			return
		}
		pipeline, err := database.CreateLLMPipeline(r.Context(), input)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_llm_pipeline", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, pipeline)
	}
}

func listProjects(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects, err := database.ListProjects(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list_projects_failed", "Could not list projects.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": projects})
	}
}

func createProject(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateProjectInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_project", "Project input must be valid JSON.")
			return
		}
		project, err := database.CreateProject(r.Context(), input)
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_project", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, project)
	}
}

func updateProjectMetadata(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.UpdateProjectMetadataInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_project_metadata", "Project metadata input must be valid JSON.")
			return
		}
		project, err := database.UpdateProjectMetadata(r.Context(), r.PathValue("projectID"), input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, project)
	}
}

func updateProjectStatus(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.UpdateProjectStatusInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_project_status", "Project status input must be valid JSON.")
			return
		}
		project, err := database.UpdateProjectStatus(r.Context(), r.PathValue("projectID"), input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, project)
	}
}

func projectExport(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		tree, err := database.ProjectExecutionTree(r.Context(), projectID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		notes, err := database.ListProjectNotes(r.Context(), projectID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		events, err := database.ListProjectEvents(r.Context(), projectID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		summary, err := database.ProjectOperationalSummary(r.Context(), projectID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Format    string                          `json:"format"`
			ProjectID string                          `json:"project_id"`
			Tree      model.ExecutionTree             `json:"execution_tree"`
			Summary   model.ProjectOperationalSummary `json:"operational_summary"`
			Notes     []model.ProjectNote             `json:"notes"`
			Events    []model.ProjectEvent            `json:"events"`
		}{
			Format: "timekeeper-project-export/v1", ProjectID: projectID, Tree: tree, Summary: summary,
			Notes: notes, Events: events,
		})
	}
}

func projectExecutionTree(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tree, err := database.ProjectExecutionTree(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "project_execution_tree_failed", "Could not load the project execution tree.")
			return
		}
		writeJSON(w, http.StatusOK, tree)
	}
}

func projectOperationalSummary(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := database.ProjectOperationalSummary(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "project_operational_summary_failed", "Could not load the project operational summary.")
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
}

func projectAttention(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attention, err := database.ProjectAttention(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "project_attention_failed", "Could not load project attention.")
			return
		}
		writeJSON(w, http.StatusOK, attention)
	}
}

func listProjectEvents(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, err := database.ListProjectEvents(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_project_events_failed", "Could not list project events.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": events})
	}
}

func applyPlanningDraft(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		draftID, err := strconv.ParseInt(r.PathValue("draftID"), 10, 64)
		if err != nil || draftID < 1 {
			writeError(w, http.StatusBadRequest, "invalid_planning_draft", "Planning draft ID must be a positive integer.")
			return
		}
		tree, err := database.ApplyPlanningDraft(r.Context(), r.PathValue("projectID"), draftID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeStoreError(w, err)
			} else {
				writeError(w, http.StatusConflict, "planning_draft_not_applicable", err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, tree)
	}
}

func generatePlanningDraft(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PipelineID int64 `json:"pipeline_id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_planning_generation", "Planning generation input must be valid JSON.")
			return
		}
		pipeline, err := database.GetLLMPipeline(r.Context(), input.PipelineID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		tree, err := database.ProjectExecutionTree(r.Context(), r.PathValue("projectID"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		contextJSON, err := json.Marshal(tree)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "planning_context_failed", "Could not prepare Project context.")
			return
		}
		prompt := "Return only one JSON object matching version timekeeper-planning-draft/v1. It must contain summary and categories; each category has tasks; each task has name, estimated_duration_seconds, buffer_pct, sprints, subtasks; each sprint has name, estimated_duration_seconds, buffer_pct; each subtask has name, estimated_duration_seconds, sprints. Do not create work directly. Project authority context:\n" + string(contextJSON)
		raw, err := planning.NewClient(nil).GenerateDraft(r.Context(), pipeline, prompt)
		if err != nil {
			writeError(w, http.StatusBadGateway, "planning_generation_failed", err.Error())
			return
		}
		draft, err := database.CreatePlanningDraft(r.Context(), tree.Project.ProjectID, pipeline.PipelineID, raw)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "invalid_planning_draft", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, draft)
	}
}

func listPlanningDrafts(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := database.ListPlanningDrafts(r.Context(), r.PathValue("projectID"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}
func createPlanningDraft(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PipelineID int64  `json:"pipeline_id"`
			RawJSON    string `json:"raw_json"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_planning_draft", "Planning draft input must be valid JSON.")
			return
		}
		draft, err := database.CreatePlanningDraft(r.Context(), r.PathValue("projectID"), input.PipelineID, input.RawJSON)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, draft)
	}
}

func listProjectNotes(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notes, err := database.ListProjectNotes(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_project_notes_failed", "Could not list project notes.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": notes})
	}
}

func createProjectNote(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateProjectNoteInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_project_note", "Project note input must be valid JSON.")
			return
		}
		note, err := database.CreateProjectNote(r.Context(), r.PathValue("projectID"), input, r.Header.Get("X-Agent-ID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			if r.Context().Err() != nil {
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_project_note", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, note)
	}
}

func listCategories(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		categories, err := database.ListCategories(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_categories_failed", "Could not list categories.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": categories})
	}
}

func createCategory(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateCategoryInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_category", "Category input must be valid JSON.")
			return
		}
		category, err := database.CreateCategory(r.Context(), r.PathValue("projectID"), input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			if r.Context().Err() != nil {
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_category", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, category)
	}
}

func updateCategoryStatus(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.UpdateCategoryStatusInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_category_status", "Category status input must be valid JSON.")
			return
		}
		category, err := database.UpdateCategoryStatus(r.Context(), r.PathValue("categoryID"), input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "Category not found.")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_category_status", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, category)
	}
}

func listTasks(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks, err := database.ListTasks(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_tasks_failed", "Could not list tasks.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": tasks})
	}
}

func createTask(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateTaskInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_task", "Task input must be valid JSON.")
			return
		}
		task, err := database.CreateTask(r.Context(), r.PathValue("projectID"), input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_or_category_not_found", "Project or Category was not found.")
				return
			}
			if r.Context().Err() != nil {
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_task", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, task)
	}
}

func updateCategoryMetadata(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.UpdateCategoryMetadataInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_category_metadata", "Category metadata input must be valid JSON.")
			return
		}
		category, err := database.UpdateCategoryMetadata(r.Context(), r.PathValue("categoryID"), input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, category)
	}
}

func updateTaskMetadata(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.UpdateTaskMetadataInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_task_metadata", "Task metadata input must be valid JSON.")
			return
		}
		task, err := database.UpdateTaskMetadata(r.Context(), r.PathValue("taskID"), input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, task)
	}
}

func updateTaskStatus(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.UpdateTaskStatusInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_task_status", "Task status input must be valid JSON.")
			return
		}
		task, err := database.UpdateTaskStatus(r.Context(), r.PathValue("taskID"), input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, task)
	}
}

func updateSubtaskStatus(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.UpdateSubtaskStatusInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_subtask_status", "Subtask status input must be valid JSON.")
			return
		}
		subtask, err := database.UpdateSubtaskStatus(r.Context(), r.PathValue("subtaskID"), input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, subtask)
	}
}

func listSubtasks(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subtasks, err := database.ListSubtasks(r.Context(), r.PathValue("taskID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "task_not_found", "Task was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_subtasks_failed", "Could not list subtasks.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": subtasks})
	}
}

func createSubtask(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateSubtaskInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_subtask", "Subtask input must be valid JSON.")
			return
		}
		subtask, err := database.CreateSubtask(r.Context(), r.PathValue("taskID"), input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "task_not_found", "Task was not found.")
				return
			}
			if r.Context().Err() != nil {
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_subtask", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, subtask)
	}
}

func listSprints(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sprints, err := database.ListSprints(r.Context(), r.PathValue("taskID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "task_not_found", "Task was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_sprints_failed", "Could not list sprints.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": sprints})
	}
}

func createSprint(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateSprintInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_sprint", "Sprint input must be valid JSON.")
			return
		}
		sprint, err := database.CreateSprint(r.Context(), r.PathValue("taskID"), input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "task_not_found", "Task was not found.")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_sprint", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, sprint)
	}
}

func listSubtaskSprints(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sprints, err := database.ListSubtaskSprints(r.Context(), r.PathValue("subtaskID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "subtask_not_found", "Subtask was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_subtask_sprints_failed", "Could not list Subtask Sprints.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": sprints})
	}
}

func createSubtaskSprint(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateSprintInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_sprint", "Sprint input must be valid JSON.")
			return
		}
		sprint, err := database.CreateSubtaskSprint(r.Context(), r.PathValue("subtaskID"), input)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "subtask_not_found", "Subtask was not found.")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_sprint", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, sprint)
	}
}

func claimNextSprint(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sprint, err := database.ClaimNextSprint(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNoRunnableSprint) {
				writeError(w, http.StatusConflict, "no_runnable_sprint", "This Project has no runnable Open Sprint.")
				return
			}
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sprint)
	}
}

func transitionSprint(database *store.Store, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Reason string `json:"reason"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid_transition", "Transition input must be valid JSON.")
			return
		}
		sprint, err := database.TransitionSprint(r.Context(), r.PathValue("sprintID"), action, input.Reason)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "sprint_not_found", "Sprint was not found.")
				return
			}
			var transitionError *lifecycle.TransitionError
			if errors.As(err, &transitionError) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": map[string]any{
					"code":    "invalid_transition",
					"message": transitionError.Error(),
					"details": map[string]any{
						"status":          transitionError.Status,
						"allowed_actions": transitionError.AllowedActions,
					},
				}})
				return
			}
			writeError(w, http.StatusConflict, "invalid_transition", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sprint)
	}
}

func updateSprintHoldReason(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Reason string `json:"reason"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_hold_reason", "Hold reason input must be valid JSON.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_hold_reason", "Hold reason input must contain one JSON object.")
			return
		}
		sprint, err := database.UpdateSprintHoldReason(r.Context(), r.PathValue("sprintID"), input.Reason)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "sprint_not_found", "Sprint was not found.")
			return
		}
		if err != nil {
			writeError(w, http.StatusConflict, "invalid_hold_reason", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sprint)
	}
}

func recordSprintRetrievalAttempt(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Reason string `json:"reason"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_retrieval_attempt", "Retrieval attempt input must be valid JSON.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_retrieval_attempt", "Retrieval attempt input must contain one JSON object.")
			return
		}
		attempt, err := database.RecordSprintRetrievalAttempt(r.Context(), r.PathValue("sprintID"), input.Reason)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "sprint_not_found", "Sprint was not found.")
			return
		}
		if err != nil {
			writeError(w, http.StatusConflict, "invalid_retrieval_attempt", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, attempt)
	}
}

func listSprintRetrievalAttempts(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := database.ListSprintRetrievalAttempts(r.Context(), r.PathValue("sprintID"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func addSprintTimeExtension(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input model.CreateSprintTimeExtensionInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_sprint_extension", "Sprint extension input must be valid JSON.")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_sprint_extension", "Sprint extension input must contain one JSON object.")
			return
		}
		extension, err := database.AddSprintTimeExtension(r.Context(), r.PathValue("sprintID"), input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, extension)
	}
}

func listSprintTimeExtensions(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := database.ListSprintTimeExtensions(r.Context(), r.PathValue("sprintID"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func listTimeEntries(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := database.ListTimeEntries(r.Context(), r.PathValue("sprintID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "sprint_not_found", "Sprint was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_time_entries_failed", "Could not list time entries.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": entries})
	}
}

func recoverJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
		return
	}
	writeError(w, http.StatusInternalServerError, "project_export_failed", "Could not build the portable Project export.")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
