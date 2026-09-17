package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func createBugReport(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		input, err := decodeBugReportInput(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_bug_report", err.Error())
			return
		}
		result, err := database.CreateBugReport(r.Context(), input)
		if errors.Is(err, store.ErrBugReportConflict) {
			writeError(w, http.StatusConflict, "bug_report_duplicate", err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_bug_report", err.Error())
			return
		}
		status := http.StatusCreated
		if result.Duplicate {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]any{
			"report_id":  result.Report.ReportID,
			"status":     result.Report.Status,
			"project_id": result.Report.ProjectID,
			"task_id":    result.Report.TaskID,
			"duplicate":  result.Duplicate,
		})
	}
}

func decodeBugReportInput(w http.ResponseWriter, r *http.Request) (model.CreateBugReportInput, error) {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if mediaType == "text/markdown" {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			return model.CreateBugReportInput{}, errors.New("Markdown bug report could not be read")
		}
		return parseMarkdownBugReport(string(body))
	}
	var input model.CreateBugReportInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return model.CreateBugReportInput{}, errors.New("Bug report input must be valid JSON")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return model.CreateBugReportInput{}, errors.New("Bug report input must contain one JSON object")
	}
	return input, nil
}

func parseMarkdownBugReport(raw string) (model.CreateBugReportInput, error) {
	if strings.Contains(raw, "-----BEGIN") || strings.Contains(strings.ToLower(raw), "x-api-key:") || strings.Contains(strings.ToLower(raw), "authorization: bearer") || strings.Contains(strings.ToLower(raw), "password=") {
		return model.CreateBugReportInput{}, errors.New("Markdown report contains sensitive material")
	}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return model.CreateBugReportInput{}, errors.New("Markdown report must start with frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return model.CreateBugReportInput{}, errors.New("Markdown report frontmatter is not closed")
	}
	input := model.CreateBugReportInput{Environment: map[string]string{}, Source: model.BugReportSource{}}
	for _, line := range lines[1:end] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return model.CreateBugReportInput{}, errors.New("Markdown frontmatter must use key: value lines")
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch {
		case key == "project_id":
			input.ProjectID = value
		case key == "category_id":
			input.CategoryID = value
		case key == "task_id":
			input.TaskID = value
		case key == "sprint_id":
			input.SprintID = value
		case key == "severity":
			input.Severity = value
		case key == "idempotency_key":
			input.IdempotencyKey = value
		case key == "dedupe_key":
			input.DedupeKey = value
		case key == "redaction_status":
			input.RedactionStatus = value
		case key == "source.agent_id":
			input.Source.AgentID = value
		case key == "source.session_id":
			input.Source.SessionID = value
		case key == "source.command":
			input.Source.Command = value
		case strings.HasPrefix(key, "environment."):
			input.Environment[strings.TrimPrefix(key, "environment.")] = value
		default:
			return model.CreateBugReportInput{}, fmt.Errorf("unknown Markdown frontmatter field %q", key)
		}
	}
	section := ""
	var sectionLines []string
	flush := func() {
		value := strings.TrimSpace(strings.Join(sectionLines, "\n"))
		switch section {
		case "title":
			input.Title = strings.TrimSpace(strings.TrimPrefix(value, "# "))
		case "expected":
			input.Expected = value
		case "actual":
			input.Actual = value
		case "reproduction":
			for _, line := range sectionLines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "-") {
					input.ReproductionSteps = append(input.ReproductionSteps, strings.TrimSpace(strings.TrimPrefix(line, "-")))
				}
			}
		case "environment":
			for _, line := range sectionLines {
				parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(line, "-")), ":", 2)
				if len(parts) == 2 {
					input.Environment[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}
			}
		case "evidence":
			for _, line := range sectionLines {
				parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(line, "-")), ":", 2)
				if len(parts) == 2 {
					input.Evidence = append(input.Evidence, model.BugReportEvidence{Type: strings.TrimSpace(parts[0]), Ref: strings.TrimSpace(parts[1])})
				}
			}
		}
		sectionLines = nil
	}
	for _, line := range lines[end+1:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			flush()
			section = "title"
			sectionLines = []string{trimmed}
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			flush()
			section = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
			sectionLines = nil
			continue
		}
		sectionLines = append(sectionLines, line)
	}
	flush()
	return input, nil
}

func listBugReports(database *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := database.ListBugReports(r.Context(), r.PathValue("projectID"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "project_not_found", "Project was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "list_bug_reports_failed", "Could not list bug reports.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}
