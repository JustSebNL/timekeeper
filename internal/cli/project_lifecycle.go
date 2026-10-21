// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func projectLifecycleCommand(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		return projectLifecycleUsage(errOut)
	}
	switch args[0] {
	case "schedule":
		return projectScheduleCLI(args[1:], out, errOut, baseURL)
	case "phase":
		return projectPhaseCLI(args[1:], out, errOut, baseURL)
	case "milestones":
		return projectMilestonesCLI(args[1:], out, errOut, baseURL)
	case "deadlines":
		return projectDeadlinesCLI(args[1:], out, errOut, baseURL)
	default:
		return projectLifecycleUsage(errOut)
	}
}

func projectScheduleCLI(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 2 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk p schedule <project-id> start=<YYYY-MM-DD|clear> due=<YYYY-MM-DD|clear> timezone=<IANA-zone> warn=<0..365>")
		return 2
	}
	payload := make(map[string]any)
	for _, assignment := range args[1:] {
		key, value, ok := strings.Cut(assignment, "=")
		if !ok {
			_, _ = fmt.Fprintf(errOut, "invalid schedule field %q; use key=value\n", assignment)
			return 2
		}
		switch key {
		case "start":
			if value == "clear" {
				value = ""
			}
			payload["planned_start_date"] = value
		case "due":
			if value == "clear" {
				value = ""
			}
			payload["target_end_date"] = value
		case "timezone":
			payload["schedule_timezone"] = value
		case "warn":
			days, err := strconv.Atoi(value)
			if err != nil || days < 0 || days > 365 {
				_, _ = fmt.Fprintln(errOut, "warn must be an integer from 0 through 365")
				return 2
			}
			payload["deadline_warning_days"] = days
		default:
			_, _ = fmt.Fprintf(errOut, "unknown schedule field %q\n", key)
			return 2
		}
	}
	var project model.Project
	if !projectLifecycleRequest(http.MethodPatch, "/api/v1/projects/"+url.PathEscape(args[0])+"/schedule", payload, http.StatusOK, &project, "update Project schedule", out, errOut, baseURL) {
		return 1
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", project.ProjectID, project.PlannedStartDate, project.TargetEndDate, project.DeadlineState)
	return 0
}

func projectPhaseCLI(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk p phase <project-id> <intake|planning|execution|validation|closeout> [reason]")
		return 2
	}
	payload := map[string]string{"phase": args[1], "reason": strings.TrimSpace(strings.Join(args[2:], " "))}
	var project model.Project
	if !projectLifecycleRequest(http.MethodPost, "/api/v1/projects/"+url.PathEscape(args[0])+"/phase", payload, http.StatusOK, &project, "transition Project phase", out, errOut, baseURL) {
		return 1
	}
	started := ""
	if project.ActualStartedAt != nil {
		started = project.ActualStartedAt.UTC().Format(time.RFC3339Nano)
	}
	_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", project.ProjectID, project.LifecyclePhase, started)
	return 0
}

