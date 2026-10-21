// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

type treasureGemDetail struct {
	model.ProjectGem
	Attachments []model.ProjectGem `json:"attachments,omitempty"`
}

type treasureGemOptions struct {
	Category    string
	Subcategory string
	Tags        string
	SourceRef   string
	Author      string
	FilePath    string
	Content     string
}

func printTreasureChestHelp(out io.Writer) {
	fmt.Fprintln(out, "usage: tk tc <add|attach|edit|lookup|list|show|pin|unpin|delete|restore|categories|revisions> <project-id-or-alias> ...")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  add <project> <category> <name> <content...> [--subcategory S] [--file PATH] [--tags T] [--source REF]")
	fmt.Fprintln(out, "  attach <project> <gem-id> <name> <content...> [--category C] [--subcategory S] [--file PATH]")
	fmt.Fprintln(out, "  edit <project> <gem-id> [--name N] [--content TEXT|--file PATH] [--category C] [--subcategory S] [--tags T] [--source REF] [--author A]")
	fmt.Fprintln(out, "  lookup <project> <query...> [--category C] [--subcategory S] [--pinned] [--include-deleted] [--limit N]")
	fmt.Fprintln(out, "  list <project> [--category C] [--subcategory S] [--parent G-ID] [--pinned] [--include-deleted] [--limit N]")
	fmt.Fprintln(out, "  show <project> <gem-id> [--include-deleted]   Includes direct attached gems.")
	fmt.Fprintln(out, "  pin|unpin <project> <gem-id> | delete|restore <project> <gem-id>")
	fmt.Fprintln(out, "  categories <project> | revisions <project> <gem-id>")
	fmt.Fprintln(out, "Categories are persisted with the first gem that uses a label. Delete is reversible; gem content is stored in Core's dedicated SQLite table.")
	fmt.Fprintf(out, "Gem name/content limits: %d / %d characters.\n", model.MaxProjectGemNameLength, model.MaxProjectGemContentLength)
}

func treasureChest(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printTreasureChestHelp(out)
		return 0
	}
	if len(args) > 1 && (args[1] == "--help" || args[1] == "-h") {
		printTreasureChestHelp(out)
		return 0
	}
	if len(args) < 2 {
		fmt.Fprintln(errOut, "usage: tk tc <command> <project-id-or-alias> ...")
		return 2
	}
	switch args[0] {
	case "add", "attach", "edit", "lookup", "list", "show", "pin", "unpin", "delete", "restore", "categories", "revisions":
	default:
		fmt.Fprintf(errOut, "unknown Treasure Chest command %q\n", args[0])
		return 2
	}
	projectID, err := resolveProjectID(baseURL, args[1])
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	switch args[0] {
	case "add":
		return treasureGemAdd(projectID, args[2:], out, errOut, baseURL)
	case "attach":
		return treasureGemAttach(projectID, args[2:], out, errOut, baseURL)
	case "edit":
		return treasureGemEdit(projectID, args[2:], out, errOut, baseURL)
	case "lookup":
		return treasureGemList(projectID, args[2:], true, out, errOut, baseURL)
	case "list":
		return treasureGemList(projectID, args[2:], false, out, errOut, baseURL)
	case "show":
		return treasureGemShow(projectID, args[2:], out, errOut, baseURL)
	case "pin":
		return treasureGemPin(projectID, args[2:], true, out, errOut, baseURL)
	case "unpin":
		return treasureGemPin(projectID, args[2:], false, out, errOut, baseURL)
	case "delete":
		return treasureGemDelete(projectID, args[2:], false, out, errOut, baseURL)
	case "restore":
		return treasureGemDelete(projectID, args[2:], true, out, errOut, baseURL)
	case "categories":
		return treasureGemCategories(projectID, args[2:], out, errOut, baseURL)
	case "revisions":
		return treasureGemRevisions(projectID, args[2:], out, errOut, baseURL)
	default:
		fmt.Fprintf(errOut, "unsupported Treasure Chest command %q\n", args[0])
		return 2
	}
}

func parseGemID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.ToUpper(value), "G-")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("gem id must be a positive integer or G-<id>")
	}
	return id, nil
}

func treasureBaseURL(baseURL, projectID string) string {
	return strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + url.PathEscape(projectID) + "/treasure-chest"
}

