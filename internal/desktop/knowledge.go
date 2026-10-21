package desktop

import (
	"context"

	"github.com/JustSebNL/timekeeper/internal/knowledge"
)

type knowledgeSearchResponse struct {
	Results []knowledge.KnowledgeResult `json:"results"`
}

// KnowledgeStatus reads the provider-neutral Memory Galaxy availability snapshot.
func (c *CoreClient) KnowledgeStatus(ctx context.Context) (knowledge.ProviderStatus, error) {
	var status knowledge.ProviderStatus
	if err := c.get(ctx, "/api/v1/knowledge/status", &status); err != nil {
		return knowledge.ProviderStatus{}, err
	}
	if status.Capabilities == nil {
		status.Capabilities = []string{}
	}
	return status, nil
}

// SearchKnowledge delegates a bounded cross-project query to the configured
// knowledge provider through Core; the desktop client does not rank results.
func (c *CoreClient) SearchKnowledge(ctx context.Context, query knowledge.KnowledgeQuery) ([]knowledge.KnowledgeResult, error) {
	var response knowledgeSearchResponse
	if err := c.post(ctx, "/api/v1/knowledge/search", query, &response); err != nil {
		return nil, err
	}
	if response.Results == nil {
		response.Results = []knowledge.KnowledgeResult{}
	}
	return response.Results, nil
}
