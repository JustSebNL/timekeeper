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
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/nwaples/rardecode/v2"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

func parseAutoCloseStrict(v string) (*bool, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return nil, nil
	}
	var value bool
	switch v {
	case "true", "yes", "1":
		value = true
	case "false", "no", "0":
		value = false
	default:
		return nil, fmt.Errorf("invalid auto_close %q: use true, false, yes, no, 1, or 0", v)
	}
	return &value, nil
}

func parseDurationStr(v string) int64 {
	seconds, err := parseImportDuration(v)
	if err != nil {
		return 0
	}
	return seconds
}

func parseImportDuration(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	duration, err := parseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid estimate %q: use a positive duration such as 30m, 2h, or seconds", v)
	}
	if duration%time.Second != 0 {
		return 0, fmt.Errorf("estimate %q must resolve to whole seconds", v)
	}
	return int64(duration / time.Second), nil
}

var taskImportColumns = map[string]struct{}{
	"name": {}, "category_id": {}, "description": {}, "goal": {}, "priority": {},
	"estimate": {}, "estimated_duration_seconds": {}, "auto_close": {},
	// Export-only fields are accepted for round-trip files and deliberately ignored.
	"task_id": {}, "project_id": {}, "item_address": {}, "status": {},
	"completion_pct": {}, "reported_completion_pct": {}, "calculated_completion_pct": {},
	"created_at": {}, "updated_at": {},
}

func taskImportHeaderMap(headers []string) (map[string]int, error) {
	headerMap := make(map[string]int, len(headers))
	for index, header := range headers {
		name := strings.ToLower(strings.TrimSpace(header))
		if name == "" {
			return nil, fmt.Errorf("header column %d is empty", index+1)
		}
		if _, ok := taskImportColumns[name]; !ok {
			return nil, fmt.Errorf("unknown column %q; remove it or use a documented task-import field", header)
		}
		if _, duplicate := headerMap[name]; duplicate {
			return nil, fmt.Errorf("duplicate column %q", name)
		}
		headerMap[name] = index
	}
	if _, ok := headerMap["name"]; !ok {
		return nil, fmt.Errorf("missing required column 'name'")
	}
	if _, hasEstimate := headerMap["estimate"]; hasEstimate {
		if _, hasSeconds := headerMap["estimated_duration_seconds"]; hasSeconds {
			return nil, fmt.Errorf("use either 'estimate' or 'estimated_duration_seconds', not both")
		}
	}
	return headerMap, nil
}

func processRow(headerMap map[string]int, row []string) (model.CreateTaskInput, error) {
	var task model.CreateTaskInput
	val := func(key string) string {
		idx, ok := headerMap[key]
		if ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}
	task.Name = val("name")
	task.CategoryID = val("category_id")
	task.Description = val("description")
	task.Goal = val("goal")
	task.Priority = val("priority")
	if v := val("auto_close"); v != "" {
		autoClose, err := parseAutoCloseStrict(v)
		if err != nil {
			return model.CreateTaskInput{}, err
		}
		task.AutoClose = autoClose
	}
	if estimate := val("estimate"); estimate != "" {
		seconds, err := parseImportDuration(estimate)
		if err != nil {
			return model.CreateTaskInput{}, err
		}
		task.EstimatedDurationSeconds = seconds
	} else if rawSeconds := val("estimated_duration_seconds"); rawSeconds != "" {
		seconds, err := strconv.ParseInt(rawSeconds, 10, 64)
		if err != nil || seconds < 0 {
			return model.CreateTaskInput{}, fmt.Errorf("invalid estimated_duration_seconds %q: use a non-negative integer", rawSeconds)
		}
		task.EstimatedDurationSeconds = seconds
	}
	for _, field := range []string{"completion_pct", "reported_completion_pct", "calculated_completion_pct"} {
		if raw := val(field); raw != "" {
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return model.CreateTaskInput{}, fmt.Errorf("invalid %s %q: use a finite number", field, raw)
			}
		}
	}
	for _, field := range []string{"created_at", "updated_at"} {
		if raw := val(field); raw != "" {
			if _, err := time.Parse(time.RFC3339, raw); err != nil {
				return model.CreateTaskInput{}, fmt.Errorf("invalid %s %q: use RFC3339 timestamp format", field, raw)
			}
		}
	}
	return task, nil
}

func blankImportRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func parseCSV(reader io.Reader, delimiter rune) ([]model.CreateTaskInput, error) {
	csvReader := csv.NewReader(reader)
	csvReader.Comma = delimiter
	csvReader.FieldsPerRecord = -1
	rows, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read delimited batch: %w", err)
	}
	if len(rows) < 1 {
		return nil, fmt.Errorf("empty file")
	}
	headerMap, err := taskImportHeaderMap(rows[0])
	if err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}
	var tasks []model.CreateTaskInput
	for rowIndex, row := range rows[1:] {
		if blankImportRow(row) {
			continue
		}
		if len(row) != len(rows[0]) {
			return nil, fmt.Errorf("row %d has %d fields; header has %d", rowIndex+2, len(row), len(rows[0]))
		}
		task, err := processRow(headerMap, row)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowIndex+2, err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func parseExcel(fileBytes []byte) ([]model.CreateTaskInput, error) {
	f, err := excelize.OpenReader(bytes.NewReader(fileBytes))
	if err != nil {
		return nil, fmt.Errorf("open Excel workbook: %w", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in Excel file")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("read first worksheet %q: %w", sheets[0], err)
	}
	if len(rows) < 1 {
		return nil, fmt.Errorf("worksheet %q is empty", sheets[0])
	}
	headerMap, err := taskImportHeaderMap(rows[0])
	if err != nil {
		return nil, fmt.Errorf("worksheet %q header: %w", sheets[0], err)
	}
	var tasks []model.CreateTaskInput
	for rowIndex, original := range rows[1:] {
		if blankImportRow(original) {
			continue
		}
		if len(original) > len(rows[0]) {
			return nil, fmt.Errorf("worksheet %q row %d has %d fields; header has %d", sheets[0], rowIndex+2, len(original), len(rows[0]))
		}
		row := append([]string(nil), original...)
		for len(row) < len(rows[0]) {
			row = append(row, "")
		}
		task, err := processRow(headerMap, row)
		if err != nil {
			return nil, fmt.Errorf("worksheet %q row %d: %w", sheets[0], rowIndex+2, err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func parseJSONOrYAML(fileBytes []byte, isJSON bool) ([]model.CreateTaskInput, error) {
	if !isJSON {
		decoder := yaml.NewDecoder(bytes.NewReader(fileBytes))
		var raw any
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("yaml unmarshal: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return nil, fmt.Errorf("YAML batch must contain exactly one document")
			}
			return nil, fmt.Errorf("invalid trailing YAML document: %w", err)
		}

		// recursively convert map[any]any to map[string]any for json.Marshal
		var convert func(any) any
		convert = func(i any) any {
			switch x := i.(type) {
			case map[any]any:
				m2 := map[string]any{}
				for k, v := range x {
					m2[fmt.Sprint(k)] = convert(v)
				}
				return m2
			case []any:
				for i, v := range x {
					x[i] = convert(v)
				}
			}
			return i
		}

		jsonBytes, err := json.Marshal(convert(raw))
		if err != nil {
			return nil, fmt.Errorf("yaml to json marshal: %w", err)
		}
		fileBytes = jsonBytes
	}

	return parseLegacyTaskJSON(fileBytes)
}

func parseSQL(fileBytes []byte) ([]model.CreateTaskInput, error) {
	return parseStrictTaskSQL(fileBytes)
}

// ImportTasks parses a file into a list of tasks.
func ImportTasks(filePath, format string) ([]model.CreateTaskInput, error) {
	var fileBytes []byte
	var err error

	if filePath == "-" {
		fileBytes, err = io.ReadAll(os.Stdin)
	} else {
		fileBytes, err = os.ReadFile(filePath)
	}

	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	if format == "" {
		if filePath == "-" {
			format = "json"
		} else {
			lowPath := strings.ToLower(filePath)
			if strings.HasSuffix(lowPath, ".tar.gz") {
				format = "tar.gz"
			} else {
				format = strings.TrimPrefix(filepath.Ext(filePath), ".")
			}
		}
	}

	format = strings.ToLower(format)

	switch format {
	case "csv":
		return parseCSV(bytes.NewReader(fileBytes), ',')
	case "tsv":
		return parseCSV(bytes.NewReader(fileBytes), '\t')
	case "xlsx":
		return parseExcel(fileBytes)
	case "yaml", "yml":
		return parseJSONOrYAML(fileBytes, false)
	case "json":
		return parseJSONOrYAML(fileBytes, true)
	case "sql":
		return parseSQL(fileBytes)
	case "zip":
		return parseZIP(fileBytes)
	case "tar":
		return parseTar(bytes.NewReader(fileBytes))
	case "tar.gz", "tgz":
		return parseTarGz(bytes.NewReader(fileBytes))
	case "rar":
		return parseRAR(fileBytes)
	case "gz":
		innerExt := filepath.Ext(strings.TrimSuffix(strings.ToLower(filePath), ".gz"))
		if innerExt == "" {
			return nil, fmt.Errorf("unknown inner format for .gz")
		}
		return parseGZ(bytes.NewReader(fileBytes), innerExt)
	default:
		return nil, fmt.Errorf("unsupported format: %q", format)
	}
}

func parseFileContent(format string, content []byte) ([]model.CreateTaskInput, error) {
	switch format {
	case "csv":
		return parseCSV(bytes.NewReader(content), ',')
	case "tsv":
		return parseCSV(bytes.NewReader(content), '\t')
	case "yaml", "yml":
		return parseJSONOrYAML(content, false)
	case "json":
		return parseJSONOrYAML(content, true)
	case "sql":
		return parseSQL(content)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func parseZIP(fileBytes []byte) ([]model.CreateTaskInput, error) {
	zr, err := zip.NewReader(bytes.NewReader(fileBytes), int64(len(fileBytes)))
	if err != nil {
		return nil, fmt.Errorf("read zip: %w", err)
	}
	var allTasks []model.CreateTaskInput
	members := 0
	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(file.Name)), ".")
		if !supportedTaskArchiveFormat(format) {
			return nil, fmt.Errorf("unsupported file %q in ZIP; include only CSV, TSV, JSON, YAML, or SQL task batches", file.Name)
		}
		reader, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open file %q in zip: %w", file.Name, err)
		}
		content, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read file %q in zip: %w", file.Name, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close file %q in zip: %w", file.Name, closeErr)
		}
		tasks, err := parseFileContent(format, content)
		if err != nil {
			return nil, fmt.Errorf("parse file %q in zip: %w", file.Name, err)
		}
		members++
		allTasks = append(allTasks, tasks...)
	}
	if members == 0 {
		return nil, fmt.Errorf("ZIP archive contains no supported task files")
	}
	return allTasks, nil
}

func supportedTaskArchiveFormat(format string) bool {
	switch format {
	case "csv", "tsv", "json", "yaml", "yml", "sql":
		return true
	default:
		return false
	}
}

func parseTar(reader io.Reader) ([]model.CreateTaskInput, error) {
	tr := tar.NewReader(reader)
	var allTasks []model.CreateTaskInput
	members := 0
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar read: %w", err)
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("unsupported non-regular archive entry %q", header.Name)
		}
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Name)), ".")
		if !supportedTaskArchiveFormat(format) {
			return nil, fmt.Errorf("unsupported file %q in TAR; include only CSV, TSV, JSON, YAML, or SQL task batches", header.Name)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read file %q in tar: %w", header.Name, err)
		}
		tasks, err := parseFileContent(format, content)
		if err != nil {
			return nil, fmt.Errorf("parse file %q in tar: %w", header.Name, err)
		}
		members++
		allTasks = append(allTasks, tasks...)
	}
	if members == 0 {
		return nil, fmt.Errorf("TAR archive contains no supported task files")
	}
	return allTasks, nil
}