func treasureGemURL(baseURL, projectID string, gemID int64) string {
	return treasureBaseURL(baseURL, projectID) + "/gems/" + strconv.FormatInt(gemID, 10)
}

func treasureRequest(method, endpoint string, payload any, actor string) ([]byte, int, error) {
	var requestBody io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, fmt.Errorf("encode request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, endpoint, requestBody)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if actor != "" {
		request.Header.Set("X-Agent-ID", actor)
	}
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, response.StatusCode, err
	}
	return body, response.StatusCode, nil
}

func treasureRequestError(operation string, status int, body []byte, err error, errOut io.Writer) int {
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", operation, err)
		return 1
	}
	statusText := fmt.Sprintf("%d %s", status, http.StatusText(status))
	fmt.Fprintf(errOut, "%s: %s\n", operation, formatAPIError(statusText, body, ""))
	return 1
}

func decodeTreasureGem(body []byte) (model.ProjectGem, error) {
	var gem model.ProjectGem
	if err := json.Unmarshal(body, &gem); err != nil {
		return model.ProjectGem{}, fmt.Errorf("decode gem response: %w", err)
	}
	return gem, nil
}

func readTreasureGem(baseURL, projectID string, gemID int64, includeDeleted bool) (model.ProjectGem, error) {
	endpoint := treasureGemURL(baseURL, projectID, gemID)
	if includeDeleted {
		endpoint += "?include_deleted=true"
	}
	body, status, err := treasureRequest(http.MethodGet, endpoint, nil, "")
	if err != nil || status != http.StatusOK {
		return model.ProjectGem{}, fmt.Errorf("%s", treasureHTTPError(status, body, err))
	}
	return decodeTreasureGem(body)
}

func treasureHTTPError(status int, body []byte, err error) string {
	if err != nil {
		return err.Error()
	}
	statusText := fmt.Sprintf("%d %s", status, http.StatusText(status))
	return formatAPIError(statusText, body, "")
}

func writeTreasureGem(out io.Writer, gem model.ProjectGem) {
	if jsonOutputEnabled {
		writeJSONResponse(out, gem)
		return
	}
	path := gem.Category
	if gem.Subcategory != "" {
		path += "/" + gem.Subcategory
	}
	fmt.Fprintf(out, "%s\t[%s]\t%s\n", gem.Ref, path, gem.Name)
}

func parseGemContentTail(args []string, options *treasureGemOptions) error {
	contentParts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "--subcategory", "--category", "--tags", "--source", "--author", "--file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", flag)
			}
			i++
			value := args[i]
			switch flag {
			case "--subcategory": options.Subcategory = strings.TrimSpace(value)
			case "--category": options.Category = strings.TrimSpace(value)
			case "--tags": options.Tags = strings.TrimSpace(value)
			case "--source": options.SourceRef = strings.TrimSpace(value)
			case "--author": options.Author = strings.TrimSpace(value)
			case "--file": options.FilePath = value
			}
		case "--help", "-h":
			return fmt.Errorf("help requested")
		default:
			if strings.HasPrefix(flag, "-") {
				return fmt.Errorf("unknown Treasure Chest option %q", flag)
			}
			contentParts = append(contentParts, flag)
		}
	}
	if options.FilePath != "" && len(contentParts) > 0 {
		return fmt.Errorf("use either content arguments or --file, not both")
	}
	options.Content = strings.TrimSpace(strings.Join(contentParts, " "))
	if options.FilePath != "" {
		data, err := os.ReadFile(options.FilePath)
		if err != nil {
			return fmt.Errorf("read gem content file %q: %w", options.FilePath, err)
		}
		options.Content = strings.TrimSpace(string(data))
	}
	if options.Content == "" {
		return fmt.Errorf("gem content is required; pass it after the name or use --file")
	}
	return nil
}

