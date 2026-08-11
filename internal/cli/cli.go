// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const defaultBaseURL = "http://127.0.0.1:1618"

// ResolveBaseURL extracts an optional explicit endpoint before dispatching a CLI command.
func ResolveBaseURL(args []string, environmentURL string) (string, []string, error) {
	baseURL := strings.TrimSpace(environmentURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if len(args) == 0 {
		return baseURL, args, nil
	}
	if args[0] == "--url" {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return "", nil, fmt.Errorf("--url requires an API base URL")
		}
		return strings.TrimSpace(args[1]), args[2:], nil
	}
	if value, ok := strings.CutPrefix(args[0], "--url="); ok {
		if strings.TrimSpace(value) == "" {
			return "", nil, fmt.Errorf("--url requires an API base URL")
		}
		return strings.TrimSpace(value), args[1:], nil
	}
	return baseURL, args, nil
}

// Run executes a framework-neutral Time Keeper CLI command.
func Run(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, _ = fmt.Fprint(out, "Usage: tk [--url <api-base-url>] <command>\n\nCommands:\n  list                                        List projects\n  tree <project-id>                           Show an executable hierarchy\n  export <project-id>                         Print a portable Project snapshot as JSON\n  summary <project-id>                        Show a durable Sprint operational snapshot\n  events <project-id>                         List immutable Project activity\n  note <project-id> <content>                 Record a Project note\n  notes <project-id>                          List Project notes\n  p new <name>                                Create a Project\n  p edit <project-id> <goal> <description>    Update Project context\n  p status <project-id> <status>              Set Project status\n  c new <project-id> <name> [parent-category-id] Create a Category\n  c edit <category-id> <goal> <description>    Update Category context\n  c status <category-id> <status>              Set Category status\n  t edit <task-id> <goal> <description>        Update Task context\n  t new <project-id> <category-id> <name> <estimate>\n                                              Create a Task\n  t status <task-id> <status>                 Set Task status\n  st new <task-id> <name> <estimate>          Create a Subtask\n  st status <subtask-id> <status>              Set Subtask status\n  sp new <task|subtask> <owner-id> <name> <estimate> [buffer-percent]\n                                              Create a Sprint\n  sp <start|hold|resume|complete> <sprint-id> Transition a Sprint\n  sp extend <sprint-id> <duration> <reason>   Record justified additional planned time\n  sp extensions <sprint-id>                   List immutable extension history\n  sp entries <sprint-id>                      List recorded work/hold intervals\n  llm new <name> <provider> <base-url> <model> [system-prompt]\n                                              Configure a local planner\n  plan <generate|apply> <project-id> <id>     Generate a Review draft or apply one\n  plan list <project-id>                       List planning drafts\n  doctor                                      Check local API readiness\n\nStatuses:\n  Open | On Hold | Completed | Cancelled\n\nEnvironment:\n  TIMEKEEPER_URL  API base URL (default http://127.0.0.1:1618)\n")
		return 0
	}
	switch args[0] {
	case "plan":
		return plan(args[1:], out, errOut, baseURL)
	case "llm":
		return llmPipeline(args[1:], out, errOut, baseURL)
	case "p":
		return project(args[1:], out, errOut, baseURL)
	case "c":
		return category(args[1:], out, errOut, baseURL)
	case "t":
		return task(args[1:], out, errOut, baseURL)
	case "st":
		return subtask(args[1:], out, errOut, baseURL)
	case "sp":
		return sprint(args[1:], out, errOut, baseURL)
	case "note":
		return note(args[1:], out, errOut, baseURL)
	case "export":
		return exportProject(args[1:], out, errOut, baseURL)
	case "summary":
		return summary(args[1:], out, errOut, baseURL)
	case "events":
		return events(args[1:], out, errOut, baseURL)
	case "notes":
		return notes(args[1:], out, errOut, baseURL)
	case "list":
		return list(out, errOut, baseURL)
	case "tree":
		return tree(args[1:], out, errOut, baseURL)
	case "doctor":
		return doctor(out, errOut, baseURL)
	default:
		_, _ = fmt.Fprintf(errOut, "unknown command %q\nRun tk help for usage.\n", args[0])
		return 2
	}
}

