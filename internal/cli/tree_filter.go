package cli

import (
	"net/http"
	"strconv"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func filterExecutionTree(tree model.ExecutionTree, taskID, status string) (model.ExecutionTree, bool) {
	return model.FilterExecutionTree(tree, taskID, status)
}

func countExecutionTasks(tree model.ExecutionTree) int {
	return model.CountExecutionTreeTasks(tree)
}

func limitExecutionTreeTasks(tree model.ExecutionTree, limit int) (model.ExecutionTree, int, bool) {
	return model.LimitExecutionTreeTasks(tree, limit)
}

func limitExecutionTreeDepth(tree model.ExecutionTree, maxDepth int) model.ExecutionTree {
	return model.LimitExecutionTreeDepth(tree, maxDepth)
}

func filterExecutionTreeToActiveSprints(tree model.ExecutionTree) (model.ExecutionTree, bool) {
	return model.FilterExecutionTreeToActiveSprints(tree)
}

func executionTreeLimitMetadata(headers http.Header, limit int) (total, shown int, truncated, valid bool) {
	if limit <= 0 {
		return 0, 0, false, false
	}
	var err error
	if total, err = strconv.Atoi(headers.Get("X-TimeKeeper-Execution-Tree-Total-Tasks")); err != nil {
		return 0, 0, false, false
	}
	if shown, err = strconv.Atoi(headers.Get("X-TimeKeeper-Execution-Tree-Shown-Tasks")); err != nil {
		return 0, 0, false, false
	}
	if truncated, err = strconv.ParseBool(headers.Get("X-TimeKeeper-Execution-Tree-Truncated")); err != nil {
		return 0, 0, false, false
	}
	if total < 0 || shown < 0 || shown > total || shown > limit || truncated != (total > shown) {
		return 0, 0, false, false
	}
	return total, shown, truncated, true
}