func treasureGemAdd(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 2 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(errOut, "usage: tk tc add <project> <category> <name> <content...> [--subcategory S] [--file PATH] [--tags T] [--source REF]")
		return 2
	}
	options := treasureGemOptions{Category: strings.TrimSpace(args[0])}
	name := strings.TrimSpace(args[1])
	if name == "" || strings.ContainsAny(name, "\r\n") {
		fmt.Fprintln(errOut, "gem name is required and must be one line")
		return 2
	}
	if err := parseGemContentTail(args[2:], &options); err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return 2
	}
	input := model.CreateProjectGemInput{Category: options.Category, Subcategory: options.Subcategory, Name: name, Content: options.Content, Tags: options.Tags, SourceRef: options.SourceRef, CreatedBy: options.Author}
	body, status, err := treasureRequest(http.MethodPost, treasureBaseURL(baseURL, projectID)+"/gems", input, options.Author)
	if err != nil || status != http.StatusCreated {
		return treasureRequestError("add Treasure Chest gem", status, body, err, errOut)
	}
	gem, err := decodeTreasureGem(body)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return 1
	}
	writeTreasureGem(out, gem)
	return 0
}

func treasureGemAttach(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 2 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(errOut, "usage: tk tc attach <project> <parent-gem-id> <name> <content...> [--category C] [--subcategory S] [--file PATH]")
		return 2
	}
	parentID, err := parseGemID(args[0])
	if err != nil { fmt.Fprintln(errOut, err); return 2 }
	parent, err := readTreasureGem(baseURL, projectID, parentID, false)
	if err != nil { fmt.Fprintf(errOut, "read parent gem: %v\n", err); return 1 }
	if parent.DeletedAt != nil { fmt.Fprintln(errOut, "cannot attach a gem to a deleted parent; restore it first"); return 2 }
	name := strings.TrimSpace(args[1])
	if name == "" || strings.ContainsAny(name, "\r\n") { fmt.Fprintln(errOut, "gem name is required and must be one line"); return 2 }
	options := treasureGemOptions{Category: parent.Category, Subcategory: parent.Subcategory}
	if err := parseGemContentTail(args[2:], &options); err != nil { fmt.Fprintf(errOut, "%v\n", err); return 2 }
	input := model.CreateProjectGemInput{ParentGemID: &parent.GemID, Category: options.Category, Subcategory: options.Subcategory, Name: name, Content: options.Content, Tags: options.Tags, SourceRef: options.SourceRef, CreatedBy: options.Author}
	body, status, err := treasureRequest(http.MethodPost, treasureBaseURL(baseURL, projectID)+"/gems", input, options.Author)
	if err != nil || status != http.StatusCreated { return treasureRequestError("attach Treasure Chest gem", status, body, err, errOut) }
	gem, err := decodeTreasureGem(body)
	if err != nil { fmt.Fprintf(errOut, "%v\n", err); return 1 }
	writeTreasureGem(out, gem)
	return 0
}

func treasureGemEdit(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 2 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(errOut, "usage: tk tc edit <project> <gem-id> [--name N] [--content TEXT|--file PATH] [--category C] [--subcategory S] [--tags T] [--source REF] [--author A]")
		return 2
	}
	gemID, err := parseGemID(args[0])
	if err != nil { fmt.Fprintln(errOut, err); return 2 }
	current, err := readTreasureGem(baseURL, projectID, gemID, false)
	if err != nil { fmt.Fprintf(errOut, "read gem: %v\n", err); return 1 }
	input := model.UpdateProjectGemInput{Category: current.Category, Subcategory: current.Subcategory, Name: current.Name, Content: current.Content, Tags: current.Tags, SourceRef: current.SourceRef}
	options := treasureGemOptions{}
	nameSet, contentSet := false, false
	for i := 1; i < len(args); i++ {
		flag := args[i]
		if flag == "--help" || flag == "-h" { fmt.Fprintln(out, "usage: tk tc edit <project> <gem-id> [--name N] [--content TEXT|--file PATH] [--category C] [--subcategory S] [--tags T] [--source REF] [--author A]"); return 0 }
		if i+1 >= len(args) { fmt.Fprintf(errOut, "%s requires a value\n", flag); return 2 }
		i++
		value := args[i]
		switch flag {
		case "--name": input.Name, nameSet = strings.TrimSpace(value), true
		case "--content": if options.FilePath != "" { fmt.Fprintln(errOut, "use either --content or --file, not both"); return 2 }; options.Content, contentSet = strings.TrimSpace(value), true
		case "--file": if contentSet || strings.TrimSpace(value) == "" { fmt.Fprintln(errOut, "--file needs a path and cannot be combined with --content"); return 2 }; options.FilePath, contentSet = value, true
		case "--category": input.Category = strings.TrimSpace(value)
		case "--subcategory": input.Subcategory = strings.TrimSpace(value)
		case "--tags": input.Tags = strings.TrimSpace(value)
		case "--source": input.SourceRef = strings.TrimSpace(value)
		case "--author": input.EditedBy = strings.TrimSpace(value)
		default: fmt.Fprintf(errOut, "unknown Treasure Chest edit option %q\n", flag); return 2
		}
	}
	if options.FilePath != "" {
		data, readErr := os.ReadFile(options.FilePath)
		if readErr != nil { fmt.Fprintf(errOut, "read gem content file %q: %v\n", options.FilePath, readErr); return 2 }
		input.Content = strings.TrimSpace(string(data))
	}
	if contentSet && options.FilePath == "" { input.Content = options.Content }
	if !nameSet && !contentSet && input.Category == current.Category && input.Subcategory == current.Subcategory && input.Tags == current.Tags && input.SourceRef == current.SourceRef {
		fmt.Fprintln(errOut, "edit requires at least one field")
		return 2
	}
	body, status, err := treasureRequest(http.MethodPatch, treasureGemURL(baseURL, projectID, gemID), input, input.EditedBy)
	if err != nil || status != http.StatusOK { return treasureRequestError("edit Treasure Chest gem", status, body, err, errOut) }
	gem, err := decodeTreasureGem(body)
	if err != nil { fmt.Fprintf(errOut, "%v\n", err); return 1 }
	writeTreasureGem(out, gem)
	return 0
}

