package desktop

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// ProjectUsageSummary reads Core's server-priced, project-scoped usage projection.
func (c *CoreClient) ProjectUsageSummary(ctx context.Context, projectID string) (model.ProjectUsageSummary, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return model.ProjectUsageSummary{}, fmt.Errorf("project ID is required")
	}

	var summary model.ProjectUsageSummary
	route := "/api/v1/projects/" + url.PathEscape(projectID) + "/usage-summary"
	if err := c.get(ctx, route, &summary); err != nil {
		return model.ProjectUsageSummary{}, err
	}
	if summary.ProjectID != projectID {
		return model.ProjectUsageSummary{}, fmt.Errorf("Core returned a usage summary for a different Project")
	}
	if summary.Sessions == nil {
		summary.Sessions = []model.UsageSession{}
	}
	if summary.Days == nil {
		summary.Days = []model.UsageDay{}
	}
	return summary, nil
}

// ProjectOperationalSummary reads Core's durable Sprint-operational snapshot.
func (c *CoreClient) ProjectOperationalSummary(ctx context.Context, projectID string) (model.ProjectOperationalSummary, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return model.ProjectOperationalSummary{}, fmt.Errorf("project ID is required")
	}

	var summary model.ProjectOperationalSummary
	route := "/api/v1/projects/" + url.PathEscape(projectID) + "/operational-summary"
	if err := c.get(ctx, route, &summary); err != nil {
		return model.ProjectOperationalSummary{}, err
	}
	if summary.ProjectID != projectID {
		return model.ProjectOperationalSummary{}, fmt.Errorf("Core returned an operational summary for a different Project")
	}
	return summary, nil
}