func plan(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 2 && args[0] == "list" {
		return listPlanningDrafts(args[1], out, errOut, baseURL)
	}
	if len(args) != 3 || (args[0] != "generate" && args[0] != "apply") {
		_, _ = fmt.Fprintln(errOut, "usage: tk plan <generate|apply> <project-id> <pipeline-id|draft-id>")
		return 2
	}
	id, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil || id < 1 {
		return 2
	}
	base := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[1] + "/planning-drafts/"
	var endpoint string
	var body []byte
	if args[0] == "generate" {
		endpoint = base + "generate"
		body, _ = json.Marshal(map[string]int64{"pipeline_id": id})
	} else {
		endpoint = base + strconv.FormatInt(id, 10) + "/apply"
		body = []byte(`{}`)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	if args[0] == "apply" {
		_, _ = fmt.Fprintln(out, "applied")
		return 0
	}
	var draft struct {
		DraftID int64  `json:"draft_id"`
		Status  string `json:"status"`
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(response.Body).Decode(&draft); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%d\t%s\t%s\n", draft.DraftID, draft.Status, draft.Summary)
	return 0
}

func listPlanningDrafts(projectID string, out, errOut io.Writer, baseURL string) int {
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/planning-drafts")
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var value struct {
		Items []struct {
			DraftID int64  `json:"draft_id"`
			Status  string `json:"status"`
			Summary string `json:"summary"`
			RawJSON string `json:"raw_json"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return 1
	}
	for _, draft := range value.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\n%s\n", draft.DraftID, draft.Status, draft.Summary, draft.RawJSON)
	}
	return 0
}

func llmPipeline(args []string, out, errOut io.Writer, baseURL string) int {
	if (len(args) != 5 && len(args) != 6) || args[0] != "new" {
		_, _ = fmt.Fprintln(errOut, "usage: tk llm new <name> <ollama|openai-compatible> <base-url> <model> [system-prompt]")
		return 2
	}
	systemPrompt := ""
	if len(args) == 6 {
		systemPrompt = args[5]
	}
	body, err := json.Marshal(map[string]string{"name": args[1], "provider": args[2], "base_url": args[3], "model": args[4], "system_prompt": systemPrompt})
	if err != nil {
		return 1
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/llm-pipelines", bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var pipeline struct {
		PipelineID int64  `json:"pipeline_id"`
		Name       string `json:"name"`
		Provider   string `json:"provider"`
		Model      string `json:"model"`
	}
	if err := json.NewDecoder(response.Body).Decode(&pipeline); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%d\t%s\t%s\t%s\n", pipeline.PipelineID, pipeline.Name, pipeline.Provider, pipeline.Model)
	return 0
}

func exportProject(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk export <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/export"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var value any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "format Project export: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(out, string(encoded))
	return 0
}

func summary(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk summary <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/operational-summary"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var value struct {
		ProjectID                string `json:"project_id"`
		TotalSprints             int64  `json:"total_sprints"`
		ActiveSprints            int64  `json:"active_sprints"`
		HeldSprints              int64  `json:"held_sprints"`
		EstimatedDurationSeconds int64  `json:"estimated_duration_seconds"`
		BufferDurationSeconds    int64  `json:"buffer_duration_seconds"`
		ExtensionDurationSeconds int64  `json:"extension_duration_seconds"`
		PlannedDurationSeconds   int64  `json:"planned_duration_seconds"`
		RecordedWorkSeconds      int64  `json:"recorded_work_seconds"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s	sprints=%d	active=%d	held=%d	estimated=%ds	buffer=%ds	extensions=%ds	planned=%ds	recorded-work=%ds\n", value.ProjectID, value.TotalSprints, value.ActiveSprints, value.HeldSprints, value.EstimatedDurationSeconds, value.BufferDurationSeconds, value.ExtensionDurationSeconds, value.PlannedDurationSeconds, value.RecordedWorkSeconds)
	return 0
}

func events(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk events <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/events"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Items []struct {
			EventID   int64  `json:"event_id"`
			EventType string `json:"event_type"`
			EntityID  string `json:"entity_id"`
			Message   string `json:"message"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	for _, event := range payload.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\t%s\n", event.EventID, event.EventType, event.EntityID, event.Message)
	}
	return 0
}

func notes(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk notes <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/notes"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Items []struct {
			NoteID    int64  `json:"note_id"`
			ProjectID string `json:"project_id"`
			Content   string `json:"content"`
			ActorID   string `json:"actor_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	for _, note := range payload.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\t%s\n", note.NoteID, note.ProjectID, note.ActorID, note.Content)
	}
	return 0
}

func note(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk note <project-id> <content>")
		return 2
	}
	body, err := json.Marshal(map[string]string{"content": args[1]})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode project note: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/notes"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create project note request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var note struct {
		NoteID    int64  `json:"note_id"`
		ProjectID string `json:"project_id"`
		Content   string `json:"content"`
	}
	if err := json.NewDecoder(response.Body).Decode(&note); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%d\t%s\t%s\n", note.NoteID, note.ProjectID, note.Content)
	return 0
}