func treasureGemList(projectID string, args []string, search bool, out, errOut io.Writer, baseURL string) int {
	options := model.ProjectGemFilter{Limit: model.DefaultProjectGemListLimit}
	queryParts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "--category", "--subcategory", "--parent", "--limit":
			if i+1 >= len(args) { fmt.Fprintf(errOut, "%s requires a value\n", flag); return 2 }
			i++
			value := args[i]
			switch flag {
			case "--category": options.Category = value
			case "--subcategory": options.Subcategory = value
			case "--parent": id, err := parseGemID(value); if err != nil { fmt.Fprintln(errOut, err); return 2 }; options.ParentGemID = &id
			case "--limit": limit, err := strconv.Atoi(value); if err != nil || limit < 1 || limit > model.MaxProjectGemListLimit { fmt.Fprintf(errOut, "--limit must be between 1 and %d\n", model.MaxProjectGemListLimit); return 2 }; options.Limit = limit
			}
		case "--pinned": options.PinnedOnly = true
		case "--include-deleted": options.IncludeDeleted = true
		case "--help", "-h": printTreasureChestHelp(out); return 0
		default:
			if strings.HasPrefix(flag, "-") { fmt.Fprintf(errOut, "unknown Treasure Chest option %q\n", flag); return 2 }
			if search { queryParts = append(queryParts, flag) } else { fmt.Fprintf(errOut, "unexpected argument %q\n", flag); return 2 }
		}
	}
	if search {
		options.Query = strings.TrimSpace(strings.Join(queryParts, " "))
		if options.Query == "" { fmt.Fprintln(errOut, "lookup requires a search query"); return 2 }
	}
	endpoint := treasureBaseURL(baseURL, projectID)+"/gems"
	if search { endpoint += "/search" }
	query := url.Values{}
	if options.Query != "" { query.Set("q", options.Query) }
	if options.Category != "" { query.Set("category", options.Category) }
	if options.Subcategory != "" { query.Set("subcategory", options.Subcategory) }
	if options.ParentGemID != nil { query.Set("parent_gem_id", strconv.FormatInt(*options.ParentGemID, 10)) }
	if options.PinnedOnly { query.Set("pinned", "true") }
	if options.IncludeDeleted { query.Set("include_deleted", "true") }
	query.Set("limit", strconv.Itoa(options.Limit))
	body, status, err := treasureRequest(http.MethodGet, endpoint+"?"+query.Encode(), nil, "")
	if err != nil || status != http.StatusOK { return treasureRequestError("list Treasure Chest gems", status, body, err, errOut) }
	var result model.ProjectGemList
	if err := json.Unmarshal(body, &result); err != nil { fmt.Fprintf(errOut, "decode gem list: %v\n", err); return 1 }
	writeJSONResponse(out, result)
	return 0
}

