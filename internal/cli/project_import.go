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
	"github.com/nwaples/rardecode/v2"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

type ParsedProjectImport struct {
	Project    model.Project
	Categories []model.Category
	Tasks      []model.Task
	Subtasks   []model.Subtask
	Sprints    []model.Sprint
}

func importProject(args []string, out, errOut io.Writer, baseURL string) int {
	var positional []string
	var format string
	var nameOverride string

	for i := 0; i < len(args); i++ {
		if args[i] == "--format" && i+1 < len(args) {
			format = args[i+1]
			i++
		} else if args[i] == "--name" && i+1 < len(args) {
			nameOverride = args[i+1]
			i++
		} else {
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 1 {
		_, _ = fmt.Fprintln(errOut, "usage: tk p import <file> [--name \"New Name\"] [--format F]")
		return 2
	}

	filePath := positional[0]

	if format == "" {
		if filePath == "-" {
			format = "json"
		} else {
			ext := strings.ToLower(filepath.Ext(filePath))
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
			case ".rar":
				format = "rar"
			case ".tar":
				format = "tar"
			case ".tgz":
				format = "tgz"
			case ".gz":
				if strings.HasSuffix(strings.ToLower(filePath), ".tar.gz") {
					format = "tar.gz"
				} else {
					format = "gz"
				}
			default:
				_, _ = fmt.Fprintf(errOut, "unknown file extension %q for project import\n", ext)
				return 2
			}
		}
	} else {
		format = strings.ToLower(format)
	}

	var fileBytes []byte
	var err error
	if filePath == "-" {
		fileBytes, err = io.ReadAll(os.Stdin)
	} else {
		fileBytes, err = os.ReadFile(filePath)
	}
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "read file: %v\n", err)
		return 1
	}

	parsed, err := parseProjectImport(fileBytes, format)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "parse import: %v\n", err)
		return 1
	}

	if nameOverride != "" {
		parsed.Project.ProjectName = nameOverride
	}

	// 1. Create Project
	projInput := model.CreateProjectInput{
		Name:        parsed.Project.ProjectName,
		Description: parsed.Project.ProjectDescription,
		Goal:        parsed.Project.ProjectGoal,
		Priority:    parsed.Project.Priority,
		PaletteID:   parsed.Project.PaletteID,
		Alias:       parsed.Project.ProjectAlias,
	}

	projInputBytes, _ := json.Marshal(projInput)
	projURL := strings.TrimRight(baseURL, "/") + "/api/v1/projects"
	req, _ := http.NewRequest(http.MethodPost, projURL, bytes.NewReader(projInputBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := newHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Time Keeper API unavailable at %s: %v\n", projURL, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "create project", resp)
		return 1
	}
	var createdProject struct {
		ProjectID string `json:"project_id"`
	}
	json.NewDecoder(resp.Body).Decode(&createdProject)
	newProjectID := createdProject.ProjectID

	// Maps from old IDs to new IDs
	catMap := make(map[string]string)
	taskMap := make(map[string]string)
	subtaskMap := make(map[string]string)

	// 2. Create Categories
	// Sort categories by parent/child so parents are created first
	var pendingCats = parsed.Categories
	var createdCats int
	for len(pendingCats) > 0 {
		var nextCats []model.Category
		var madeProgress bool
		for _, c := range pendingCats {
			if c.ParentCategoryID == "" || catMap[c.ParentCategoryID] != "" {
				catInput := model.CreateCategoryInput{
					Name:             c.Name,
					ParentCategoryID: catMap[c.ParentCategoryID],
					Description:      c.Description,
					Goal:             c.Goal,
					Priority:         c.Priority,
				}
				cBytes, _ := json.Marshal(catInput)
				cURL := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + newProjectID + "/categories"
				cReq, _ := http.NewRequest(http.MethodPost, cURL, bytes.NewReader(cBytes))
				cReq.Header.Set("Content-Type", "application/json")
				cResp, err := newHTTPClient(10 * time.Second).Do(cReq)
				if err == nil && cResp.StatusCode == http.StatusCreated {
					var cc struct {
						CategoryID string `json:"category_id"`
					}
					json.NewDecoder(cResp.Body).Decode(&cc)
					cResp.Body.Close()
					catMap[c.CategoryID] = cc.CategoryID
					madeProgress = true
					createdCats++
				} else {
					if err == nil {
						reportHTTPStatus(errOut, "create category", cResp)
					} else {
						_, _ = fmt.Fprintf(errOut, "create category err: %v\n", err)
					}
					return 1
				}
			} else {
				nextCats = append(nextCats, c)
			}
		}
		if !madeProgress {
			_, _ = fmt.Fprintln(errOut, "circular or missing parent category dependency detected")
			return 1
		}
		pendingCats = nextCats
	}

	// 3. Create Tasks in Batch
	var taskInputs []model.CreateTaskInput
	var oldTaskIDs []string
	for _, t := range parsed.Tasks {
		ac := t.AutoClose
		in := model.CreateTaskInput{
			CategoryID:               catMap[t.CategoryID],
			Name:                     t.Name,
			Description:              t.Description,
			Goal:                     t.Goal,
			Priority:                 t.Priority,
			EstimatedDurationSeconds: t.EstimatedDurationSeconds,
			AutoClose:                &ac,
		}
		taskInputs = append(taskInputs, in)
		oldTaskIDs = append(oldTaskIDs, t.TaskID)
	}

	if len(taskInputs) > 0 {
		batchInput := model.CreateTasksBatchInput{Tasks: taskInputs}
		bBytes, _ := json.Marshal(batchInput)
		bURL := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + newProjectID + "/tasks/batch"
		bReq, _ := http.NewRequest(http.MethodPost, bURL, bytes.NewReader(bBytes))
		bReq.Header.Set("Content-Type", "application/json")
		bResp, err := newHTTPClient(30 * time.Second).Do(bReq)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "create tasks err: %v\n", err)
			return 1
		}
		if bResp.StatusCode != http.StatusCreated {
			reportHTTPStatus(errOut, "create tasks batch", bResp)
			return 1
		}
		var batchRes struct {
			Tasks []model.Task `json:"tasks"`
		}
		json.NewDecoder(bResp.Body).Decode(&batchRes)
		bResp.Body.Close()

		for i, rt := range batchRes.Tasks {
			taskMap[oldTaskIDs[i]] = rt.TaskID
		}
	}

	// 4. Create Subtasks
	for _, st := range parsed.Subtasks {
		stInput := model.CreateSubtaskInput{
			Name:                     st.Name,
			Description:              st.Description,
			Goal:                     st.Goal,
			Priority:                 st.Priority,
			EstimatedDurationSeconds: st.EstimatedDurationSeconds,
		}
		stBytes, _ := json.Marshal(stInput)
		stURL := strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + taskMap[st.TaskID] + "/subtasks"
		stReq, _ := http.NewRequest(http.MethodPost, stURL, bytes.NewReader(stBytes))
		stReq.Header.Set("Content-Type", "application/json")
		stResp, err := newHTTPClient(10 * time.Second).Do(stReq)
		if err == nil && stResp.StatusCode == http.StatusCreated {
			var rst struct {
				SubtaskID string `json:"subtask_id"`
			}
			json.NewDecoder(stResp.Body).Decode(&rst)
			stResp.Body.Close()
			subtaskMap[st.SubtaskID] = rst.SubtaskID
		} else {
			if err == nil {
				reportHTTPStatus(errOut, "create subtask", stResp)
			} else {
				_, _ = fmt.Fprintf(errOut, "create subtask err: %v\n", err)
			}
			return 1
		}
	}

	// 5. Create Sprints
	var createdSprints int
	for _, sp := range parsed.Sprints {
		spInput := model.CreateSprintInput{
			Name:                     sp.Name,
			Description:              sp.Description,
			Goal:                     sp.Goal,
			Priority:                 sp.Priority,
			EstimatedDurationSeconds: sp.EstimatedDurationSeconds,
			BufferPct:                sp.BufferPct,
		}
		spBytes, _ := json.Marshal(spInput)

		var spURL string
		if subtaskMap[sp.SubtaskID] != "" {
			spURL = strings.TrimRight(baseURL, "/") + "/api/v1/subtasks/" + subtaskMap[sp.SubtaskID] + "/sprints"
		} else {
			spURL = strings.TrimRight(baseURL, "/") + "/api/v1/tasks/" + taskMap[sp.TaskID] + "/sprints"
		}

		spReq, _ := http.NewRequest(http.MethodPost, spURL, bytes.NewReader(spBytes))
		spReq.Header.Set("Content-Type", "application/json")
		spResp, err := newHTTPClient(10 * time.Second).Do(spReq)
		if err == nil && spResp.StatusCode == http.StatusCreated {
			spResp.Body.Close()
			createdSprints++
		} else {
			if err == nil {
				reportHTTPStatus(errOut, "create sprint", spResp)
			} else {
				_, _ = fmt.Fprintf(errOut, "create sprint err: %v\n", err)
			}
			return 1
		}
	}

	if jsonOutputEnabled {
		_ = json.NewEncoder(out).Encode(map[string]any{
			"project_id": newProjectID,
			"categories": createdCats,
			"tasks":      len(parsed.Tasks),
			"subtasks":   len(parsed.Subtasks),
			"sprints":    createdSprints,
		})
	} else {
		nameStr := nameOverride
		if nameStr == "" {
			nameStr = parsed.Project.ProjectName
		}
		fmt.Fprintf(out, "Imported Project %s: %q\n", newProjectID, nameStr)
		fmt.Fprintf(out, "  Categories: %d\n", createdCats)
		fmt.Fprintf(out, "  Tasks: %d\n", len(parsed.Tasks))
		fmt.Fprintf(out, "  Subtasks: %d\n", len(parsed.Subtasks))
		fmt.Fprintf(out, "  Sprints: %d\n", createdSprints)
	}

	return 0
}

