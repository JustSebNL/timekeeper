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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

// formatDuration formats seconds into a human-readable duration (e.g. 1h30m).
func formatDuration(seconds int64) string {
	if seconds == 0 {
		return "0s"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60

	var parts []string
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}

	return strings.Join(parts, "")
}

func taskExport(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(errOut, "usage: tk t export <project-id> <file> [--status S] [--category C]")
		return 2
	}

	var format string
	var statusFilter string
	var categoryFilter string

	// parse optional flags
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--format" && i+1 < len(args) {
			format = args[i+1]
			i++
		} else if args[i] == "--status" && i+1 < len(args) {
			statusFilter = args[i+1]
			i++
		} else if args[i] == "--category" && i+1 < len(args) {
			categoryFilter = args[i+1]
			i++
		} else {
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 2 {
		_, _ = fmt.Fprintln(errOut, "usage: tk t export <project-id> <file> [--status S] [--category C] [--format F]")
		return 2
	}

	projectID := positional[0]
	filePath := positional[1]

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
					_, _ = fmt.Fprintf(errOut, "unknown file extension %q, use --format to specify\n", ext)
					return 2
				}
			}
		}
	} else {
		format = strings.ToLower(format)
		if format == "tgz" {
			format = "tar.gz"
		}
		if format != "csv" && format != "tsv" && format != "yaml" && format != "json" && format != "xlsx" && format != "sql" && format != "zip" && format != "json-zip" && format != "tar" && format != "tar.gz" && format != "gz" && format != "rar" && format != "json-tar.gz" {
			_, _ = fmt.Fprintf(errOut, "unknown format %q\n", format)
			return 2
		}
	}

	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/tasks"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create request: %v\n", err)
		return 1
	}

	resp, err := newHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", url, err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "fetch tasks", resp)
		return 1
	}

	var responseData struct {
		Items []model.Task `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API returned invalid JSON: %v\n", err)
		return 1
	}

	var filteredTasks []model.Task
	for _, t := range responseData.Items {
		if statusFilter != "" && t.Status != statusFilter {
			continue
		}
		if categoryFilter != "" && t.CategoryID != categoryFilter {
			continue
		}
		filteredTasks = append(filteredTasks, t)
	}

	var writer io.Writer
	if filePath == "-" {
		writer = out
	} else {
		f, err := os.Create(filePath)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "create file %q: %v\n", filePath, err)
			return 1
		}
		defer f.Close()
		writer = f
	}

	switch format {
	case "csv", "tsv":
		w := csv.NewWriter(writer)
		if format == "tsv" {
			w.Comma = '\t'
		}

		headers := []string{"task_id", "category_id", "name", "description", "goal", "status", "priority", "estimate", "completion_pct", "auto_close", "created_at"}
		if err := w.Write(headers); err != nil {
			_, _ = fmt.Fprintf(errOut, "write header: %v\n", err)
			return 1
		}
		for _, t := range filteredTasks {
			row := []string{
				t.TaskID,
				t.CategoryID,
				t.Name,
				t.Description,
				t.Goal,
				t.Status,
				t.Priority,
				formatDuration(t.EstimatedDurationSeconds),
				fmt.Sprintf("%.2f", t.CalculatedCompletionPct),
				strconv.FormatBool(t.AutoClose),
				t.CreatedAt.Format(time.RFC3339),
			}
			if err := w.Write(row); err != nil {
				_, _ = fmt.Fprintf(errOut, "write row: %v\n", err)
				return 1
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			_, _ = fmt.Fprintf(errOut, "flush %s: %v\n", format, err)
			return 1
		}

	case "xlsx":
		f := excelize.NewFile()
		defer f.Close()
		sheetName := "Tasks"

		// Ensure sheet exists and we rename Sheet1
		f.SetSheetName("Sheet1", sheetName)

		headers := []string{"task_id", "category_id", "name", "description", "goal", "status", "priority", "estimate", "completion_pct", "auto_close", "created_at"}

		boldStyle, err := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true},
		})
		if err == nil {
			_ = f.SetRowStyle(sheetName, 1, 1, boldStyle)
		}

		for colIdx, h := range headers {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
			f.SetCellValue(sheetName, cell, h)
		}

		for rowIdx, t := range filteredTasks {
			rowNum := rowIdx + 2
			row := []any{
				t.TaskID,
				t.CategoryID,
				t.Name,
				t.Description,
				t.Goal,
				t.Status,
				t.Priority,
				formatDuration(t.EstimatedDurationSeconds),
				t.CalculatedCompletionPct,
				t.AutoClose,
				t.CreatedAt.Format(time.RFC3339),
			}
			for colIdx, val := range row {
				cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowNum)
				f.SetCellValue(sheetName, cell, val)
			}
		}

		// Auto-size columns (rudimentary)
		for colIdx := range headers {
			colName, _ := excelize.ColumnNumberToName(colIdx + 1)
			_ = f.SetColWidth(sheetName, colName, colName, 20)
		}

		if filePath == "-" {
			if err := f.Write(writer); err != nil {
				_, _ = fmt.Fprintf(errOut, "write xlsx to stdout: %v\n", err)
				return 1
			}
		} else {
			if err := f.SaveAs(filePath); err != nil {
				_, _ = fmt.Fprintf(errOut, "save xlsx: %v\n", err)
				return 1
			}
		}

	case "yaml":
		// Format to match CSV columns
		var yamlOutput []map[string]any
		for _, t := range filteredTasks {
			yamlOutput = append(yamlOutput, map[string]any{
				"task_id":        t.TaskID,
				"category_id":    t.CategoryID,
				"name":           t.Name,
				"description":    t.Description,
				"goal":           t.Goal,
				"status":         t.Status,
				"priority":       t.Priority,
				"estimate":       formatDuration(t.EstimatedDurationSeconds),
				"completion_pct": t.CalculatedCompletionPct,
				"auto_close":     t.AutoClose,
				"created_at":     t.CreatedAt.Format(time.RFC3339),
			})
		}
		enc := yaml.NewEncoder(writer)
		if err := enc.Encode(yamlOutput); err != nil {
			_, _ = fmt.Fprintf(errOut, "encode yaml: %v\n", err)
			return 1
		}
		enc.Close()

	case "json":
		wrapper := map[string]any{"tasks": filteredTasks}
		enc := json.NewEncoder(writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(wrapper); err != nil {
			_, _ = fmt.Fprintf(errOut, "encode json: %v\n", err)
			return 1
		}

	case "sql":
		fmt.Fprintf(writer, "-- TimeKeeper task export for %s\n", projectID)
		fmt.Fprintf(writer, "-- Generated: %s\n\n", time.Now().UTC().Format(time.RFC3339))

		if len(filteredTasks) > 0 {
			fmt.Fprintln(writer, "INSERT INTO tasks (task_id, category_id, name, description, goal, status, priority, estimated_duration_seconds, auto_close) VALUES")
			for i, t := range filteredTasks {
				name := strings.ReplaceAll(t.Name, "'", "''")
				desc := strings.ReplaceAll(t.Description, "'", "''")
				goal := strings.ReplaceAll(t.Goal, "'", "''")
				status := strings.ReplaceAll(t.Status, "'", "''")
				priority := strings.ReplaceAll(t.Priority, "'", "''")

				autoClose := 0
				if t.AutoClose {
					autoClose = 1
				}

				fmt.Fprintf(writer, "('%s', '%s', '%s', '%s', '%s', '%s', '%s', %d, %d)",
					t.TaskID, t.CategoryID, name, desc, goal, status, priority, t.EstimatedDurationSeconds, autoClose)

				if i < len(filteredTasks)-1 {
					fmt.Fprintln(writer, ",")
				} else {
					fmt.Fprintln(writer, ";")
				}
			}
		}

	case "rar":
		_, _ = fmt.Fprintf(errOut, "rar export is not supported, only decoding is supported\n")
		return 1

	case "gz":
		// just write gzipped csv
		gw := gzip.NewWriter(writer)
		w := csv.NewWriter(gw)
		headers := []string{"task_id", "category_id", "name", "description", "goal", "status", "priority", "estimate", "completion_pct", "auto_close", "created_at"}
		_ = w.Write(headers)
		for _, t := range filteredTasks {
			_ = w.Write([]string{t.TaskID, t.CategoryID, t.Name, t.Description, t.Goal, t.Status, t.Priority, formatDuration(t.EstimatedDurationSeconds), fmt.Sprintf("%.2f", t.CalculatedCompletionPct), strconv.FormatBool(t.AutoClose), t.CreatedAt.Format(time.RFC3339)})
		}
		w.Flush()
		_ = gw.Close()

	case "tar", "tar.gz", "json-tar.gz":
		var gw *gzip.Writer
		var tarWriter *tar.Writer
		if format == "tar.gz" || format == "json-tar.gz" {
			gw = gzip.NewWriter(writer)
			tarWriter = tar.NewWriter(gw)
		} else {
			tarWriter = tar.NewWriter(writer)
		}

		var filename string
		var buf bytes.Buffer

		if format == "json-tar.gz" {
			filename = "tasks.json"
			wrapper := map[string]any{"tasks": filteredTasks}
			enc := json.NewEncoder(&buf)
			enc.SetIndent("", "  ")
			_ = enc.Encode(wrapper)
		} else {
			filename = "tasks.csv"
			w := csv.NewWriter(&buf)
			headers := []string{"task_id", "category_id", "name", "description", "goal", "status", "priority", "estimate", "completion_pct", "auto_close", "created_at"}
			_ = w.Write(headers)
			for _, t := range filteredTasks {
				_ = w.Write([]string{t.TaskID, t.CategoryID, t.Name, t.Description, t.Goal, t.Status, t.Priority, formatDuration(t.EstimatedDurationSeconds), fmt.Sprintf("%.2f", t.CalculatedCompletionPct), strconv.FormatBool(t.AutoClose), t.CreatedAt.Format(time.RFC3339)})
			}
			w.Flush()
		}

		hdr := &tar.Header{
			Name:    filename,
			Mode:    0600,
			Size:    int64(buf.Len()),
			ModTime: time.Now(),
		}
		if err := tarWriter.WriteHeader(hdr); err != nil {
			_, _ = fmt.Fprintf(errOut, "tar header write: %v\n", err)
			return 1
		}
		if _, err := tarWriter.Write(buf.Bytes()); err != nil {
			_, _ = fmt.Fprintf(errOut, "tar content write: %v\n", err)
			return 1
		}

		if err := tarWriter.Close(); err != nil {
			_, _ = fmt.Fprintf(errOut, "close tar: %v\n", err)
			return 1
		}
		if gw != nil {
			if err := gw.Close(); err != nil {
				_, _ = fmt.Fprintf(errOut, "close gzip: %v\n", err)
				return 1
			}
		}

	case "zip", "json-zip":
		zw := zip.NewWriter(writer)
		var filename string
		if format == "json-zip" {
			filename = "tasks.json"
		} else {
			filename = "tasks.csv"
		}

		f, err := zw.Create(filename)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "create file in zip: %v\n", err)
			return 1
		}

		if format == "json-zip" {
			wrapper := map[string]any{"tasks": filteredTasks}
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			if err := enc.Encode(wrapper); err != nil {
				_, _ = fmt.Fprintf(errOut, "encode json in zip: %v\n", err)
				return 1
			}
		} else {
			w := csv.NewWriter(f)
			headers := []string{"task_id", "category_id", "name", "description", "goal", "status", "priority", "estimate", "completion_pct", "auto_close", "created_at"}
			if err := w.Write(headers); err != nil {
				_, _ = fmt.Fprintf(errOut, "write header: %v\n", err)
				return 1
			}
			for _, t := range filteredTasks {
				row := []string{
					t.TaskID,
					t.CategoryID,
					t.Name,
					t.Description,
					t.Goal,
					t.Status,
					t.Priority,
					formatDuration(t.EstimatedDurationSeconds),
					fmt.Sprintf("%.2f", t.CalculatedCompletionPct),
					strconv.FormatBool(t.AutoClose),
					t.CreatedAt.Format(time.RFC3339),
				}
				if err := w.Write(row); err != nil {
					_, _ = fmt.Fprintf(errOut, "write row: %v\n", err)
					return 1
				}
			}
			w.Flush()
			if err := w.Error(); err != nil {
				_, _ = fmt.Fprintf(errOut, "flush zip csv: %v\n", err)
				return 1
			}
		}

		if err := zw.Close(); err != nil {
			_, _ = fmt.Fprintf(errOut, "close zip: %v\n", err)
			return 1
		}
	}

	return 0
}
