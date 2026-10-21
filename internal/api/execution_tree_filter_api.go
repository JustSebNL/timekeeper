package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

type executionTreeFilterOptions struct {
	taskID string
	status string
	active bool
	depth  int
	limit  int
}

func parseExecutionTreeFilterOptions(query url.Values) (executionTreeFilterOptions, error) {
	var options executionTreeFilterOptions
	var err error
	if options.taskID, _, err = singleExecutionTreeQueryValue(query, "task_id"); err != nil {
		return options, err
	}
	options.taskID = strings.TrimSpace(options.taskID)
	if len(options.taskID) > 128 {
		return options, fmt.Errorf("task_id is too long")
	}
	if options.status, _, err = singleExecutionTreeQueryValue(query, "status"); err != nil {
		return options, err
	}
	options.status = strings.TrimSpace(options.status)
	if len(options.status) > 64 {
		return options, fmt.Errorf("status is too long")
	}
	active, present, err := singleExecutionTreeQueryValue(query, "active")
	if err != nil {
		return options, err
	}
	if present {
		options.active, err = strconv.ParseBool(active)
		if err != nil {
			return options, fmt.Errorf("active must be true or false")
		}
	}
	if raw, present, err := singleExecutionTreeQueryValue(query, "depth"); err != nil {
		return options, err
	} else if present {
		options.depth, err = strconv.Atoi(raw)
		if err != nil || options.depth < 1 || options.depth > 10 {
			return options, fmt.Errorf("depth must be between 1 and 10")
		}
	}
	if raw, present, err := singleExecutionTreeQueryValue(query, "limit"); err != nil {
		return options, err
	} else if present {
		options.limit, err = strconv.Atoi(raw)
		if err != nil || options.limit < 1 || options.limit > 200 {
			return options, fmt.Errorf("limit must be between 1 and 200")
		}
	}
	return options, nil
}

func singleExecutionTreeQueryValue(query url.Values, key string) (string, bool, error) {
	values, present := query[key]
	if !present {
		return "", false, nil
	}
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return "", true, fmt.Errorf("%s must be provided exactly once with a value", key)
	}
	return values[0], true, nil
}

func applyExecutionTreeFilterOptions(tree model.ExecutionTree, options executionTreeFilterOptions) (model.ExecutionTree, int, int, bool) {
	if options.taskID != "" || options.status != "" {
		filtered, found := model.FilterExecutionTree(tree, options.taskID, options.status)
		if !found {
			tree.Categories = make([]model.ExecutionCategory, 0)
			return tree, 0, 0, false
		}
		tree = filtered
	}
	if options.active {
		filtered, found := model.FilterExecutionTreeToActiveSprints(tree)
		if !found {
			tree.Categories = make([]model.ExecutionCategory, 0)
			return tree, 0, 0, false
		}
		tree = filtered
	}
	if options.depth > 0 {
		tree = model.LimitExecutionTreeDepth(tree, options.depth)
	}
	total := model.CountExecutionTreeTasks(tree)
	shown := total
	truncated := false
	if options.limit > 0 {
		tree, shown, truncated = model.LimitExecutionTreeTasks(tree, options.limit)
	}
	return tree, total, shown, truncated
}
