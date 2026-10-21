package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func parseLegacyTaskJSON(data []byte) ([]model.CreateTaskInput, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return nil, fmt.Errorf("invalid task JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var root json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("invalid task JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("task JSON must contain exactly one document")
		}
		return nil, fmt.Errorf("invalid trailing task JSON: %w", err)
	}

	trimmed := bytes.TrimSpace(root)
	var rows []json.RawMessage
	switch {
	case len(trimmed) > 0 && trimmed[0] == '[':
		if err := json.Unmarshal(trimmed, &rows); err != nil {
			return nil, fmt.Errorf("tasks must be an array: %w", err)
		}
	case len(trimmed) > 0 && trimmed[0] == '{':
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, fmt.Errorf("task batch must be an object: %w", err)
		}
		for key := range envelope {
			if key != "tasks" {
				return nil, fmt.Errorf("unknown task-batch field %q; only 'tasks' is allowed", key)
			}
		}
		rawTasks, ok := envelope["tasks"]
		if !ok {
			return nil, fmt.Errorf("task batch object requires a 'tasks' array")
		}
		if err := json.Unmarshal(rawTasks, &rows); err != nil || rows == nil {
			return nil, fmt.Errorf("'tasks' must be an array")
		}
	default:
		return nil, fmt.Errorf("task batch must be a JSON array or an object containing 'tasks'")
	}

	tasks := make([]model.CreateTaskInput, 0, len(rows))
	for index, raw := range rows {
		task, err := parseLegacyTaskJSONRow(raw)
		if err != nil {
			return nil, fmt.Errorf("tasks[%d]: %w", index, err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func parseLegacyTaskJSONRow(raw json.RawMessage) (model.CreateTaskInput, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return model.CreateTaskInput{}, fmt.Errorf("task row must be an object")
	}
	for key := range fields {
		if _, ok := taskImportColumns[key]; !ok {
			return model.CreateTaskInput{}, fmt.Errorf("unknown field %q; remove it or use a documented task-import field", key)
		}
	}
	stringField := func(key string) (string, error) {
		rawValue, ok := fields[key]
		if !ok {
			return "", nil
		}
		if bytes.Equal(bytes.TrimSpace(rawValue), []byte("null")) {
			return "", fmt.Errorf("%s must be a string when provided", key)
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return "", fmt.Errorf("%s must be a string: %w", key, err)
		}
		return value, nil
	}

	var task model.CreateTaskInput
	var err error
	if task.Name, err = stringField("name"); err != nil {
		return task, err
	}
	if strings.TrimSpace(task.Name) == "" {
		return task, fmt.Errorf("name is required and must not be blank")
	}
	if task.CategoryID, err = stringField("category_id"); err != nil {
		return task, err
	}
	if task.Description, err = stringField("description"); err != nil {
		return task, err
	}
	if task.Goal, err = stringField("goal"); err != nil {
		return task, err
	}
	if task.Priority, err = stringField("priority"); err != nil {
		return task, err
	}
	if rawEstimate, ok := fields["estimate"]; ok {
		if _, alsoSeconds := fields["estimated_duration_seconds"]; alsoSeconds {
			return task, fmt.Errorf("use either estimate or estimated_duration_seconds, not both")
		}
		if bytes.Equal(bytes.TrimSpace(rawEstimate), []byte("null")) {
			return task, fmt.Errorf("estimate must be a duration string when provided")
		}
		var estimate string
		if err := json.Unmarshal(rawEstimate, &estimate); err != nil {
			return task, fmt.Errorf("estimate must be a duration string: %w", err)
		}
		task.EstimatedDurationSeconds, err = parseImportDuration(estimate)
		if err != nil {
			return task, err
		}
	}
	if rawSeconds, ok := fields["estimated_duration_seconds"]; ok {
		if bytes.Equal(bytes.TrimSpace(rawSeconds), []byte("null")) {
			return task, fmt.Errorf("estimated_duration_seconds must be a non-negative integer when provided")
		}
		if err := json.Unmarshal(rawSeconds, &task.EstimatedDurationSeconds); err != nil || task.EstimatedDurationSeconds < 0 {
			return task, fmt.Errorf("estimated_duration_seconds must be a non-negative integer")
		}
	}
	if rawAutoClose, ok := fields["auto_close"]; ok {
		if bytes.Equal(bytes.TrimSpace(rawAutoClose), []byte("null")) {
			return task, fmt.Errorf("auto_close must be a boolean when provided")
		}
		var value bool
		if err := json.Unmarshal(rawAutoClose, &value); err != nil {
			return task, fmt.Errorf("auto_close must be a boolean: %w", err)
		}
		task.AutoClose = &value
	}

	// Validate the types of documented export-only metadata even though import
	// deliberately does not restore IDs, workflow status, timestamps, or progress.
	for _, key := range []string{"task_id", "project_id", "item_address", "status", "created_at", "updated_at"} {
		value, err := stringField(key)
		if err != nil {
			return task, err
		}
		if value != "" && (key == "created_at" || key == "updated_at") {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return task, fmt.Errorf("%s must use RFC3339 timestamp format", key)
			}
		}
	}
	for _, key := range []string{"completion_pct", "reported_completion_pct", "calculated_completion_pct"} {
		if rawValue, ok := fields[key]; ok {
			if bytes.Equal(bytes.TrimSpace(rawValue), []byte("null")) {
				return task, fmt.Errorf("%s must be numeric when provided", key)
			}
			var value float64
			if err := json.Unmarshal(rawValue, &value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return task, fmt.Errorf("%s must be a finite number", key)
			}
		}
	}
	return task, nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var scan func() error
	scan = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate object key %q", key)
				}
				seen[key] = struct{}{}
				if err := scan(); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := scan(); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delim)
		}
	}
	if err := scan(); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("document contains trailing JSON data")
		}
		return err
	}
	return nil
}
