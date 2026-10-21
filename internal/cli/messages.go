// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// messages dispatches tk msg <project-id-or-alias> <add|list|search|show|edit|checkpoint|pin|unpin|thread|context|revisions|seed>.
//
//	tk msg <project> add   [--kind <k>] [--author <a>] [--link <u>] [--tags <t>] [--reply-to M-1] <body...>
//	tk msg <project> edit  M-1 [--kind <k>] [--author <a>] [--link <u>] [--tags <t>] <body...>
//	tk msg <project> thread M-1
//	tk msg <project> revisions M-1
//	tk msg <project> list  [--kind <k>] [--limit <n>]
//	tk msg <project> search <query...> [--limit <n>]
//	tk msg <project> show  M-1
//
// Output is JSON for `list` / `search` / `show` so it composes with the
// rest of the CLI; `add` prints the created record as a one-liner.
func printMessagesHelp(out io.Writer) {
	fmt.Fprintln(out, "usage: tk msg <project-id-or-alias> <add|list|search|show|edit|checkpoint|pin|unpin|thread|context|revisions|seed> ...")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  add [--kind note|decision|observation|link|lesson|question|answer] [--author <id>] [--tags <t>] [--parent <id>] [--link <url>] <body...>")
	fmt.Fprintln(out, "  list [--kind <kind>] [--limit <n>]")
	fmt.Fprintln(out, "  search <query> [--limit <n>] [--kind <kind>] [--tags <tag,...>] [--after <date>] [--before <date>]")
	fmt.Fprintln(out, "  show <message-id>")
	fmt.Fprintln(out, "  edit <message-id> [--kind <kind>] [--body <text>] [--add-tags <t>] [--remove-tags <t>]")
	fmt.Fprintln(out, "  thread <message-id>")
	fmt.Fprintln(out, "  context <message-id>")
	fmt.Fprintln(out, "  revisions <message-id>")
	fmt.Fprintln(out, "  checkpoint --author <id> --entry kind|body|tags [--entry ...]")
	fmt.Fprintln(out, "  pin <message-id>")
	fmt.Fprintln(out, "  unpin <message-id>")
	fmt.Fprintln(out, "  seed [--author <id>] [--dry-run] [--yes]")
}

func messages(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: tk msg <project-id-or-alias> <add|list|search|show|edit|checkpoint|pin|unpin|thread|context|revisions|seed> ...")
		return 2
	}
	// Help must work before project resolution. `tk msg --help` is a
	// top-level command request, not a project id named --help.
	if args[0] == "--help" || args[0] == "-h" || (len(args) > 1 && (args[1] == "--help" || args[1] == "-h")) {
		printMessagesHelp(out)
		return 0
	}
	if len(args) < 2 {
		fmt.Fprintln(errOut, "usage: tk msg <project-id-or-alias> <add|list|search|show|edit|checkpoint|pin|unpin|thread|context|revisions|seed> ...")
		return 2
	}
	projectID, err := resolveProjectID(baseURL, args[0])
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	switch args[1] {
	case "add":
		return msgAdd(projectID, args[2:], out, errOut, baseURL)
	case "edit":
		return msgEdit(projectID, args[2:], out, errOut, baseURL)
	case "thread":
		return msgThread(projectID, args[2:], out, errOut, baseURL)
	case "context":
		return msgContext(projectID, args[2:], out, errOut, baseURL)
	case "checkpoint":
		return msgCheckpoint(projectID, args[2:], out, errOut, baseURL)
	case "pin":
		return msgPin(projectID, args[2:], true, out, errOut, baseURL)
	case "unpin":
		return msgPin(projectID, args[2:], false, out, errOut, baseURL)
	case "revisions":
		return msgRevisions(projectID, args[2:], out, errOut, baseURL)
	case "list":
		return msgList(projectID, args[2:], out, errOut, baseURL)
	case "search":
		return msgSearch(projectID, args[2:], out, errOut, baseURL)
	case "show":
		return msgShow(projectID, args[2:], out, errOut, baseURL)
	case "seed":
		return msgSeed(projectID, args[2:], out, errOut, baseURL)
	default:
		fmt.Fprintf(errOut, "unknown msg subcommand %q (use add|list|search|show|edit|checkpoint|pin|unpin|thread|context|revisions|seed)\n", args[1])
		return 2
	}
}