func sprintAction(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp <start|hold|resume|complete> <sprint-id>")
		return 2
	}
	action := args[0]
	switch action {
	case "start", "hold", "resume", "complete":
	default:
		_, _ = fmt.Fprintf(errOut, "unknown sprint action %q\n", action)
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + args[1] + "/" + action
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create sprint action request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var sprint struct {
		SprintID    string `json:"sprint_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sprint); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", sprint.SprintID, sprint.Status, sprint.Name, sprint.ItemAddress)
	return 0
}

func sprintEntries(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp entries <sprint-id>")
		return 2
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + args[1] + "/time-entries")
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var value struct {
		Items []struct {
			TimeEntryID     int64  `json:"time_entry_id"`
			SprintID        string `json:"sprint_id"`
			EntryType       string `json:"entry_type"`
			DurationSeconds int64  `json:"duration_seconds"`
			Reason          string `json:"reason"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return 1
	}
	for _, entry := range value.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%s\t%d\t%s\n", entry.TimeEntryID, entry.SprintID, entry.EntryType, entry.DurationSeconds, entry.Reason)
	}
	return 0
}

func sprintExtensions(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp extensions <sprint-id>")
		return 2
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(strings.TrimRight(baseURL, "/") + "/api/v1/sprints/" + args[1] + "/extensions")
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var value struct {
		Items []struct {
			ExtensionID     int64  `json:"extension_id"`
			SprintID        string `json:"sprint_id"`
			DurationSeconds int64  `json:"duration_seconds"`
			Reason          string `json:"reason"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return 1
	}
	for _, extension := range value.Items {
		_, _ = fmt.Fprintf(out, "%d\t%s\t%d\t%s\n", extension.ExtensionID, extension.SprintID, extension.DurationSeconds, extension.Reason)
	}
	return 0
}

func sprintExtend(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 4 || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[3]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp extend <sprint-id> <duration> <reason>")
		return 2
	}
	duration, err := time.ParseDuration(args[2])
	if err != nil || duration <= 0 {
		_, _ = fmt.Fprintln(errOut, "extension duration must be positive, such as 10m")
		return 2
	}
	body, err := json.Marshal(map[string]any{"duration_seconds": int64(duration.Seconds()), "reason": args[3]})
	if err != nil {
		return 1
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/sprints/"+args[1]+"/extensions", bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var extension struct {
		ExtensionID     int64  `json:"extension_id"`
		SprintID        string `json:"sprint_id"`
		DurationSeconds int64  `json:"duration_seconds"`
		Reason          string `json:"reason"`
	}
	if err := json.NewDecoder(response.Body).Decode(&extension); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%d\t%s\t%d\t%s\n", extension.ExtensionID, extension.SprintID, extension.DurationSeconds, extension.Reason)
	return 0
}

func sprint(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) > 0 && args[0] == "entries" {
		return sprintEntries(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "extensions" {
		return sprintExtensions(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] == "extend" {
		return sprintExtend(args, out, errOut, baseURL)
	}
	if len(args) > 0 && args[0] != "new" {
		return sprintAction(args, out, errOut, baseURL)
	}
	if (len(args) != 5 && len(args) != 6) || args[0] != "new" || (args[1] != "task" && args[1] != "subtask") || strings.TrimSpace(args[2]) == "" || strings.TrimSpace(args[3]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk sp new <task|subtask> <owner-id> <sprint-name> <estimate> [buffer-percent]")
		return 2
	}
	estimate, err := time.ParseDuration(args[4])
	if err != nil || estimate <= 0 {
		_, _ = fmt.Fprintln(errOut, "estimate must be a positive duration such as 30m or 2h")
		return 2
	}
	bufferPct := int64(0)
	if len(args) == 6 {
		bufferPct, err = strconv.ParseInt(args[5], 10, 64)
		if err != nil || bufferPct < 0 || bufferPct > 100 {
			_, _ = fmt.Fprintln(errOut, "buffer percent must be a whole number from 0 to 100")
			return 2
		}
	}
	body, err := json.Marshal(map[string]any{"name": args[3], "estimated_duration_seconds": int64(estimate.Seconds()), "buffer_pct": bufferPct})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode sprint input: %v\n", err)
		return 1
	}
	ownerPath := "tasks"
	if args[1] == "subtask" {
		ownerPath = "subtasks"
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/" + ownerPath + "/" + args[2] + "/sprints"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create sprint request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var sprint struct {
		SprintID    string `json:"sprint_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sprint); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", sprint.SprintID, sprint.Status, sprint.Name, sprint.ItemAddress)
	return 0
}

func subtask(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 3 && args[0] == "status" {
		return subtaskStatus(args[1], args[2], out, errOut, baseURL)
	}
	if len(args) != 4 || args[0] != "new" || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk st new <task-id> <subtask-name> <estimate>")
		return 2
	}
	estimate, err := time.ParseDuration(args[3])
	if err != nil || estimate <= 0 {
		_, _ = fmt.Fprintln(errOut, "estimate must be a positive duration such as 30m or 2h")
		return 2
	}
	body, err := json.Marshal(map[string]any{"name": args[2], "estimated_duration_seconds": int64(estimate.Seconds())})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode subtask input: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + args[1] + "/subtasks"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create subtask request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var subtask struct {
		SubtaskID   string `json:"subtask_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.NewDecoder(response.Body).Decode(&subtask); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", subtask.SubtaskID, subtask.Name, subtask.ItemAddress)
	return 0
}

func task(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 4 && args[0] == "edit" && strings.TrimSpace(args[1]) != "" {
		return taskEdit(args[1], args[2], args[3], out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "status" {
		return taskStatus(args[1], args[2], out, errOut, baseURL)
	}
	if len(args) != 5 || args[0] != "new" || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" || strings.TrimSpace(args[3]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk t new <project-id> <category-id> <task-name> <estimate>")
		return 2
	}
	estimate, err := time.ParseDuration(args[4])
	if err != nil || estimate <= 0 {
		_, _ = fmt.Fprintln(errOut, "estimate must be a positive duration such as 30m or 2h")
		return 2
	}
	body, err := json.Marshal(map[string]any{"category_id": args[2], "name": args[3], "estimated_duration_seconds": int64(estimate.Seconds())})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode task input: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[1] + "/tasks"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create task request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var task struct {
		TaskID      string `json:"task_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", task.TaskID, task.Name, task.ItemAddress)
	return 0
}

