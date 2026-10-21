// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

type ExecutionTree = model.ExecutionTree
type ExecutionCategory = model.ExecutionCategory
type ExecutionTask = model.ExecutionTask
type ExecutionSubtask = model.ExecutionSubtask

type ProjectExportResponse struct {
	Format        string        `json:"format"`
	ProjectID     string        `json:"project_id"`
	ExecutionTree ExecutionTree `json:"execution_tree"`
}

func projectExportMulti(args []string, out, errOut io.Writer, baseURL string) int {
	var positional []string
	var format, taskID string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format", "--task":
			flag := args[i]
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				_, _ = fmt.Fprintf(errOut, "%s requires a value\n", flag)
				return 2
			}
			i++
			if flag == "--format" {
				format = args[i]
			} else {
				if taskID != "" {
					_, _ = fmt.Fprintln(errOut, "--task may be specified once")
					return 2
				}
				taskID = strings.TrimSpace(args[i])
			}
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				_, _ = fmt.Fprintf(errOut, "unknown export option %q\n", args[i])
				return 2
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 1 || len(positional) > 2 || strings.TrimSpace(positional[0]) == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk export <project-id> [file] [--task <T-ID>] [--format F]")
		return 2
	}
	projectID := strings.TrimSpace(positional[0])
	filePath := "-"
	if len(positional) == 2 {
		filePath = positional[1]
	}
	if len(taskID) > 128 {
		_, _ = fmt.Fprintln(errOut, "--task ID is too long")
		return 2
	}

	if format == "" {
		if filePath == "-" {
			format = "json"
		} else {
			lowPath := strings.ToLower(filePath)
			if strings.HasSuffix(lowPath, ".tar.gz") {
				format = "tar.gz"
			} else {
				ext := filepath.Ext(lowPath)
				switch ext {
				case ".csv":
					format = "csv"
				case ".tsv":
					format = "tsv"
				case ".yaml", ".yml":
					format = "yaml"
				case ".json":
					format = "json"
				case ".xlsx":
					format = "xlsx"
				case ".sql":
					format = "sql"
				case ".zip":
					format = "zip"
				case ".tar":
					format = "tar"
				case ".tgz":
					format = "tar.gz"
				case ".gz":
					format = "gz"
				case ".rar":
					format = "rar"
				default:
					_, _ = fmt.Fprintf(errOut, "unknown file extension %q for project export\n", ext)
					return 2
				}
			}
		}
	} else {
		format = strings.ToLower(format)
		if format == "tgz" {
			format = "tar.gz"
		}
	}

	isZip := format == "zip" || filepath.Ext(filePath) == ".zip"
	if isZip && format == "zip" {
		format = "csv" // default to CSV in zip if only extension is provided and no format flag
	}

	exportURL := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/export"
	if taskID != "" {
		query := url.Values{}
		query.Set("task_id", taskID)
		exportURL += "?" + query.Encode()
	}
	resp, err := newHTTPClient(10 * time.Second).Get(exportURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", exportURL, err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "export project", resp)
		return 1
	}

	bodyData, err := io.ReadAll(resp.Body)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "read response body: %v\n", err)
		return 1
	}

	var data ProjectExportResponse
	if err := json.Unmarshal(bodyData, &data); err != nil {
		_, _ = fmt.Fprintf(errOut, "decode project export: %v\n", err)
		return 1
	}
	if taskID != "" {
		filteredTree, found := model.FilterExecutionTree(data.ExecutionTree, taskID, "")
		if !found {
			_, _ = fmt.Fprintf(errOut, "Task %s was not found in Project %s.\n", taskID, projectID)
			return 1
		}
		data.ExecutionTree = filteredTree
		filteredExport := struct {
			Format        string              `json:"format"`
			ProjectID     string              `json:"project_id"`
			ExecutionTree model.ExecutionTree `json:"execution_tree"`
			FilteredBy    struct {
				TaskID string `json:"task_id"`
			} `json:"filtered_by"`
		}{
			Format: data.Format, ProjectID: data.ProjectID, ExecutionTree: data.ExecutionTree,
			FilteredBy: struct {
				TaskID string `json:"task_id"`
			}{TaskID: taskID},
		}
		bodyData, err = json.Marshal(filteredExport)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "format filtered Project export: %v\n", err)
			return 1
		}
	}

	// Flattening
	var projects []model.Project
	var categories []model.Category
	var tasks []model.Task
	var subtasks []model.Subtask
	var sprints []model.Sprint

	projects = append(projects, data.ExecutionTree.Project)

	var walkCategory func(c ExecutionCategory)
	walkCategory = func(c ExecutionCategory) {
		categories = append(categories, c.Category)
		for _, sub := range c.Categories {
			walkCategory(sub)
		}
		for _, t := range c.Tasks {
			tasks = append(tasks, t.Task)
			sprints = append(sprints, t.Sprints...)
			for _, st := range t.Subtasks {
				subtasks = append(subtasks, st.Subtask)
				sprints = append(sprints, st.Sprints...)
			}
		}
	}
	for _, c := range data.ExecutionTree.Categories {
		walkCategory(c)
	}

	var writer io.Writer
	if filePath == "-" {
		writer = out
	} else {
		if format == "csv" {
			filePath = strings.TrimSuffix(filePath, ".csv") + ".zip"
		} else if format == "tsv" {
			filePath = strings.TrimSuffix(filePath, ".tsv") + ".zip"
		}
		f, err := os.Create(filePath)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "create file %q: %v\n", filePath, err)
			return 1
		}
		defer f.Close()
		writer = f
	}

	switch format {
	case "yaml":
		var fullMap map[string]any
		_ = json.Unmarshal(bodyData, &fullMap)
		enc := yaml.NewEncoder(writer)
		if err := enc.Encode(fullMap); err != nil {
			_, _ = fmt.Fprintf(errOut, "encode yaml: %v\n", err)
			return 1
		}
		enc.Close()

	case "json":
		var fullMap map[string]any
		_ = json.Unmarshal(bodyData, &fullMap)
		enc := json.NewEncoder(writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(fullMap); err != nil {
			_, _ = fmt.Fprintf(errOut, "encode json: %v\n", err)
			return 1
		}

	case "xlsx":
		f := excelize.NewFile()
		defer f.Close()

		boldStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})

		writeSheet := func(sheetName string, headers []string, rows [][]any) {
			f.NewSheet(sheetName)
			_ = f.SetRowStyle(sheetName, 1, 1, boldStyle)
			for colIdx, h := range headers {
				cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
				f.SetCellValue(sheetName, cell, h)
			}
			for rowIdx, row := range rows {
				for colIdx, val := range row {
					cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
					f.SetCellValue(sheetName, cell, val)
				}
			}
			for colIdx := range headers {
				colName, _ := excelize.ColumnNumberToName(colIdx + 1)
				_ = f.SetColWidth(sheetName, colName, colName, 20)
			}
		}

		writeSheet("Project", []string{"project_id", "name", "goal", "status", "created_at"}, [][]any{{data.ExecutionTree.Project.ProjectID, data.ExecutionTree.Project.ProjectName, data.ExecutionTree.Project.ProjectGoal, data.ExecutionTree.Project.Status, data.ExecutionTree.Project.CreatedAt.Format(time.RFC3339)}})

		var catRows [][]any
		for _, c := range categories {
			catRows = append(catRows, []any{c.CategoryID, c.ProjectID, c.ParentCategoryID, c.Name, c.Description, c.Goal, c.Status, c.CreatedAt.Format(time.RFC3339)})
		}
		writeSheet("Categories", []string{"category_id", "project_id", "parent_category_id", "name", "description", "goal", "status", "created_at"}, catRows)

		var taskRows [][]any
		for _, t := range tasks {
			taskRows = append(taskRows, []any{t.TaskID, t.CategoryID, t.Name, t.Description, t.Goal, t.Status, t.Priority, t.EstimatedDurationSeconds, t.CalculatedCompletionPct, t.AutoClose, t.CreatedAt.Format(time.RFC3339)})
		}
		writeSheet("Tasks", []string{"task_id", "category_id", "name", "description", "goal", "status", "priority", "estimate_seconds", "completion_pct", "auto_close", "created_at"}, taskRows)

		var subtaskRows [][]any
		for _, st := range subtasks {
			subtaskRows = append(subtaskRows, []any{st.SubtaskID, st.TaskID, st.Name, st.Status, st.EstimatedDurationSeconds, st.CalculatedCompletionPct, st.CreatedAt.Format(time.RFC3339)})
		}
		writeSheet("Subtasks", []string{"subtask_id", "task_id", "name", "status", "estimate_seconds", "completion_pct", "created_at"}, subtaskRows)

		var sprintRows [][]any
		for _, sp := range sprints {
			owner := sp.TaskID
			if sp.SubtaskID != "" {
				owner = sp.SubtaskID
			}
			sprintRows = append(sprintRows, []any{sp.SprintID, owner, sp.Name, sp.Status, sp.EstimatedDurationSeconds, sp.CreatedAt.Format(time.RFC3339)})
		}
		writeSheet("Sprints", []string{"sprint_id", "owner_id", "name", "status", "estimate_seconds", "created_at"}, sprintRows)

		f.DeleteSheet("Sheet1")

		if filePath == "-" {
			_ = f.Write(writer)
		} else {
			_ = f.SaveAs(filePath)
		}

	case "rar":
		_, _ = fmt.Fprintf(errOut, "rar export is not supported\n")
		return 1

	case "gz":
		// Not really supporting raw .gz containing multiple files unless it's tar.gz
		// Fall through to error or we could tar.gz it. Let's just error
		_, _ = fmt.Fprintf(errOut, ".gz export without tar is not supported for full projects\n")
		return 1

	case "csv", "tsv", "tar", "tar.gz":
		var zipWriter *zip.Writer
		var tarWriter *tar.Writer
		var gw *gzip.Writer

		if format == "tar" || format == "tar.gz" {
			if format == "tar.gz" {
				gw = gzip.NewWriter(writer)
				tarWriter = tar.NewWriter(gw)
			} else {
				tarWriter = tar.NewWriter(writer)
			}
		} else {
			zipWriter = zip.NewWriter(writer)
		}

		writeCSVFile := func(filename string, headers []string, rows [][]string) error {
			var w *csv.Writer
			var buf bytes.Buffer

			if zipWriter != nil {
				f, err := zipWriter.Create(filename)
				if err != nil {
					return err
				}
				w = csv.NewWriter(f)
				if format == "tsv" {
					w.Comma = '\t'
				}
				_ = w.Write(headers)
				for _, r := range rows {
					_ = w.Write(r)
				}
				w.Flush()
				return w.Error()
			} else {
				w = csv.NewWriter(&buf)
				_ = w.Write(headers)
				for _, r := range rows {
					_ = w.Write(r)
				}
				w.Flush()
				if err := w.Error(); err != nil {
					return err
				}

				hdr := &tar.Header{
					Name:    filename,
					Mode:    0600,
					Size:    int64(buf.Len()),
					ModTime: time.Now(),
				}
				if err := tarWriter.WriteHeader(hdr); err != nil {
					return err
				}
				if _, err := tarWriter.Write(buf.Bytes()); err != nil {
					return err
				}
				return nil
			}
		}
		ext := ".csv"
		if format == "tsv" {
			ext = ".tsv"
		}

		var catRows [][]string
		for _, c := range categories {
			catRows = append(catRows, []string{c.CategoryID, c.ProjectID, c.ParentCategoryID, c.Name, c.Description, c.Goal, c.Status, c.CreatedAt.Format(time.RFC3339)})
		}
		_ = writeCSVFile("categories"+ext, []string{"category_id", "project_id", "parent_category_id", "name", "description", "goal", "status", "created_at"}, catRows)

		var taskRows [][]string
		for _, t := range tasks {
			taskRows = append(taskRows, []string{t.TaskID, t.CategoryID, t.Name, t.Description, t.Goal, t.Status, t.Priority, strconv.FormatInt(t.EstimatedDurationSeconds, 10), fmt.Sprintf("%.2f", t.CalculatedCompletionPct), strconv.FormatBool(t.AutoClose), t.CreatedAt.Format(time.RFC3339)})
		}
		_ = writeCSVFile("tasks"+ext, []string{"task_id", "category_id", "name", "description", "goal", "status", "priority", "estimate_seconds", "completion_pct", "auto_close", "created_at"}, taskRows)

		var subtaskRows [][]string
		for _, st := range subtasks {
			subtaskRows = append(subtaskRows, []string{st.SubtaskID, st.TaskID, st.Name, st.Status, strconv.FormatInt(st.EstimatedDurationSeconds, 10), fmt.Sprintf("%.2f", st.CalculatedCompletionPct), st.CreatedAt.Format(time.RFC3339)})
		}
		_ = writeCSVFile("subtasks"+ext, []string{"subtask_id", "task_id", "name", "status", "estimate_seconds", "completion_pct", "created_at"}, subtaskRows)

		var sprintRows [][]string
		for _, sp := range sprints {
			owner := sp.TaskID
			if sp.SubtaskID != "" {
				owner = sp.SubtaskID
			}
			sprintRows = append(sprintRows, []string{sp.SprintID, owner, sp.Name, sp.Status, strconv.FormatInt(sp.EstimatedDurationSeconds, 10), sp.CreatedAt.Format(time.RFC3339)})
		}
		_ = writeCSVFile("sprints"+ext, []string{"sprint_id", "owner_id", "name", "status", "estimate_seconds", "created_at"}, sprintRows)

		_ = writeCSVFile("project"+ext, []string{"project_id", "name", "goal", "status", "created_at"}, [][]string{{data.ExecutionTree.Project.ProjectID, data.ExecutionTree.Project.ProjectName, data.ExecutionTree.Project.ProjectGoal, data.ExecutionTree.Project.Status, data.ExecutionTree.Project.CreatedAt.Format(time.RFC3339)}})

		if zipWriter != nil {
			_ = zipWriter.Close()
		}
		if tarWriter != nil {
			_ = tarWriter.Close()
		}
		if gw != nil {
			_ = gw.Close()
		}

	case "sql":
		fmt.Fprintf(writer, "-- TimeKeeper full project export for %s\n", projectID)

		if len(projects) > 0 {
			fmt.Fprintln(writer, "INSERT INTO projects (project_id, name, description, goal, status) VALUES")
			for i, p := range projects {
				fmt.Fprintf(writer, "('%s', '%s', '%s', '%s', '%s')",
					p.ProjectID, strings.ReplaceAll(p.ProjectName, "'", "''"), strings.ReplaceAll(p.ProjectDescription, "'", "''"), strings.ReplaceAll(p.ProjectGoal, "'", "''"), p.Status)
				if i < len(projects)-1 {
					fmt.Fprintln(writer, ",")
				} else {
					fmt.Fprintln(writer, ";")
				}
			}
		}

		if len(categories) > 0 {
			fmt.Fprintln(writer, "INSERT INTO categories (category_id, project_id, parent_category_id, name, description, goal, status) VALUES")
			for i, c := range categories {
				fmt.Fprintf(writer, "('%s', '%s', '%s', '%s', '%s', '%s', '%s')",
					c.CategoryID, c.ProjectID, c.ParentCategoryID, strings.ReplaceAll(c.Name, "'", "''"), strings.ReplaceAll(c.Description, "'", "''"), strings.ReplaceAll(c.Goal, "'", "''"), c.Status)
				if i < len(categories)-1 {
					fmt.Fprintln(writer, ",")
				} else {
					fmt.Fprintln(writer, ";")
				}
			}
		}

		if len(tasks) > 0 {
			fmt.Fprintln(writer, "INSERT INTO tasks (task_id, category_id, name, description, goal, status, priority, estimated_duration_seconds, auto_close) VALUES")
			for i, t := range tasks {
				autoClose := 0
				if t.AutoClose {
					autoClose = 1
				}
				fmt.Fprintf(writer, "('%s', '%s', '%s', '%s', '%s', '%s', '%s', %d, %d)",
					t.TaskID, t.CategoryID, strings.ReplaceAll(t.Name, "'", "''"), strings.ReplaceAll(t.Description, "'", "''"), strings.ReplaceAll(t.Goal, "'", "''"), t.Status, t.Priority, t.EstimatedDurationSeconds, autoClose)
				if i < len(tasks)-1 {
					fmt.Fprintln(writer, ",")
				} else {
					fmt.Fprintln(writer, ";")
				}
			}
		}

		if len(subtasks) > 0 {
			fmt.Fprintln(writer, "INSERT INTO subtasks (subtask_id, task_id, name, status, estimated_duration_seconds) VALUES")
			for i, st := range subtasks {
				fmt.Fprintf(writer, "('%s', '%s', '%s', '%s', %d)",
					st.SubtaskID, st.TaskID, strings.ReplaceAll(st.Name, "'", "''"), st.Status, st.EstimatedDurationSeconds)
				if i < len(subtasks)-1 {
					fmt.Fprintln(writer, ",")
				} else {
					fmt.Fprintln(writer, ";")
				}
			}
		}

		if len(sprints) > 0 {
			fmt.Fprintln(writer, "INSERT INTO sprints (sprint_id, owner_id, name, status, estimated_duration_seconds) VALUES")
			for i, sp := range sprints {
				owner := sp.TaskID
				if sp.SubtaskID != "" {
					owner = sp.SubtaskID
				}
				fmt.Fprintf(writer, "('%s', '%s', '%s', '%s', %d)",
					sp.SprintID, owner, strings.ReplaceAll(sp.Name, "'", "''"), sp.Status, sp.EstimatedDurationSeconds)
				if i < len(sprints)-1 {
					fmt.Fprintln(writer, ",")
				} else {
					fmt.Fprintln(writer, ";")
				}
			}
		}
	}

	return 0
}