func msgAdd(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	var kind model.MessageKind = model.MessageKindNote
	var author, link, tags string
	var parentMessageID *int64
	var fileBody string
	bodyParts := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--kind":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--kind requires a value")
				return 2
			}
			i++
			kind = model.MessageKind(args[i])
			if !model.IsValidMessageKind(kind) {
				fmt.Fprintf(errOut, "invalid --kind %q\n", args[i])
				return 2
			}
		case "--author":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--author requires a value")
				return 2
			}
			i++
			author = args[i]
		case "--link":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--link requires a value")
				return 2
			}
			i++
			link = args[i]
		case "--tags":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--tags requires a value")
				return 2
			}
			i++
			tags = args[i]
		case "--reply-to":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--reply-to requires a message id")
				return 2
			}
			i++
			id, err := parseMessageID(args[i])
			if err != nil {
				fmt.Fprintln(errOut, err)
				return 2
			}
			parentMessageID = &id
		case "--file":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--file requires a path")
				return 2
			}
			i++
			content, err := os.ReadFile(args[i])
			if err != nil {
				fmt.Fprintf(errOut, "read file %q: %v\n", args[i], err)
				return 2
			}
			fileBody = string(content)
		case "--help", "-h":
			fmt.Fprintln(out, "usage: tk msg <project> add [--kind <k>] [--author <a>] [--link <u>] [--tags <t>] [--reply-to M-1] [--file <path>] <body...>")
			return 0
		default:
			bodyParts = append(bodyParts, a)
		}
		i++
	}
	body := strings.TrimSpace(strings.Join(bodyParts, " "))
	if fileBody != "" {
		body = strings.TrimSpace(fileBody)
	}
	if body == "" {
		fmt.Fprintln(errOut, "message body is required (pass it as the final argument or use --file)")
		return 2
	}
	input := model.CreateProjectMessageInput{Kind: kind, Author: author, Body: body, Link: link, Tags: tags, ParentMessageID: parentMessageID}
	warnForPossibleMessageDuplicates(projectID, body, errOut, baseURL)
	raw, _ := json.Marshal(input)
	postURL := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/messages"
	req, err := http.NewRequest(http.MethodPost, postURL, strings.NewReader(string(raw)))
	if err != nil {
		fmt.Fprintf(errOut, "build request: %v\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	if author != "" {
		req.Header.Set("X-Agent-ID", author)
	}
	resp, err := newHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		fmt.Fprintf(errOut, "post message: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "create message", resp)
		return 1
	}
	var created model.ProjectMessage
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		fmt.Fprintf(errOut, "decode response: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "M-%d\t%s\t%s\n", created.MessageID, created.Kind, truncate(created.Body, 80))
	return 0
}

func msgList(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	q := url.Values{}
	kind := ""
	limit := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--kind":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--kind requires a value")
				return 2
			}
			i++
			kind = args[i]
			q.Set("kind", kind)
		case "--limit":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--limit requires a value")
				return 2
			}
			i++
			limit = args[i]
			q.Set("limit", limit)
		case "--help", "-h":
			fmt.Fprintln(out, "usage: tk msg <project> list [--kind <k>] [--limit <n>]")
			return 0
		default:
			fmt.Fprintf(errOut, "unknown flag %q\n", args[i])
			return 2
		}
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/messages"
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	body, status, err := getJSON(endpoint)
	if err != nil {
		fmt.Fprintf(errOut, "list messages: %v\n", err)
		return 1
	}
	if !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "list messages: %s\n", formatAPIError(status, []byte(body), ""))
		return 1
	}
	io.WriteString(out, body)
	io.WriteString(out, "\n")
	return 0
}