func projectMilestonesCLI(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
		return projectLifecycleUsage(errOut)
	}
	action, projectID := args[0], args[1]
	root := "/api/v1/projects/" + url.PathEscape(projectID) + "/milestones"
	switch action {
	case "list":
		if len(args) != 2 {
			return projectLifecycleUsage(errOut)
		}
		var milestones []model.ProjectMilestone
		if !projectLifecycleRequest(http.MethodGet, root, nil, http.StatusOK, &milestones, "list Project milestones", out, errOut, baseURL) {
			return 1
		}
		for _, milestone := range milestones {
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\n", milestone.MilestoneID, milestone.TargetDate, milestone.Status, milestone.DeadlineState, milestone.LinkedTaskID, milestone.Name)
		}
		return 0
	case "add":
		if len(args) < 4 {
			return projectLifecycleUsage(errOut)
		}
		nameParts := args[3:]
		taskID := ""
		if len(nameParts) > 0 && strings.HasPrefix(nameParts[len(nameParts)-1], "task=") {
			taskID = strings.TrimPrefix(nameParts[len(nameParts)-1], "task=")
			nameParts = nameParts[:len(nameParts)-1]
		}
		name := strings.TrimSpace(strings.Join(nameParts, " "))
		if name == "" {
			return projectLifecycleUsage(errOut)
		}
		payload := model.CreateProjectMilestoneInput{Name: name, TargetDate: args[2], LinkedTaskID: taskID}
		var milestone model.ProjectMilestone
		if !projectLifecycleRequest(http.MethodPost, root, payload, http.StatusCreated, &milestone, "create Project milestone", out, errOut, baseURL) {
			return 1
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", milestone.MilestoneID, milestone.TargetDate, milestone.DeadlineState, milestone.Name)
		return 0
	case "date":
		if len(args) != 4 {
			return projectLifecycleUsage(errOut)
		}
		payload := map[string]string{"target_date": args[3]}
		var milestone model.ProjectMilestone
		path := root + "/" + url.PathEscape(args[2])
		if !projectLifecycleRequest(http.MethodPatch, path, payload, http.StatusOK, &milestone, "update milestone date", out, errOut, baseURL) {
			return 1
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", milestone.MilestoneID, milestone.TargetDate, milestone.DeadlineState, milestone.Name)
		return 0
	case "status":
		if len(args) < 4 {
			return projectLifecycleUsage(errOut)
		}
		payload := model.UpdateProjectMilestoneStatusInput{Status: model.ProjectMilestoneStatus(args[3]), Reason: strings.TrimSpace(strings.Join(args[4:], " "))}
		var milestone model.ProjectMilestone
		path := root + "/" + url.PathEscape(args[2]) + "/status"
		if !projectLifecycleRequest(http.MethodPost, path, payload, http.StatusOK, &milestone, "update milestone status", out, errOut, baseURL) {
			return 1
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", milestone.MilestoneID, milestone.Status, milestone.DeadlineState)
		return 0
	default:
		return projectLifecycleUsage(errOut)
	}
}

func projectDeadlinesCLI(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(errOut, "usage: tk p deadlines")
		return 2
	}
	var watch model.ProjectDeadlineWatch
	if !projectLifecycleRequest(http.MethodGet, "/api/v1/project-deadlines", nil, http.StatusOK, &watch, "list Project deadlines", out, errOut, baseURL) {
		return 1
	}
	for _, item := range watch.Items {
		days := ""
		if item.DaysToDeadline != nil {
			days = strconv.Itoa(*item.DaysToDeadline)
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", item.DeadlineState, item.TargetDate, item.ProjectName, item.EntityType, item.EntityID, item.Name, item.Status, days)
	}
	return 0
}

func projectLifecycleRequest(method, path string, payload any, expectedStatus int, result any, operation string, out, errOut io.Writer, baseURL string) bool {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "encode %s input: %v\n", operation, err)
			return false
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, strings.TrimRight(baseURL, "/")+path, body)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "%s request: %v\n", operation, err)
		return false
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", request.URL, err)
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		reportHTTPStatus(errOut, operation, response)
		return false
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return false
	}
	return true
}

func projectLifecycleUsage(errOut io.Writer) int {
	_, _ = fmt.Fprintln(errOut, "usage: tk p schedule <project-id> start=<YYYY-MM-DD|clear> due=<YYYY-MM-DD|clear> timezone=<zone> warn=<0..365>")
	_, _ = fmt.Fprintln(errOut, "       tk p phase <project-id> <phase> [reason]")
	_, _ = fmt.Fprintln(errOut, "       tk p milestones list <project-id>")
	_, _ = fmt.Fprintln(errOut, "       tk p milestones add <project-id> <YYYY-MM-DD> <name...> [task=<task-id>]")
	_, _ = fmt.Fprintln(errOut, "       tk p milestones date <project-id> <milestone-id> <YYYY-MM-DD>")
	_, _ = fmt.Fprintln(errOut, "       tk p milestones status <project-id> <milestone-id> <status> [reason]")
	return 2
}
