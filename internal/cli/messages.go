// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
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

// messages dispatches tk msg <project> <add|list|search|show>.
//
//   tk msg <project> add   [--kind <k>] [--author <a>] [--link <u>] [--tags <t>] <body...>
//   tk msg <project> list  [--kind <k>] [--limit <n>]
//   tk msg <project> search <query...> [--limit <n>]
//   tk msg <project> show  <message-id>
//
// Output is JSON for `list` / `search` / `show` so it composes with the
// rest of the CLI; `add` prints the created record as a one-liner.
func messages(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 2 {
		fmt.Fprintln(errOut, "usage: tk msg <project-id> <add|list|search|show|seed> ...")
		return 2
	}
	projectID := args[0]
	switch args[1] {
	case "add":
		return msgAdd(projectID, args[2:], out, errOut, baseURL)
	case "list":
		return msgList(projectID, args[2:], out, errOut, baseURL)
	case "search":
		return msgSearch(projectID, args[2:], out, errOut, baseURL)
	case "show":
		return msgShow(projectID, args[2:], out, errOut, baseURL)
	case "seed":
		return msgSeed(projectID, args[2:], out, errOut, baseURL)
	default:
		fmt.Fprintf(errOut, "unknown msg subcommand %q (use add|list|search|show|seed)\n", args[1])
		return 2
	}
}

func msgAdd(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	var kind model.MessageKind = model.MessageKindNote
	var author, link, tags string
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
		case "--help", "-h":
			fmt.Fprintln(out, "usage: tk msg <project> add [--kind <k>] [--author <a>] [--link <u>] [--tags <t>] <body...>")
			return 0
		default:
			bodyParts = append(bodyParts, a)
		}
		i++
	}
	body := strings.TrimSpace(strings.Join(bodyParts, " "))
	if body == "" {
		fmt.Fprintln(errOut, "message body is required (pass it as the final argument)")
		return 2
	}
	input := model.CreateProjectMessageInput{Kind: kind, Author: author, Body: body, Link: link, Tags: tags}
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
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		fmt.Fprintf(errOut, "post message: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		fmt.Fprintf(errOut, "create message: %s\n", resp.Status)
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
		fmt.Fprintf(errOut, "list messages: HTTP %s\n", status)
		return 1
	}
	io.WriteString(out, body)
	io.WriteString(out, "\n")
	return 0
}

func msgSearch(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: tk msg <project> search <query...> [--limit <n>]")
		return 2
	}
	limit := ""
	qParts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--limit" {
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--limit requires a value")
				return 2
			}
			i++
			limit = args[i]
			continue
		}
		qParts = append(qParts, a)
	}
	q := strings.TrimSpace(strings.Join(qParts, " "))
	if q == "" {
		fmt.Fprintln(errOut, "search query is required")
		return 2
	}
	qvals := url.Values{}
	qvals.Set("q", q)
	if limit != "" {
		qvals.Set("limit", limit)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/messages/search?" + qvals.Encode()
	body, status, err := getJSON(endpoint)
	if err != nil {
		fmt.Fprintf(errOut, "search messages: %v\n", err)
		return 1
	}
	if !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "search messages: HTTP %s\n", status)
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
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--author":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--author requires a value")
				return 2
			}
			i++
			author = args[i]
		case "--help", "-h":
			fmt.Fprintln(out, "usage: tk msg <project> seed [--author <a>]")
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
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			fmt.Fprintf(errOut, "post seed message: %v\n", err)
			return 1
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			fmt.Fprintf(errOut, "seed message returned %s\n", resp.Status)
			return 1
		}
		created++
	}
	fmt.Fprintf(out, "Seeded %d messages on %s (author: %s)\n", created, projectID, author)
	return 0
}

func msgShow(projectID string, args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: tk msg <project> show <message-id>")
		return 2
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id < 1 {
		fmt.Fprintln(errOut, "message id must be a positive integer")
		return 2
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/messages/" + strconv.FormatInt(id, 10)
	body, status, err := getJSON(endpoint)
	if err != nil {
		fmt.Fprintf(errOut, "show message: %v\n", err)
		return 1
	}
	if !strings.HasPrefix(status, "200") {
		fmt.Fprintf(errOut, "show message: HTTP %s\n", status)
		return 1
	}
	io.WriteString(out, body)
	io.WriteString(out, "\n")
	return 0
}

// getJSON is a small wrapper used by the msg subcommands to fetch JSON
// from the API and return the body + status string.
func getJSON(url string) (string, string, error) {
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
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