func msgSearch(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, "usage: tk msg <project> search <query...> [--limit <n>] [--kind <kind>] [--tags <tag,...>] [--after <date>] [--before <date>]")
		return 0
	}
	limit, kind, after, before := "", "", "", ""
	var tags []string
	qParts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "--limit", "--kind", "--tags", "--after", "--before":
			if i+1 >= len(args) {
				fmt.Fprintf(errOut, "%s requires a value\n", flag)
				return 2
			}
			i++
			switch flag {
			case "--limit":
				limit = args[i]
			case "--kind":
				kind = args[i]
			case "--tags":
				for _, rawTag := range strings.Split(args[i], ",") {
					if tag := strings.TrimSpace(rawTag); tag != "" {
						tags = append(tags, tag)
					}
				}
			case "--after":
				after = args[i]
			case "--before":
				before = args[i]
			}
		case "--help", "-h":
			fmt.Fprintln(out, "usage: tk msg <project> search <query...> [--limit <n>] [--kind <kind>] [--tags <tag,...>] [--after <date>] [--before <date>]")
			return 0
		default:
			if strings.HasPrefix(flag, "-") {
				fmt.Fprintf(errOut, "unknown search option %q\n", flag)
				return 2
			}
			qParts = append(qParts, flag)
		}
	}
	q := strings.TrimSpace(strings.Join(qParts, " "))
	if q == "" {
		fmt.Fprintln(errOut, "usage: tk msg <project> search <query...> [--limit <n>] [--kind <kind>] [--tags <tag,...>] [--after <date>] [--before <date>]")
		return 2
	}
	qvals := url.Values{}
	qvals.Set("q", q)
	if limit != "" {
		qvals.Set("limit", limit)
	}
	if kind != "" {
		qvals.Set("kind", kind)
	}
	if len(tags) > 0 {
		qvals.Set("tags", strings.Join(tags, ","))
	}
	if after != "" {
		qvals.Set("after", after)
	}
	if before != "" {
		qvals.Set("before", before)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/messages/search?" + qvals.Encode()
	body, status, err := getJSON(endpoint)
	if err != nil {
		fmt.Fprintf(errOut, "search messages: %v\n", err)
		return 1
	}
	if !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "search messages: %s\n", formatAPIError(status, []byte(body), ""))
		return 1
	}
	io.WriteString(out, body)
	io.WriteString(out, "\n")
	return 0
}

// msgSeed posts a small, fixed set of example messages to a project so the
// dashboard panel has data to render. Idempotent at the semantic level: it
// just adds more messages. Each entry is small, distinct, and covers a
// different message kind so the kind filter and search box both have
// something to do.
func msgSeed(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	author := "xatia"
	confirmed := false
	dryRun := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--author":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--author requires a value")
				return 2
			}
			i++
			author = args[i]
		case "--yes":
			confirmed = true
		case "--dry-run":
			dryRun = true
		case "--help", "-h":
			fmt.Fprintln(out, "seed usage: tk msg <project> seed [--author <a>] [--dry-run] [--yes]")
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "  --dry-run  Print the sample messages that would be written, then exit.")
			fmt.Fprintln(out, "  --yes      Confirm the write. Seeding posts real messages to a real")
			fmt.Fprintln(out, "             project and cannot be undone, so it is refused by default.")
			return 0
		default:
			fmt.Fprintf(errOut, "unknown flag %q\n", args[i])
			return 2
		}
	}
	seeds := []struct {
		kind model.MessageKind
		body string
		tags string
		link string
	}{
		{model.MessageKindNote, "Project conventions: canonical port is 1618; canonical address is 127.0.0.1:1618; the friendly URLs are timekeeper.local and api.timekeeper.local once the installer has run.", "port,url,convention", ""},
		{model.MessageKindDecision, "Adopt NSSM for the Windows service installer; the kernel-level service recovery is the right primary mechanism. The 30-minute scheduled task is the bounded failure-only health kick, not a primary.", "service,architecture", ""},
		{model.MessageKindObservation, "When port 80 is already in use (Windows HTTP.sys / PID 4), the friendly-URL proxy logs a clean warning and the canonical 1618 listener keeps answering. tk doctor reports the conflict honestly.", "port,recovery", ""},
		{model.MessageKindLesson, "Always set the working directory to the repo root before launching the Windows-cross-compiled binary. Absolute /mnt/d/ paths get mangled into D:\\mnt\\d\\... by the loader.", "windows,build,launch", ""},
		{model.MessageKindLink, "TimeKeeper architecture: see TIMEKEEPER.md for the full contract.", "docs", "/TIMEKEEPER.md"},
		{model.MessageKindQuestion, "Should the message board expose a project picker on the dashboard, or always show the most recent project by default?", "ux", ""},
		{model.MessageKindAnswer, "Default to the most recent project; expose a project picker as a follow-up once users have multi-project workflows that demand it.", "ux", ""},
	}
	// Seeding writes real, immutable messages into a real project and there is
	// no delete subcommand, so the default must be to show and refuse rather
	// than to write.
	if !confirmed || dryRun {
		fmt.Fprintf(out, "seed would write %d messages on %s (author: %s):\n", len(seeds), projectID, author)
		for i, s := range seeds {
			fmt.Fprintf(out, "  %d. %-12s [%s] %s\n", i+1, s.kind, s.tags, truncate(s.body, 96))
		}
		if !confirmed {
			fmt.Fprintln(out, "refusing to write: re-run with --yes to seed this project")
			return 2
		}
		fmt.Fprintln(out, "dry run only: no messages were written")
		return 0
	}
	postURL := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/messages"
	created := 0
	for _, s := range seeds {
		input := model.CreateProjectMessageInput{Kind: s.kind, Author: author, Body: s.body, Link: s.link, Tags: s.tags}
		raw, _ := json.Marshal(input)
		req, err := http.NewRequest(http.MethodPost, postURL, strings.NewReader(string(raw)))
		if err != nil {
			fmt.Fprintf(errOut, "build request: %v\n", err)
			return 1
		}
		req.Header.Set("Content-Type", "application/json")
		if author != "" {
			req.Header.Set("X-Agent-ID", author)
		}
		resp, err := newHTTPClient(10 * time.Second).Do(req)
		if err != nil {
			fmt.Fprintf(errOut, "post seed message: %v\n", err)
			return 1
		}
		if resp.StatusCode != http.StatusCreated {
			reportHTTPStatus(errOut, "seed message", resp)
			return 1
		}
		resp.Body.Close()
		created++
	}
	fmt.Fprintf(out, "Seeded %d messages on %s (author: %s)\n", created, projectID, author)
	return 0
}

