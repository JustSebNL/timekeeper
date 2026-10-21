package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func globalSearchCLI(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		printGlobalSearchHelp(out)
		return 0
	}
	var queryParts, scopes []string
	projectID := ""
	kind, after, before := "", "", ""
	var tagFilters []string
	limit := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--projects":
			scopes = append(scopes, model.GlobalSearchScopeProjects)
		case "--tasks":
			scopes = append(scopes, model.GlobalSearchScopeTasks)
		case "--messages":
			scopes = append(scopes, model.GlobalSearchScopeMessages)
		case "--project":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--project requires a Project ID")
				return 2
			}
			i++
			projectID = strings.TrimSpace(args[i])
			if projectID == "" {
				fmt.Fprintln(errOut, "--project requires a Project ID")
				return 2
			}
		case "--limit":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--limit requires a value")
				return 2
			}
			i++
			parsed, err := strconv.Atoi(args[i])
			if err != nil || parsed < 1 || parsed > model.MaxGlobalSearchLimit {
				fmt.Fprintf(errOut, "--limit must be between 1 and %d\n", model.MaxGlobalSearchLimit)
				return 2
			}
			limit = parsed
		case "--kind", "--tags", "--after", "--before":
			if i+1 >= len(args) {
				fmt.Fprintf(errOut, "%s requires a value\n", args[i])
				return 2
			}
			flag := args[i]
			i++
			switch flag {
			case "--kind":
				kind = strings.TrimSpace(args[i])
			case "--tags":
				for _, rawTag := range strings.Split(args[i], ",") {
					if tag := strings.TrimSpace(rawTag); tag != "" {
						tagFilters = append(tagFilters, tag)
					}
				}
			case "--after":
				after = strings.TrimSpace(args[i])
			case "--before":
				before = strings.TrimSpace(args[i])
			}
		case "--help", "-h":
			printGlobalSearchHelp(out)
			return 0
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(errOut, "unknown search option %q\n", args[i])
				return 2
			}
			queryParts = append(queryParts, args[i])
		}
	}
	query := strings.TrimSpace(strings.Join(queryParts, " "))
	if query == "" {
		fmt.Fprintln(errOut, "usage: tk search <query...> [--projects|--tasks|--messages] [--project <P-ID>] [--limit N]")
		return 2
	}
	values := url.Values{}
	values.Set("q", query)
	if projectID != "" {
		values.Set("project", projectID)
	}
	if limit != 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if kind != "" {
		values.Set("kind", kind)
	}
	if len(tagFilters) > 0 {
		values.Set("tags", strings.Join(tagFilters, ","))
	}
	if after != "" {
		values.Set("after", after)
	}
	if before != "" {
		values.Set("before", before)
	}
	for _, scope := range scopes {
		values.Add("scope", scope)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/search?" + values.Encode()
	body, status, err := getJSON(endpoint)
	if err != nil {
		fmt.Fprintf(errOut, "search Core API: %s\n", boundedDiagnosticText(err.Error(), 2048))
		return 1
	}
	if !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "search: %s\n", formatAPIError(status, []byte(body), ""))
		return 1
	}
	var response model.GlobalSearchResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		fmt.Fprintf(errOut, "search returned invalid JSON: %s\n", boundedDiagnosticText(err.Error(), 1024))
		return 1
	}
	if jsonOutputEnabled {
		writeJSONResponse(out, response)
		return 0
	}
	countLabel := "matches"
	if response.Total == 1 {
		countLabel = "match"
	}
	fmt.Fprintf(out, "%d %s; showing first %d\n", response.Total, countLabel, len(response.Items))
	for _, item := range response.Items {
		snippet := searchOneLine(item.Snippet)
		switch item.Type {
		case "project":
			fmt.Fprintf(out, "Project %s | %s | %s | %s\n", item.Ref, searchOneLine(item.Title), item.Status, snippet)
		case "task":
			fmt.Fprintf(out, "Task %s | %s (%s) | %s | %s | %s\n", item.Ref, searchOneLine(item.ProjectName), item.ProjectID, item.Status, searchOneLine(item.Title), snippet)
		case "message":
			fmt.Fprintf(out, "Message %s | %s (%s) | %s | %s | tags=%s | %s\n", item.Ref, searchOneLine(item.ProjectName), item.ProjectID, item.CreatedAt.Format("2006-01-02"), item.Kind, searchOneLine(item.Tags), snippet)
		}
	}
	return 0
}

func printGlobalSearchHelp(out io.Writer) {
	fmt.Fprintln(out, "usage: tk search <query...> [--projects|--tasks|--messages] [--project <P-ID>] [--limit N] [--kind K] [--tags T1,T2] [--after DATE] [--before DATE]")
	fmt.Fprintln(out, "Searches Projects, Tasks, and project messages across Time Keeper; add scope flags to narrow the search.")
	fmt.Fprintln(out, "Use --json for the structured result envelope.")
}

func searchOneLine(value string) string { return strings.Join(strings.Fields(value), " ") }
