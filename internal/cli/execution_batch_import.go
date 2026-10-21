package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
	"gopkg.in/yaml.v3"
)

const executionBatchVersion = "timekeeper-execution-batch/v1"

func importTasksFromBytes(data []byte, format, filePath string) ([]model.CreateTaskInput, error) {
	switch format {
	case "csv":
		return parseCSV(bytes.NewReader(data), ',')
	case "tsv":
		return parseCSV(bytes.NewReader(data), '\t')
	case "xlsx":
		return parseExcel(data)
	case "json":
		return parseJSONOrYAML(data, true)
	case "yaml", "yml":
		return parseJSONOrYAML(data, false)
	case "sql":
		return parseSQL(data)
	case "zip":
		return parseZIP(data)
	case "tar":
		return parseTar(bytes.NewReader(data))
	case "tar.gz", "tgz":
		return parseTarGz(bytes.NewReader(data))
	case "rar":
		return parseRAR(data)
	case "gz":
		innerExt := filepath.Ext(strings.TrimSuffix(strings.ToLower(filePath), ".gz"))
		if innerExt == "" {
			return nil, fmt.Errorf("unknown inner format for .gz")
		}
		return parseGZ(bytes.NewReader(data), innerExt)
	default:
		return nil, fmt.Errorf("unsupported format: %q", format)
	}
}

// ParseExecutionBatch selects strict versioned JSON/YAML batches while leaving
// every legacy task-only format on its existing parser path.
func ParseExecutionBatch(filePath, format string) (model.ExecutionBatchInput, []model.CreateTaskInput, error) {
	var data []byte
	var err error
	if filePath == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(filePath)
	}
	if err != nil {
		return model.ExecutionBatchInput{}, nil, fmt.Errorf("read batch file: %w", err)
	}
	if format == "" {
		lowerPath := strings.ToLower(filePath)
		switch {
		case strings.HasSuffix(lowerPath, ".tar.gz"):
			format = "tar.gz"
		case strings.HasSuffix(lowerPath, ".tgz"):
			format = "tgz"
		default:
			format = strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
		}
		if filePath == "-" {
			format = "json"
		}
	}
	format = strings.ToLower(format)
	if format != "json" && format != "yaml" && format != "yml" {
		if filePath == "-" {
			tasks, err := importTasksFromBytes(data, format, filePath)
			return model.ExecutionBatchInput{}, tasks, err
		}
		tasks, err := ImportTasks(filePath, format)
		return model.ExecutionBatchInput{}, tasks, err
	}

	jsonBytes := data
	if format == "yaml" || format == "yml" {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		var document any
		if err := decoder.Decode(&document); err != nil {
			return model.ExecutionBatchInput{}, nil, fmt.Errorf("invalid YAML: %v; fix the reported syntax/location and remove duplicate mapping keys", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return model.ExecutionBatchInput{}, nil, fmt.Errorf("YAML batch must contain exactly one document")
			}
			return model.ExecutionBatchInput{}, nil, fmt.Errorf("invalid trailing YAML document: %w", err)
		}
		jsonBytes, err = json.Marshal(normalizeYAMLValue(document))
		if err != nil {
			return model.ExecutionBatchInput{}, nil, fmt.Errorf("convert YAML batch: %w; fix unsupported YAML values", err)
		}
	}

	if err := rejectDuplicateJSONKeys(jsonBytes); err != nil {
		return model.ExecutionBatchInput{}, nil, fmt.Errorf("invalid %s: %v; remove duplicate keys and keep one document", format, err)
	}
	var root any
	if err := json.Unmarshal(jsonBytes, &root); err != nil {
		return model.ExecutionBatchInput{}, nil, fmt.Errorf("invalid %s: %v; fix the reported syntax and ensure the document is one JSON/YAML value", format, err)
	}
	rootObject, isObject := root.(map[string]any)
	if !isObject {
		legacyTasks, err := parseJSONOrYAML(data, format == "json")
		return model.ExecutionBatchInput{}, legacyTasks, err
	}
	if _, versioned := rootObject["version"]; !versioned {
		if hasNestedBatchMarkers(rootObject) {
			return model.ExecutionBatchInput{}, nil, fmt.Errorf("version is required for nested batch fields; set version to %s", executionBatchVersion)
		}
		legacyTasks, err := parseJSONOrYAML(data, format == "json")
		return model.ExecutionBatchInput{}, legacyTasks, err
	}

	if err := validateExecutionBatchKeys(rootObject); err != nil {
		return model.ExecutionBatchInput{}, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(jsonBytes))
	decoder.DisallowUnknownFields()
	var input model.ExecutionBatchInput
	if err := decoder.Decode(&input); err != nil {
		return model.ExecutionBatchInput{}, nil, fmt.Errorf("invalid execution batch value: %v; fix the field type or value at the reported location", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return model.ExecutionBatchInput{}, nil, fmt.Errorf("batch file contains trailing data; keep exactly one document and remove the extra value")
	}
	return input, nil, nil
}