func msgEdit(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: tk msg <project> edit M-1 [--kind <kind>] [--author <a>] [--link <url>] [--tags <t>] [--body <text>] [--add-tags <t>] [--remove-tags <t>] <body...>")
		return 2
	}
	id, err := parseMessageID(args[0])
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	bodyParts := make([]string, 0, len(args)-1)
	var kind, author, link, tags, bodyText string
	var addTags, removeTags string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--kind", "--author", "--link", "--tags", "--body", "--add-tags", "--remove-tags":
			if i+1 >= len(args) {
				fmt.Fprintf(errOut, "%s requires a value\n", args[i])
				return 2
			}
			value := args[i+1]
			i++
			switch args[i-1] {
			case "--kind":
				kind = value
			case "--author":
				author = value
			case "--link":
				link = value
			case "--tags":
				tags = value
			case "--body":
				bodyText = value
			case "--add-tags":
				addTags = value
			case "--remove-tags":
				removeTags = value
			}
		case "--help", "-h":
			fmt.Fprintln(out, "usage: tk msg <project> edit M-1 [--kind <kind>] [--author <a>] [--link <url>] [--tags <t>] [--body <text>] [--add-tags <t>] [--remove-tags <t>] <body...>")
			return 0
		default:
			bodyParts = append(bodyParts, args[i])
		}
	}
	currentBody, status, err := getJSON(messageEndpoint(baseURL, projectID, id))
	if err != nil || !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "read message: %s\n", statusOrErrorBody(status, currentBody, err))
		return 1
	}
	var current model.ProjectMessage
	if err := json.Unmarshal([]byte(currentBody), &current); err != nil {
		fmt.Fprintf(errOut, "decode message: %v\n", err)
		return 1
	}
	if kind == "" {
		kind = string(current.Kind)
	}
	if author == "" {
		author = current.Author
	}
	if link == "" {
		link = current.Link
	}
	if tags == "" {
		tags = current.Tags
	}
	// Apply tag modifications (add/remove) relative to current tags
	if addTags != "" || removeTags != "" {
		tagSet := make(map[string]bool)
		for _, t := range strings.Split(current.Tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tagSet[t] = true
			}
		}
		for _, t := range strings.Split(addTags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tagSet[t] = true
			}
		}
		for _, t := range strings.Split(removeTags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				delete(tagSet, t)
			}
		}
		modifiedTags := make([]string, 0, len(tagSet))
		for t := range tagSet {
			modifiedTags = append(modifiedTags, t)
		}
		// Sort for deterministic output
		sort.Strings(modifiedTags)
		tags = strings.Join(modifiedTags, ",")
	}
	body := strings.TrimSpace(strings.Join(bodyParts, " "))
	if bodyText != "" {
		body = strings.TrimSpace(bodyText)
	} else if body == "" {
		body = current.Body
	}
	input := model.UpdateProjectMessageInput{Kind: model.MessageKind(kind), Author: author, Body: body, Link: link, Tags: tags}
	raw, _ := json.Marshal(input)
	req, err := http.NewRequest(http.MethodPatch, messageEndpoint(baseURL, projectID, id), strings.NewReader(string(raw)))
	if err != nil {
		fmt.Fprintf(errOut, "build request: %v\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := newHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		fmt.Fprintf(errOut, "edit message: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "edit message", resp)
		return 1
	}
	var edited model.ProjectMessage
	if err := json.NewDecoder(resp.Body).Decode(&edited); err != nil {
		fmt.Fprintf(errOut, "decode response: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "%s\t%s\t%s\n", edited.Ref, edited.Kind, truncate(edited.Body, 80))
	return 0
}

func msgPin(projectID string, args []string, pinned bool, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 {
		verb := "pin"
		if !pinned {
			verb = "unpin"
		}
		fmt.Fprintf(errOut, "usage: tk msg <project> %s M-1\n", verb)
		return 2
	}
	id, err := parseMessageID(args[0])
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	payload, _ := json.Marshal(map[string]bool{"pinned": pinned})
	req, err := http.NewRequest(http.MethodPost, messageEndpoint(baseURL, projectID, id)+"/pin", strings.NewReader(string(payload)))
	if err != nil {
		fmt.Fprintf(errOut, "build pin request: %v\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := newHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		fmt.Fprintf(errOut, "change message pin: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		reportHTTPStatus(errOut, "change message pin", resp)
		return 1
	}
	var message model.ProjectMessage
	if err := json.NewDecoder(resp.Body).Decode(&message); err != nil {
		fmt.Fprintf(errOut, "decode pin response: %v\n", err)
		return 1
	}
	state := "unpinned"
	if message.Pinned {
		state = "pinned"
	}
	fmt.Fprintf(out, "%s\t%s\t%s\n", message.Ref, state, truncate(message.Body, 80))
	return 0
}

func msgCheckpoint(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	author := ""
	entries := make([]model.CreateProjectMessageInput, 0)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--author":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--author requires a value")
				return 2
			}
			i++
			author = args[i]
		case "--entry":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--entry requires kind|body|tags")
				return 2
			}
			i++
			parts := strings.SplitN(args[i], "|", 3)
			if len(parts) < 2 || !model.IsValidMessageKind(model.MessageKind(parts[0])) || strings.TrimSpace(parts[1]) == "" {
				fmt.Fprintln(errOut, "--entry must be kind|body|tags with a valid message kind and non-empty body")
				return 2
			}
			tags := ""
			if len(parts) == 3 {
				tags = parts[2]
			}
			entries = append(entries, model.CreateProjectMessageInput{Kind: model.MessageKind(parts[0]), Body: parts[1], Tags: tags})
		case "--help", "-h":
			fmt.Fprintln(out, "usage: tk msg <project> checkpoint [--author <a>] --entry kind|body|tags [--entry kind|body|tags ...]")
			return 0
		default:
			fmt.Fprintf(errOut, "unknown checkpoint flag %q\n", args[i])
			return 2
		}
	}
	if len(entries) == 0 {
		fmt.Fprintln(errOut, "checkpoint requires at least one --entry")
		return 2
	}
	payload, _ := json.Marshal(model.ProjectMessageCheckpointInput{Author: author, Entries: entries})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/projects/"+projectID+"/messages/checkpoint", strings.NewReader(string(payload)))
	if err != nil {
		fmt.Fprintf(errOut, "build checkpoint request: %v\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	if author != "" {
		req.Header.Set("X-Agent-ID", author)
	}
	resp, err := newHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		fmt.Fprintf(errOut, "post checkpoint: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		reportHTTPStatus(errOut, "post checkpoint", resp)
		return 1
	}
	var checkpoint model.ProjectMessageCheckpoint
	if err := json.NewDecoder(resp.Body).Decode(&checkpoint); err != nil {
		fmt.Fprintf(errOut, "decode checkpoint: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "checkpoint\t%s\t%d entries\n", checkpoint.Root.Ref, len(checkpoint.Items))
	for _, item := range checkpoint.Items {
		fmt.Fprintf(out, "%s\t%s\t%s\n", item.Ref, item.Kind, truncate(item.Body, 80))
	}
	return 0
}

func msgContext(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: tk msg <project> context M-1 [--query <q>] [--thread-limit <n>] [--related-limit <n>] [--revision-limit <n>]")
		return 2
	}
	id, err := parseMessageID(args[0])
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	q := url.Values{}
	for i := 1; i < len(args); i++ {
		flag := args[i]
		if flag == "--help" || flag == "-h" {
			fmt.Fprintln(out, "usage: tk msg <project> context M-1 [--query <q>] [--thread-limit <n>] [--related-limit <n>] [--revision-limit <n>]")
			return 0
		}
		if flag != "--query" && flag != "--thread-limit" && flag != "--related-limit" && flag != "--revision-limit" {
			fmt.Fprintf(errOut, "unknown context flag %q\n", flag)
			return 2
		}
		if i+1 >= len(args) {
			fmt.Fprintf(errOut, "%s requires a value\n", flag)
			return 2
		}
		i++
		value := args[i]
		if flag != "--query" {
			n, parseErr := strconv.Atoi(value)
			if parseErr != nil || n < 0 {
				fmt.Fprintf(errOut, "%s must be a non-negative integer\n", flag)
				return 2
			}
		}
		q.Set(strings.ReplaceAll(strings.TrimPrefix(flag, "--"), "-", "_"), value)
	}
	endpoint := messageEndpoint(baseURL, projectID, id) + "/context"
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	body, status, err := getJSON(endpoint)
	if err != nil || !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "read message context: %s\n", statusOrErrorBody(status, body, err))
		return 1
	}
	io.WriteString(out, body)
	io.WriteString(out, "\n")
	return 0
}

