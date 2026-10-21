package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func (s *Store) SearchProjectMessagesFiltered(ctx context.Context, projectID, query string, filter model.ProjectMessageSearchFilter) ([]model.ProjectMessageSearchHit, int, int, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > model.MaxGlobalSearchLimit {
		limit = model.MaxGlobalSearchLimit
	}
	response, err := s.SearchGlobal(ctx, model.GlobalSearchInput{
		Query: query, ProjectID: projectID, Scopes: []string{model.GlobalSearchScopeMessages},
		Limit: limit, Kind: filter.Kind, Tags: filter.Tags, After: filter.After, Before: filter.Before,
	})
	if err != nil {
		return nil, 0, 0, err
	}
	hits := make([]model.ProjectMessageSearchHit, 0, len(response.Items))
	for _, item := range response.Items {
		id, err := strconv.ParseInt(item.ObjectID, 10, 64)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("decode search message id %q: %w", item.ObjectID, err)
		}
		message, err := s.GetProjectMessage(ctx, projectID, id)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("hydrate project message %s: %w", item.Ref, err)
		}
		hits = append(hits, model.ProjectMessageSearchHit{ProjectMessage: message, Snippet: item.Snippet})
	}
	return hits, response.Total, limit, nil
}
