package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func warnForPossibleMessageDuplicates(projectID, body string, errOut io.Writer, baseURL string) {
	terms := messageDuplicateTokens(body)
	if len(terms) < 4 {
		return
	}
	probe := terms
	if len(probe) > 5 {
		probe = probe[:5]
	}
	query := url.Values{}
	query.Set("q", strings.Join(probe, " "))
	query.Set("limit", "5")
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/messages/search?" + query.Encode()
	responseBody, status, err := getJSON(endpoint)
	if err != nil {
		fmt.Fprintf(errOut, "note: duplicate check unavailable; continuing message add (%s)\n", boundedDiagnosticText(err.Error(), 512))
		return
	}
	if !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "note: duplicate check unavailable (Core returned %s); continuing message add\n", status)
		return
	}
	var response struct {
		Items []model.ProjectMessageSearchHit `json:"items"`
	}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		fmt.Fprintln(errOut, "note: duplicate check returned invalid JSON; continuing message add")
		return
	}
	for _, hit := range response.Items {
		score := messageDuplicateOverlap(terms, messageDuplicateTokens(hit.Body))
		if score < 0.75 {
			continue
		}
		ref := hit.Ref
		if ref == "" {
			ref = fmt.Sprintf("M-%d", hit.MessageID)
		}
		preview := strings.Join(strings.Fields(hit.Body), " ")
		if runes := []rune(preview); len(runes) > 120 {
			preview = string(runes[:120]) + "…"
		}
		fmt.Fprintf(errOut, "warning: possible duplicate %s (%s, %.0f%% token overlap): %s\n", ref, hit.Kind, score*100, preview)
	}
}

func messageDuplicateTokens(text string) []string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			normalized.WriteRune(r)
		} else {
			normalized.WriteByte(' ')
		}
	}
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, token := range strings.Fields(normalized.String()) {
		if !seen[token] {
			result = append(result, token)
			seen[token] = true
		}
	}
	return result
}

func messageDuplicateOverlap(left, right []string) float64 {
	if len(left) < 4 || len(right) < 4 {
		return 0
	}
	leftSet := make(map[string]bool, len(left))
	for _, token := range left {
		leftSet[token] = true
	}
	intersection := 0
	for _, token := range right {
		if leftSet[token] {
			intersection++
		}
	}
	union := len(leftSet)
	for _, token := range right {
		if !leftSet[token] {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
