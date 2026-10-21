package desktop

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// GlobalSearch delegates query semantics and ranking to the authoritative Core.
func (c *CoreClient) GlobalSearch(ctx context.Context, query string, limit int, scopes []string) (model.GlobalSearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return model.GlobalSearchResponse{}, fmt.Errorf("search query is required")
	}
	if limit <= 0 {
		limit = model.DefaultGlobalSearchLimit
	}
	if limit > model.MaxGlobalSearchLimit {
		limit = model.MaxGlobalSearchLimit
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("limit", strconv.Itoa(limit))
	for _, scope := range scopes {
		if scope = strings.TrimSpace(scope); scope != "" {
			params.Add("scope", scope)
		}
	}

	var response model.GlobalSearchResponse
	route := "/api/v1/search?" + params.Encode()
	if err := c.get(ctx, route, &response); err != nil {
		return model.GlobalSearchResponse{}, err
	}
	if response.Items == nil {
		response.Items = []model.GlobalSearchResult{}
	}
	if response.Scopes == nil {
		response.Scopes = []string{}
	}
	return response, nil
}
