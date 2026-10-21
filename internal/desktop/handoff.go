package desktop

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// ProjectHandoff loads the bounded Project context snapshot through Core's
// existing API. The desktop client does not reconstruct or persist its contents.
func (c *CoreClient) ProjectHandoff(ctx context.Context, projectID string) (model.HandoffSnapshot, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return model.HandoffSnapshot{}, fmt.Errorf("project ID is required")
	}

	var snapshot model.HandoffSnapshot
	route := "/api/v1/projects/" + url.PathEscape(projectID) + "/handoff"
	if err := c.get(ctx, route, &snapshot); err != nil {
		return model.HandoffSnapshot{}, err
	}
	if snapshot.Project.ProjectID != projectID {
		return model.HandoffSnapshot{}, fmt.Errorf("Core returned a handoff snapshot for a different Project")
	}
	if snapshot.RecentEvents == nil {
		snapshot.RecentEvents = []model.ProjectEvent{}
	}
	if snapshot.RecentNotes == nil {
		snapshot.RecentNotes = []model.ProjectNote{}
	}
	if snapshot.RecentMessages == nil {
		snapshot.RecentMessages = []model.ProjectMessage{}
	}
	return snapshot, nil
}
