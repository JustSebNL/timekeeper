package desktop

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// ExecutionTree reads the persisted hierarchy for one Project through Core's
// existing read-only API. The desktop adapter never reconstructs hierarchy.
func (c *CoreClient) ExecutionTree(ctx context.Context, projectID string) (model.ExecutionTree, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return model.ExecutionTree{}, fmt.Errorf("project ID is required")
	}
	var tree model.ExecutionTree
	route := "/api/v1/projects/" + url.PathEscape(projectID) + "/execution-tree"
	if err := c.get(ctx, route, &tree); err != nil {
		return model.ExecutionTree{}, err
	}
	if tree.Project.ProjectID == "" || tree.Project.ProjectID != projectID {
		return model.ExecutionTree{}, fmt.Errorf("Core returned an execution tree for a different Project")
	}
	if tree.Categories == nil {
		return model.ExecutionTree{}, fmt.Errorf("Core returned no category array for the Project execution tree")
	}
	return tree, nil
}