func subtaskStatus(subtaskID, status string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"status": status})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/subtasks/" + subtaskID + "/status"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var subtask struct {
		SubtaskID string `json:"subtask_id"`
		Status    string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&subtask); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\n", subtask.SubtaskID, subtask.Status)
	return 0
}

func taskEdit(taskID, goal, description string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"goal": goal, "description": description})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + taskID + "/metadata"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var task struct {
		TaskID      string `json:"task_id"`
		Goal        string `json:"goal"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", task.TaskID, task.Goal, task.Description)
	return 0
}

func taskStatus(taskID, status string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"status": status})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + taskID + "/status"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var task struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\n", task.TaskID, task.Status)
	return 0
}

func categoryEdit(categoryID, goal, description string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"goal": goal, "description": description})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/categories/" + categoryID + "/metadata"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	var category struct {
		CategoryID string `json:"category_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&category); err != nil {
		return 1
	}
	_, _ = fmt.Fprintln(out, category.CategoryID)
	return 0
}

func categoryStatus(categoryID, status string, out, errOut io.Writer, baseURL string) int {
	body, _ := json.Marshal(map[string]string{"status": status})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/categories/" + categoryID + "/status"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create category status request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var category struct {
		CategoryID string `json:"category_id"`
		Status     string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&category); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\n", category.CategoryID, category.Status)
	return 0
}