func parseProjectImport(fileBytes []byte, format string) (*ParsedProjectImport, error) {
	if format == "gz" {
		gzr, err := gzip.NewReader(bytes.NewReader(fileBytes))
		if err != nil {
			return nil, err
		}
		decompressed, err := io.ReadAll(gzr)
		gzr.Close()
		if err != nil {
			return nil, err
		}
		return parseProjectImport(decompressed, "json")
	}

	var parsed ParsedProjectImport

	switch format {
	case "json", "yaml":
		if format == "yaml" {
			var raw any
			if err := yaml.Unmarshal(fileBytes, &raw); err != nil {
				return nil, err
			}
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
					for idx, v := range x {
						x[idx] = convert(v)
					}
				}
				return i
			}
			jb, err := json.Marshal(convert(raw))
			if err != nil {
				return nil, err
			}
			fileBytes = jb
		}

		// Attempt parsing as ExecutionTree or alternative
		var tree model.ExecutionTree
		if err := json.Unmarshal(fileBytes, &tree); err == nil && tree.Project.ProjectID != "" {
			parsed.Project = tree.Project
			var walkCategory func(c model.ExecutionCategory)
			walkCategory = func(c model.ExecutionCategory) {
				parsed.Categories = append(parsed.Categories, c.Category)
				for _, sub := range c.Categories {
					walkCategory(sub)
				}
				for _, t := range c.Tasks {
					parsed.Tasks = append(parsed.Tasks, t.Task)
					parsed.Sprints = append(parsed.Sprints, t.Sprints...)
					for _, st := range t.Subtasks {
						parsed.Subtasks = append(parsed.Subtasks, st.Subtask)
						parsed.Sprints = append(parsed.Sprints, st.Sprints...)
					}
				}
			}
			for _, c := range tree.Categories {
				walkCategory(c)
			}
		} else {
			// Alternative flat JSON
			var flat struct {
				Project    model.Project    `json:"project"`
				Categories []model.Category `json:"categories"`
				Tasks      []model.Task     `json:"tasks"`
				Subtasks   []model.Subtask  `json:"subtasks"`
				Sprints    []model.Sprint   `json:"sprints"`
			}
			if err := json.Unmarshal(fileBytes, &flat); err != nil {
				return nil, err
			}
			parsed.Project = flat.Project
			parsed.Categories = flat.Categories
			parsed.Tasks = flat.Tasks
			parsed.Subtasks = flat.Subtasks
			parsed.Sprints = flat.Sprints
		}

	case "csv", "tsv", "zip", "tar", "tar.gz", "tgz", "rar":
		var files map[string][]byte
		if format == "csv" || format == "tsv" {
			files = map[string][]byte{"project." + format: fileBytes, "categories." + format: fileBytes, "tasks." + format: fileBytes, "subtasks." + format: fileBytes, "sprints." + format: fileBytes}
		} else {
			var err error
			files, err = extractArchiveFiles(fileBytes, format)
			if err != nil {
				return nil, err
			}
		}

		var readZipCSV = func(name string) ([][]string, error) {
			for fName, data := range files {
				if strings.Contains(fName, name) {
					cr := csv.NewReader(bytes.NewReader(data))
					if strings.HasSuffix(fName, ".tsv") || format == "tsv" {
						cr.Comma = '	'
					}
					return cr.ReadAll()
				}
			}
			return nil, nil
		}

		if rows, _ := readZipCSV("project"); len(rows) > 1 {
			parsed.Project.ProjectID = rows[1][0]
			parsed.Project.ProjectName = rows[1][1]
			parsed.Project.ProjectGoal = rows[1][2]
			parsed.Project.Status = rows[1][3]
		}
		if rows, _ := readZipCSV("categories"); len(rows) > 1 {
			for i := 1; i < len(rows); i++ {
				parsed.Categories = append(parsed.Categories, model.Category{
					CategoryID:       rows[i][0],
					ProjectID:        rows[i][1],
					ParentCategoryID: rows[i][2],
					Name:             rows[i][3],
					Description:      rows[i][4],
					Goal:             rows[i][5],
					Status:           rows[i][6],
				})
			}
		}
		if rows, _ := readZipCSV("tasks"); len(rows) > 1 {
			for i := 1; i < len(rows); i++ {
				dur, _ := strconv.ParseInt(rows[i][7], 10, 64)
				ac := rows[i][9] == "1" || rows[i][9] == "true"
				parsed.Tasks = append(parsed.Tasks, model.Task{
					TaskID:                   rows[i][0],
					CategoryID:               rows[i][1],
					Name:                     rows[i][2],
					Description:              rows[i][3],
					Goal:                     rows[i][4],
					Status:                   rows[i][5],
					Priority:                 rows[i][6],
					EstimatedDurationSeconds: dur,
					AutoClose:                ac,
				})
			}
		}
		if rows, _ := readZipCSV("subtasks"); len(rows) > 1 {
			for i := 1; i < len(rows); i++ {
				dur, _ := strconv.ParseInt(rows[i][4], 10, 64)
				parsed.Subtasks = append(parsed.Subtasks, model.Subtask{
					SubtaskID:                rows[i][0],
					TaskID:                   rows[i][1],
					Name:                     rows[i][2],
					Status:                   rows[i][3],
					EstimatedDurationSeconds: dur,
				})
			}
		}
		if rows, _ := readZipCSV("sprints"); len(rows) > 1 {
			for i := 1; i < len(rows); i++ {
				dur, _ := strconv.ParseInt(rows[i][4], 10, 64)
				owner := rows[i][1]
				sp := model.Sprint{
					SprintID:                 rows[i][0],
					Name:                     rows[i][2],
					Status:                   rows[i][3],
					EstimatedDurationSeconds: dur,
				}
				// We don't know if owner is task or subtask here easily unless we check
				// We will check during creation or we can just try to see if it matches a task
				isTask := false
				for _, t := range parsed.Tasks {
					if t.TaskID == owner {
						isTask = true
						break
					}
				}
				if isTask {
					sp.TaskID = owner
				} else {
					sp.SubtaskID = owner
				}
				parsed.Sprints = append(parsed.Sprints, sp)
			}
		}

	case "xlsx":
		f, err := excelize.OpenReader(bytes.NewReader(fileBytes))
		if err != nil {
			return nil, err
		}
		defer f.Close()

		if rows, _ := f.GetRows("Project"); len(rows) > 1 {
			parsed.Project.ProjectID = rows[1][0]
			parsed.Project.ProjectName = rows[1][1]
			parsed.Project.ProjectGoal = rows[1][2]
			parsed.Project.Status = rows[1][3]
		}
		if rows, _ := f.GetRows("Categories"); len(rows) > 1 {
			for i := 1; i < len(rows); i++ {
				parsed.Categories = append(parsed.Categories, model.Category{
					CategoryID:       rows[i][0],
					ProjectID:        rows[i][1],
					ParentCategoryID: rows[i][2],
					Name:             rows[i][3],
					Description:      rows[i][4],
					Goal:             rows[i][5],
					Status:           rows[i][6],
				})
			}
		}
		if rows, _ := f.GetRows("Tasks"); len(rows) > 1 {
			for i := 1; i < len(rows); i++ {
				dur, _ := strconv.ParseInt(rows[i][7], 10, 64)
				ac := rows[i][9] == "1" || rows[i][9] == "true"
				parsed.Tasks = append(parsed.Tasks, model.Task{
					TaskID:                   rows[i][0],
					CategoryID:               rows[i][1],
					Name:                     rows[i][2],
					Description:              rows[i][3],
					Goal:                     rows[i][4],
					Status:                   rows[i][5],
					Priority:                 rows[i][6],
					EstimatedDurationSeconds: dur,
					AutoClose:                ac,
				})
			}
		}
		if rows, _ := f.GetRows("Subtasks"); len(rows) > 1 {
			for i := 1; i < len(rows); i++ {
				dur, _ := strconv.ParseInt(rows[i][4], 10, 64)
				parsed.Subtasks = append(parsed.Subtasks, model.Subtask{
					SubtaskID:                rows[i][0],
					TaskID:                   rows[i][1],
					Name:                     rows[i][2],
					Status:                   rows[i][3],
					EstimatedDurationSeconds: dur,
				})
			}
		}
		if rows, _ := f.GetRows("Sprints"); len(rows) > 1 {
			for i := 1; i < len(rows); i++ {
				dur, _ := strconv.ParseInt(rows[i][4], 10, 64)
				owner := rows[i][1]
				sp := model.Sprint{
					SprintID:                 rows[i][0],
					Name:                     rows[i][2],
					Status:                   rows[i][3],
					EstimatedDurationSeconds: dur,
				}
				isTask := false
				for _, t := range parsed.Tasks {
					if t.TaskID == owner {
						isTask = true
						break
					}
				}
				if isTask {
					sp.TaskID = owner
				} else {
					sp.SubtaskID = owner
				}
				parsed.Sprints = append(parsed.Sprints, sp)
			}
		}

	case "sql":
		// Quick and dirty parser for the SQL generated by project_export.go
		text := string(fileBytes)

		extractInserts := func(table string) [][]string {
			var res [][]string
			idx := strings.Index(text, "INSERT INTO "+table+" ")
			if idx == -1 {
				return nil
			}
			endIdx := strings.Index(text[idx:], ";")
			if endIdx == -1 {
				return nil
			}
			stmt := text[idx : idx+endIdx]
			valsIdx := strings.Index(stmt, "VALUES")
			if valsIdx == -1 {
				return nil
			}
			valsStr := stmt[valsIdx+6:]

			// Parse tuples
			var currentRow []string
			var currentVal strings.Builder
			inQuote := false
			inTuple := false

			for i := 0; i < len(valsStr); i++ {
				c := valsStr[i]
				if !inQuote && c == '(' && !inTuple {
					inTuple = true
					currentRow = nil
					currentVal.Reset()
					continue
				}
				if !inQuote && c == ')' && inTuple {
					inTuple = false
					val := strings.TrimSpace(currentVal.String())
					if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
						val = val[1 : len(val)-1]
					}
					val = strings.ReplaceAll(val, "''", "'")
					currentRow = append(currentRow, val)
					res = append(res, currentRow)
					continue
				}
				if inTuple {
					if c == '\'' {
						// Lookahead for ''
						if i+1 < len(valsStr) && valsStr[i+1] == '\'' {
							currentVal.WriteByte('\'')
							currentVal.WriteByte('\'')
							i++ // skip next
						} else {
							inQuote = !inQuote
							currentVal.WriteByte(c)
						}
					} else if !inQuote && c == ',' {
						val := strings.TrimSpace(currentVal.String())
						if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
							val = val[1 : len(val)-1]
						}
						val = strings.ReplaceAll(val, "''", "'")
						currentRow = append(currentRow, val)
						currentVal.Reset()
					} else {
						currentVal.WriteByte(c)
					}
				}
			}
			return res
		}

		if rows := extractInserts("projects"); len(rows) > 0 {
			parsed.Project.ProjectID = rows[0][0]
			parsed.Project.ProjectName = rows[0][1]
			parsed.Project.ProjectDescription = rows[0][2]
			parsed.Project.ProjectGoal = rows[0][3]
			parsed.Project.Status = rows[0][4]
		}
		if rows := extractInserts("categories"); len(rows) > 0 {
			for _, r := range rows {
				parsed.Categories = append(parsed.Categories, model.Category{
					CategoryID:       r[0],
					ProjectID:        r[1],
					ParentCategoryID: r[2],
					Name:             r[3],
					Description:      r[4],
					Goal:             r[5],
					Status:           r[6],
				})
			}
		}
		if rows := extractInserts("tasks"); len(rows) > 0 {
			for _, r := range rows {
				dur, _ := strconv.ParseInt(r[7], 10, 64)
				ac := r[8] == "1"
				parsed.Tasks = append(parsed.Tasks, model.Task{
					TaskID:                   r[0],
					CategoryID:               r[1],
					Name:                     r[2],
					Description:              r[3],
					Goal:                     r[4],
					Status:                   r[5],
					Priority:                 r[6],
					EstimatedDurationSeconds: dur,
					AutoClose:                ac,
				})
			}
		}
		if rows := extractInserts("subtasks"); len(rows) > 0 {
			for _, r := range rows {
				dur, _ := strconv.ParseInt(r[4], 10, 64)
				parsed.Subtasks = append(parsed.Subtasks, model.Subtask{
					SubtaskID:                r[0],
					TaskID:                   r[1],
					Name:                     r[2],
					Status:                   r[3],
					EstimatedDurationSeconds: dur,
				})
			}
		}
		if rows := extractInserts("sprints"); len(rows) > 0 {
			for _, r := range rows {
				dur, _ := strconv.ParseInt(r[4], 10, 64)
				owner := r[1]
				sp := model.Sprint{
					SprintID:                 r[0],
					Name:                     r[2],
					Status:                   r[3],
					EstimatedDurationSeconds: dur,
				}
				isTask := false
				for _, t := range parsed.Tasks {
					if t.TaskID == owner {
						isTask = true
						break
					}
				}
				if isTask {
					sp.TaskID = owner
				} else {
					sp.SubtaskID = owner
				}
				parsed.Sprints = append(parsed.Sprints, sp)
			}
		}

	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}

	return &parsed, nil
}

func extractArchiveFiles(fileBytes []byte, format string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	reader := bytes.NewReader(fileBytes)

	if format == "zip" {
		zr, err := zip.NewReader(reader, int64(len(fileBytes)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(rc)
			files[f.Name] = data
			rc.Close()
		}
	} else if format == "rar" {
		rr, err := rardecode.NewReader(reader)
		if err != nil {
			return nil, err
		}
		for {
			h, err := rr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if !h.IsDir {
				data, _ := io.ReadAll(rr)
				files[h.Name] = data
			}
		}
	} else {
		var tr *tar.Reader
		if format == "tgz" || format == "tar.gz" {
			gzr, err := gzip.NewReader(reader)
			if err != nil {
				return nil, err
			}
			defer gzr.Close()
			tr = tar.NewReader(gzr)
		} else {
			tr = tar.NewReader(reader)
		}
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if h.Typeflag == tar.TypeReg {
				data, _ := io.ReadAll(tr)
				files[h.Name] = data
			}
		}
	}
	return files, nil
}