func hasNestedBatchMarkers(root map[string]any) bool {
	if _, ok := root["extensions"]; ok {
		return true
	}
	if _, ok := root["planning"]; ok {
		return true
	}
	if tasks, ok := root["tasks"].([]any); ok {
		for _, raw := range tasks {
			if task, ok := raw.(map[string]any); ok {
				if _, ok := task["subtasks"]; ok {
					return true
				}
				if _, ok := task["sprints"]; ok {
					return true
				}
			}
		}
	}
	return false
}

func normalizeYAMLValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		converted := make(map[string]any, len(current))
		for key, child := range current {
			converted[key] = normalizeYAMLValue(child)
		}
		return converted
	case map[any]any:
		converted := make(map[string]any, len(current))
		for key, child := range current {
			converted[fmt.Sprint(key)] = normalizeYAMLValue(child)
		}
		return converted
	case []any:
		converted := make([]any, len(current))
		for index, child := range current {
			converted[index] = normalizeYAMLValue(child)
		}
		return converted
	default:
		return value
	}
}

func validateExecutionBatchKeys(root map[string]any) error {
	if err := checkBatchKeys(root, "", "version", "tasks", "extensions", "planning"); err != nil {
		return err
	}
	if tasks, ok := root["tasks"].([]any); ok {
		for index, raw := range tasks {
			path := fmt.Sprintf("tasks[%d]", index)
			object, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := checkBatchKeys(object, path, "category_id", "name", "description", "goal", "priority", "estimated_duration_seconds", "auto_close", "sprints", "subtasks"); err != nil {
				return err
			}
			if err := checkBatchChildren(object, path); err != nil {
				return err
			}
		}
	}
	if extensions, ok := root["extensions"].([]any); ok {
		for index, raw := range extensions {
			path := fmt.Sprintf("extensions[%d]", index)
			object, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := checkBatchKeys(object, path, "task_id", "subtask_id", "subtasks", "sprints"); err != nil {
				return err
			}
			if err := checkBatchChildren(object, path); err != nil {
				return err
			}
		}
	}
	if planning, ok := root["planning"].(map[string]any); ok {
		if err := checkBatchKeys(planning, "planning", "executable_task_metadata", "deferred_items", "release_gates", "application_constraints"); err != nil {
			return err
		}
		if err := checkPlanningRecords(planning); err != nil {
			return err
		}
	}
	return nil
}

func checkBatchChildren(parent map[string]any, path string) error {
	if sprints, ok := parent["sprints"].([]any); ok {
		for index, raw := range sprints {
			object, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := checkBatchKeys(object, fmt.Sprintf("%s.sprints[%d]", path, index), "name", "description", "goal", "priority", "estimated_duration_seconds", "buffer_pct"); err != nil {
				return err
			}
		}
	}
	if subtasks, ok := parent["subtasks"].([]any); ok {
		for index, raw := range subtasks {
			object, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			subtaskPath := fmt.Sprintf("%s.subtasks[%d]", path, index)
			if err := checkBatchKeys(object, subtaskPath, "name", "description", "goal", "priority", "estimated_duration_seconds", "sprints"); err != nil {
				return err
			}
			if sprints, ok := object["sprints"].([]any); ok {
				for sprintIndex, rawSprint := range sprints {
					sprint, ok := rawSprint.(map[string]any)
					if !ok {
						continue
					}
					if err := checkBatchKeys(sprint, fmt.Sprintf("%s.sprints[%d]", subtaskPath, sprintIndex), "name", "description", "goal", "priority", "estimated_duration_seconds", "buffer_pct"); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func checkPlanningRecords(planning map[string]any) error {
	if records, ok := planning["executable_task_metadata"].([]any); ok {
		for index, raw := range records {
			if object, ok := raw.(map[string]any); ok {
				if err := checkBatchKeys(object, fmt.Sprintf("planning.executable_task_metadata[%d]", index), "task_index", "key", "phase", "phases", "goal", "acceptance", "condition"); err != nil {
					return err
				}
			}
		}
	}
	if records, ok := planning["deferred_items"].([]any); ok {
		for index, raw := range records {
			if object, ok := raw.(map[string]any); ok {
				if err := checkBatchKeys(object, fmt.Sprintf("planning.deferred_items[%d]", index), "key", "condition", "acceptance"); err != nil {
					return err
				}
			}
		}
	}
	if records, ok := planning["release_gates"].([]any); ok {
		for index, raw := range records {
			if object, ok := raw.(map[string]any); ok {
				if err := checkBatchKeys(object, fmt.Sprintf("planning.release_gates[%d]", index), "reference", "phase", "phases", "required_evidence"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkBatchKeys(object map[string]any, path string, allowed ...string) error {
	known := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		known[key] = struct{}{}
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := known[key]; !ok {
			fieldPath := key
			if path != "" {
				fieldPath = path + "." + key
			}
			return fmt.Errorf("unknown field %s; remove it or rename it to a supported field", fieldPath)
		}
	}
	return nil
}

// ValidateExecutionBatchDocument exposes the shared pure batch validation to
// the CLI while Core reuses the same rules before applying requests.
func ValidateExecutionBatchDocument(input model.ExecutionBatchInput) model.BatchValidationResult {
	return model.ValidateExecutionBatchDocument(input)
}