func parseTarGz(reader io.Reader) ([]model.CreateTaskInput, error) {
	gzr, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("gzip read: %w", err)
	}
	defer gzr.Close()
	return parseTar(gzr)
}

func parseGZ(reader io.Reader, originalExt string) ([]model.CreateTaskInput, error) {
	gzr, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("gzip read: %w", err)
	}
	defer gzr.Close()

	content, err := io.ReadAll(gzr)
	if err != nil {
		return nil, fmt.Errorf("read gz content: %w", err)
	}

	format := strings.TrimPrefix(originalExt, ".")
	return parseFileContent(format, content)
}

func parseRAR(fileBytes []byte) ([]model.CreateTaskInput, error) {
	reader, err := rardecode.NewReader(bytes.NewReader(fileBytes))
	if err != nil {
		return nil, fmt.Errorf("rar init: %w", err)
	}
	var allTasks []model.CreateTaskInput
	members := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("rar read: %w", err)
		}
		if header.IsDir {
			continue
		}
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Name)), ".")
		if !supportedTaskArchiveFormat(format) {
			return nil, fmt.Errorf("unsupported file %q in RAR; include only CSV, TSV, JSON, YAML, or SQL task batches", header.Name)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("read file %q in rar: %w", header.Name, err)
		}
		tasks, err := parseFileContent(format, content)
		if err != nil {
			return nil, fmt.Errorf("parse file %q in rar: %w", header.Name, err)
		}
		members++
		allTasks = append(allTasks, tasks...)
	}
	if members == 0 {
		return nil, fmt.Errorf("RAR archive contains no supported task files")
	}
	return allTasks, nil
}