func treasureGemShow(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 { fmt.Fprintln(errOut, "usage: tk tc show <project> <gem-id> [--include-deleted]"); return 2 }
	gemID, err := parseGemID(args[0])
	if err != nil { fmt.Fprintln(errOut, err); return 2 }
	includeDeleted := false
	for _, arg := range args[1:] { if arg == "--include-deleted" { includeDeleted = true } else { fmt.Fprintf(errOut, "unknown Treasure Chest show option %q\n", arg); return 2 } }
	gem, err := readTreasureGem(baseURL, projectID, gemID, includeDeleted)
	if err != nil { fmt.Fprintf(errOut, "read gem: %v\n", err); return 1 }
	query := url.Values{"parent_gem_id": []string{strconv.FormatInt(gemID, 10)}, "include_deleted": []string{strconv.FormatBool(includeDeleted)}, "limit": []string{strconv.Itoa(model.MaxProjectGemListLimit)}}
	body, status, err := treasureRequest(http.MethodGet, treasureBaseURL(baseURL, projectID)+"/gems?"+query.Encode(), nil, "")
	if err != nil || status != http.StatusOK { return treasureRequestError("read gem attachments", status, body, err, errOut) }
	var children model.ProjectGemList
	if err := json.Unmarshal(body, &children); err != nil { fmt.Fprintf(errOut, "decode gem attachments: %v\n", err); return 1 }
	writeJSONResponse(out, treasureGemDetail{ProjectGem: gem, Attachments: children.Items})
	return 0
}

func treasureGemCategories(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 0 { fmt.Fprintf(errOut, "usage: tk tc categories <project>\n"); return 2 }
	body, status, err := treasureRequest(http.MethodGet, treasureBaseURL(baseURL, projectID)+"/categories", nil, "")
	if err != nil || status != http.StatusOK { return treasureRequestError("list Treasure Chest categories", status, body, err, errOut) }
	io.WriteString(out, string(body)); io.WriteString(out, "\n")
	return 0
}

func treasureGemPin(projectID string, args []string, pinned bool, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 { fmt.Fprintln(errOut, "usage: tk tc pin|unpin <project> <gem-id>"); return 2 }
	gemID, err := parseGemID(args[0]); if err != nil { fmt.Fprintln(errOut, err); return 2 }
	body, status, err := treasureRequest(http.MethodPost, treasureGemURL(baseURL, projectID, gemID)+"/pin", map[string]bool{"pinned": pinned}, "")
	if err != nil || status != http.StatusOK { return treasureRequestError("pin Treasure Chest gem", status, body, err, errOut) }
	gem, err := decodeTreasureGem(body); if err != nil { fmt.Fprintf(errOut, "%v\n", err); return 1 }
	writeTreasureGem(out, gem)
	return 0
}

func treasureGemDelete(projectID string, args []string, restore bool, out, errOut io.Writer, baseURL string) int {
	verb := "delete"; endpoint := "delete"
	if restore { verb, endpoint = "restore", "restore" }
	if len(args) != 1 { fmt.Fprintf(errOut, "usage: tk tc %s <project> <gem-id>\n", verb); return 2 }
	gemID, err := parseGemID(args[0]); if err != nil { fmt.Fprintln(errOut, err); return 2 }
	body, status, err := treasureRequest(http.MethodPost, treasureGemURL(baseURL, projectID, gemID)+"/"+endpoint, map[string]any{}, "")
	if err != nil || status != http.StatusOK { return treasureRequestError(verb+" Treasure Chest gem", status, body, err, errOut) }
	gem, err := decodeTreasureGem(body); if err != nil { fmt.Fprintf(errOut, "%v\n", err); return 1 }
	writeTreasureGem(out, gem)
	return 0
}

func treasureGemRevisions(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 { fmt.Fprintln(errOut, "usage: tk tc revisions <project> <gem-id>"); return 2 }
	gemID, err := parseGemID(args[0]); if err != nil { fmt.Fprintln(errOut, err); return 2 }
	body, status, err := treasureRequest(http.MethodGet, treasureGemURL(baseURL, projectID, gemID)+"/revisions", nil, "")
	if err != nil || status != http.StatusOK { return treasureRequestError("list gem revisions", status, body, err, errOut) }
	io.WriteString(out, string(body)); io.WriteString(out, "\n")
	return 0
}