func msgThread(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	return msgReadCollection(projectID, args, out, errOut, baseURL, "thread")
}

func msgRevisions(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	return msgReadCollection(projectID, args, out, errOut, baseURL, "revisions")
}

func msgReadCollection(projectID string, args []string, out, errOut io.Writer, baseURL string, collection string) int {
	if len(args) != 1 {
		fmt.Fprintf(errOut, "usage: tk msg <project> %s M-1\n", collection)
		return 2
	}
	id, err := parseMessageID(args[0])
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	body, status, err := getJSON(messageEndpoint(baseURL, projectID, id) + "/" + collection)
	if err != nil || !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "read message %s: %s\n", collection, statusOrErrorBody(status, body, err))
		return 1
	}
	io.WriteString(out, body)
	io.WriteString(out, "\n")
	return 0
}

func parseMessageID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == 'M' || value[0] == 'm') && value[1] == '-' {
		value = value[2:]
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("message id must be a positive integer or M-<id>")
	}
	return id, nil
}

func messageEndpoint(baseURL, projectID string, id int64) string {
	return strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/messages/" + strconv.FormatInt(id, 10)
}

func statusOrError(status string, err error) string {
	if err != nil {
		return err.Error()
	}
	return status
}

func statusOrErrorBody(status, body string, err error) string {
	if err != nil {
		return err.Error()
	}
	if !strings.HasPrefix(status, "2") && body != "" {
		return formatAPIError(status, []byte(body), "")
	}
	return status
}

func msgShow(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: tk msg <project> show M-1")
		return 2
	}
	id, err := parseMessageID(args[0])
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	body, status, err := getJSON(messageEndpoint(baseURL, projectID, id))
	if err != nil || !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "show message: %s\n", statusOrErrorBody(status, body, err))
		return 1
	}
	io.WriteString(out, body)
	io.WriteString(out, "\n")
	return 0
}

// getJSON is a small wrapper used by the msg subcommands to fetch JSON
// from the API and return the body + status string.
func getJSON(url string) (string, string, error) {
	resp, err := newHTTPClient(10 * time.Second).Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.Status, err
	}
	if resp.StatusCode != http.StatusOK {
		return string(body), resp.Status, nil
	}
	return string(body), resp.Status, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Ensure os is referenced (avoid unused import if this file is later trimmed).
var _ = os.Stderr