func category(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 4 && args[0] == "edit" && strings.TrimSpace(args[1]) != "" {
		return categoryEdit(args[1], args[2], args[3], out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "status" && strings.TrimSpace(args[1]) != "" && strings.TrimSpace(args[2]) != "" {
		return categoryStatus(args[1], args[2], out, errOut, baseURL)
	}
	if (len(args) != 3 && len(args) != 4) || args[0] != "new" || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk c new <project-id> <category-name> [parent-category-id]")
		return 2
	}
	input := map[string]string{"name": args[2]}
	if len(args) == 4 && strings.TrimSpace(args[3]) != "" {
		input["parent_category_id"] = args[3]
	}
	body, err := json.Marshal(input)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode category input: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[1] + "/categories"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create category request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var category struct {
		CategoryID  string `json:"category_id"`
		Name        string `json:"name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.NewDecoder(response.Body).Decode(&category); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", category.CategoryID, category.Name, category.ItemAddress)
	return 0
}

func project(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 4 && args[0] == "edit" && strings.TrimSpace(args[1]) != "" {
		return projectEdit(args[1], args[2], args[3], out, errOut, baseURL)
	}
	if len(args) == 3 && args[0] == "status" && strings.TrimSpace(args[1]) != "" && strings.TrimSpace(args[2]) != "" {
		return projectStatus(args[1], args[2], out, errOut, baseURL)
	}
	if len(args) != 2 || args[0] != "new" || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk p new <project-name>")
		return 2
	}
	body, err := json.Marshal(map[string]string{"name": args[1]})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode project input: %v\n", err)
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create project request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var project struct {
		ProjectID   string `json:"project_id"`
		ProjectName string `json:"project_name"`
		ItemAddress string `json:"item_address"`
	}
	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", project.ProjectID, project.ProjectName, project.ItemAddress)
	return 0
}

func projectStatus(projectID, status string, out, errOut io.Writer, baseURL string) int {
	body, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/status"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "update Project status request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var project struct {
		ProjectID string `json:"project_id"`
		Status    string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\n", project.ProjectID, project.Status)
	return 0
}

func projectEdit(projectID, goal, description string, out, errOut io.Writer, baseURL string) int {
	body, err := json.Marshal(map[string]string{"goal": goal, "description": description})
	if err != nil {
		return 1
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/metadata"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "update Project metadata request: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var project struct {
		ProjectID   string `json:"project_id"`
		Goal        string `json:"project_goal"`
		Description string `json:"project_description"`
	}
	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", project.ProjectID, project.Goal, project.Description)
	return 0
}

func doctor(out, errOut io.Writer, baseURL string) int {
	url := strings.TrimRight(baseURL, "/") + "/health"
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "[failed] API unreachable: %s (%v)\nStart Time Keeper, then run tk doctor again.\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "[failed] health endpoint returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.Status != "ok" {
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "[failed] health endpoint returned invalid JSON: %v\n", err)
		} else {
			_, _ = fmt.Fprintf(errOut, "[failed] health endpoint reported status %q\n", payload.Status)
		}
		return 1
	}
	_, _ = fmt.Fprintf(out, "Time Keeper doctor\n\n[ok] API reachable: %s\n[ok] health status: %s\n\nTime Keeper is ready.\n", baseURL, payload.Status)
	return 0
}

func tree(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk tree <project-id>")
		return 2
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + args[0] + "/execution-tree"
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload model.ExecutionTree
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s	%.1f%%	%s\n", payload.Project.ProjectID, payload.Project.CalculatedCompletionPct, payload.Project.ProjectName)
	for _, category := range payload.Categories {
		writeCategoryTree(out, category, 1)
	}
	return 0
}

func writeCategoryTree(out io.Writer, category model.ExecutionCategory, depth int) {
	indent := strings.Repeat("  ", depth)
	_, _ = fmt.Fprintf(out, "%s%s	%s\n", indent, category.Category.CategoryID, category.Category.Name)
	for _, child := range category.Categories {
		writeCategoryTree(out, child, depth+1)
	}
	for _, task := range category.Tasks {
		_, _ = fmt.Fprintf(out, "%s  %s	%s	%s\n", indent, task.Task.TaskID, task.Task.Status, task.Task.Name)
		for _, sprint := range task.Sprints {
			_, _ = fmt.Fprintf(out, "%s    %s	%s	%s\n", indent, sprint.SprintID, sprint.Status, sprint.Name)
		}
		for _, subtask := range task.Subtasks {
			_, _ = fmt.Fprintf(out, "%s    %s	%s	%s\n", indent, subtask.Subtask.SubtaskID, subtask.Subtask.Status, subtask.Subtask.Name)
			for _, sprint := range subtask.Sprints {
				_, _ = fmt.Fprintf(out, "%s      %s	%s	%s\n", indent, sprint.SprintID, sprint.Status, sprint.Name)
			}
		}
	}
}

func list(out, errOut io.Writer, baseURL string) int {
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects"
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned %s\n", response.Status)
		return 1
	}
	var payload struct {
		Items []struct {
			ProjectID               string  `json:"project_id"`
			ProjectName             string  `json:"project_name"`
			Status                  string  `json:"status"`
			CalculatedCompletionPct float64 `json:"calculated_completion_pct"`
			ItemAddress             string  `json:"item_address"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}
	for _, project := range payload.Items {
		_, _ = fmt.Fprintf(out, "%s	%s	%.1f%%	%s	%s\n", project.ProjectID, project.Status, project.CalculatedCompletionPct, project.ProjectName, project.ItemAddress)
	}
	return 0
}
