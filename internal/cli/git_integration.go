// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func gitCommitCLI(args []string, out, errOut io.Writer, baseURL string) int {
	var all bool
	var message string
	for _, arg := range args {
		if arg == "-a" || arg == "--all" {
			all = true
		} else if message == "" {
			message = arg
		} else {
			message += " " + arg
		}
	}
	if message == "" {
		fmt.Fprintln(errOut, "usage: tk commit <message> [-a|--all]")
		return 2
	}

	sprintID, taskID, projectID := resolveActiveSprintOrBranch(baseURL)
	if sprintID == "" && taskID == "" {
		fmt.Fprintln(errOut, "No active Sprint or Task found.")
		return 1
	}

	id := sprintID
	if id == "" {
		id = taskID
	}

	if !strings.Contains(message, id) {
		message = fmt.Sprintf("feat(%s): %s", id, message)
	}

	if all {
		exec.Command("git", "add", "-u").Run()
		if out, err := exec.Command("git", "status", "--porcelain").Output(); err == nil && len(out) > 0 {
			exec.Command("git", "add", ".").Run()
		}
	}

	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Stdout = out
	cmd.Stderr = errOut
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(errOut, "git commit failed: %v\n", err)
		return 1
	}

	hashCmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	hashBytes, _ := hashCmd.Output()
	hash := strings.TrimSpace(string(hashBytes))

	if projectID != "" {
		postMemoryNote(baseURL, projectID, fmt.Sprintf("Commit %s: %s", hash, message), errOut)
	}

	fmt.Fprintf(out, "Commit %s created: %s\n", hash, message)

	provider, rawUrl, _ := getGitProviderURL()
	if provider != "generic" {
		branchBytes, _ := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
		branch := strings.TrimSpace(string(branchBytes))
		if owner, repo := parseRepoOwnerName(rawUrl); owner != "" && repo != "" && branch != "" {
			mrUrl := getMRURL(provider, owner, repo, branch, rawUrl)
			fmt.Fprintf(out, "MR/PR ready: %s\n", mrUrl)
		}
	}

	return 0
}

func gitSyncCLI(args []string, out, errOut io.Writer, baseURL string) int {
	cmdPull := exec.Command("git", "pull", "--rebase")
	cmdPull.Stdout = out
	cmdPull.Stderr = errOut
	if err := cmdPull.Run(); err != nil {
		fmt.Fprintf(errOut, "git pull failed: %v\n", err)
		return 1
	}

	cmdPush := exec.Command("git", "push")
	if outBytes, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch := strings.TrimSpace(string(outBytes))
		// simplistic check if upstream is missing, push may fail. If so, push -u.
		if err := cmdPush.Run(); err != nil {
			cmdPushU := exec.Command("git", "push", "-u", "origin", branch)
			cmdPushU.Stdout = out
			cmdPushU.Stderr = errOut
			if err := cmdPushU.Run(); err != nil {
				fmt.Fprintf(errOut, "git push failed: %v\n", err)
				return 1
			}
		}

		projectID := getActiveProject(baseURL)
		if projectID != "" {
			postMemoryNote(baseURL, projectID, fmt.Sprintf("Synced branch %s with remote", branch), errOut)
		}

		provider, rawUrl, _ := getGitProviderURL()
		if provider != "generic" {
			if owner, repo := parseRepoOwnerName(rawUrl); owner != "" && repo != "" {
				mrUrl := getMRURL(provider, owner, repo, branch, rawUrl)
				fmt.Fprintf(out, "MR/PR ready: %s\n", mrUrl)
			}
		}
	}
	return 0
}

func gitCLI(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 || (args[0] != "status" && args[0] != "hooks" && args[0] != "mr" && args[0] != "pr") {
		fmt.Fprintln(errOut, "usage: tk git <status|hooks|mr|pr>")
		return 2
	}

	if args[0] == "hooks" {
		return gitHooksCLI(args[1:], out, errOut, baseURL)
	}

	if args[0] == "mr" || args[0] == "pr" {
		return gitMRCLI(args[1:], out, errOut, baseURL)
	}

	branchBytes, _ := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	branch := strings.TrimSpace(string(branchBytes))

	statusBytes, _ := exec.Command("git", "status", "--porcelain").Output()
	lines := strings.Split(string(statusBytes), "\n")
	staged, unstaged, untracked := 0, 0, 0
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		if x == '?' && y == '?' {
			untracked++
		} else {
			if x != ' ' && x != '?' {
				staged++
			}
			if y != ' ' && y != '?' {
				unstaged++
			}
		}
	}

	sprintID, _, _ := resolveActiveSprintOrBranch(baseURL)
	activeStr := "None"
	if sprintID != "" {
		activeStr = sprintID + " [Active]"
	}

	fmt.Fprintf(out, "Branch: %s\nActive Sprint: %s\nStaged files: %d\nUnstaged files: %d\nUntracked files: %d\n",
		branch, activeStr, staged, unstaged, untracked)

	return 0
}

func getActiveProject(baseURL string) string {
	data, err := os.ReadFile(".tkrc")
	if err != nil {
		return ""
	}
	var binding struct {
		ProjectID string `json:"project_id"`
	}
	if json.Unmarshal(data, &binding) != nil {
		return ""
	}
	return binding.ProjectID
}

func getActiveWork(baseURL string) (string, string, string) {
	projectID := getActiveProject(baseURL)
	if projectID == "" {
		return "", "", ""
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + url.PathEscape(projectID) + "/execution-tree"
	resp, err := newHTTPClient(10 * time.Second).Get(endpoint)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", "", projectID
	}
	defer resp.Body.Close()

	var tree model.ExecutionTree
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&tree); err != nil {
		return "", "", projectID
	}

	sprintID, taskID := findActiveInCategories(tree.Categories)
	return sprintID, taskID, projectID
}

func findActiveInCategories(categories []model.ExecutionCategory) (string, string) {
	for _, c := range categories {
		for _, t := range c.Tasks {
			for _, s := range t.Sprints {
				if s.Status == "Active" {
					return s.SprintID, t.Task.TaskID
				}
			}
			for _, st := range t.Subtasks {
				for _, s := range st.Sprints {
					if s.Status == "Active" {
						return s.SprintID, t.Task.TaskID
					}
				}
			}
		}
		if s, t := findActiveInCategories(c.Categories); s != "" || t != "" {
			return s, t
		}
	}
	return "", ""
}

func postMemoryNote(baseURL, projectID, message string, errOut io.Writer) {
	body, _ := json.Marshal(map[string]string{"body": message})
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + url.PathEscape(projectID) + "/messages"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, err := newHTTPClient(10 * time.Second).Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}
}

func autoSyncEnabled() bool {
	if os.Getenv("TIMEKEEPER_AUTO_GIT_SYNC") == "false" {
		return false
	}
	data, err := os.ReadFile(".tkrc")
	if err != nil {
		return true
	}
	var binding struct {
		AutoGitSync *bool `json:"auto_git_sync"`
	}
	if json.Unmarshal(data, &binding) == nil && binding.AutoGitSync != nil {
		return *binding.AutoGitSync
	}
	return true
}

func hasGitRemoteOrigin() bool {
	cmd := exec.Command("git", "remote")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, remote := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(remote) == "origin" {
			return true
		}
	}
	return false
}

func performAutoGitSync(action string, sprintID string, out io.Writer) {
	if !autoSyncEnabled() || !hasGitRemoteOrigin() {
		return
	}

	if action == "start" {
		exec.Command("git", "fetch").Run()
		if err := exec.Command("git", "pull", "--rebase").Run(); err == nil {
			fmt.Fprintln(out, "[git] Synced repo with origin")
		}
	} else if action == "complete" {
		if outBytes, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
			branch := strings.TrimSpace(string(outBytes))
			if branch != "" && branch != "HEAD" {
				if err := exec.Command("git", "push").Run(); err == nil {
					fmt.Fprintf(out, "[git] Pushed branch %s to origin\n", branch)
				}
			}
		}
	}
}

var sprintOrTaskRegex = regexp.MustCompile(`(SP-\d+|T-\d+)`)

func resolveActiveSprintOrBranch(baseURL string) (string, string, string) {
	sprintID, taskID, projectID := getActiveWork(baseURL)
	if sprintID != "" || taskID != "" {
		return sprintID, taskID, projectID
	}

	branchBytes, err := exec.Command("git", "branch", "--show-current").Output()
	if err == nil {
		branch := strings.TrimSpace(string(branchBytes))
		matches := sprintOrTaskRegex.FindStringSubmatch(branch)
		if len(matches) > 1 {
			id := matches[1]
			if strings.HasPrefix(id, "SP-") {
				return id, "", projectID
			} else {
				return "", id, projectID
			}
		}
	}
	return "", "", projectID
}

func getGitProviderURL() (string, string, string) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		out, err = exec.Command("git", "config", "--get", "remote.origin.url").Output()
		if err != nil {
			return "generic", "", ""
		}
	}
	u := strings.TrimSpace(string(out))
	if strings.Contains(u, "github.com") || strings.Contains(u, "@github") {
		return "GitHub", u, "github.com"
	}
	if strings.Contains(u, "gitlab.com") || strings.Contains(u, "@gitlab") {
		return "GitLab", u, "gitlab.com"
	}
	if strings.Contains(u, "bitbucket.org") || strings.Contains(u, "@bitbucket") {
		return "Bitbucket", u, "bitbucket.org"
	}
	if strings.Contains(u, "dev.azure.com") || strings.Contains(u, "visualstudio.com") || strings.Contains(u, "ssh.dev.azure.com") {
		return "AzureDevOps", u, "dev.azure.com"
	}
	if strings.Contains(u, "gitea") || strings.Contains(u, "forgejo") {
		return "Gitea", u, ""
	}
	return "generic", u, ""
}

func parseRepoOwnerName(rawUrl string) (string, string) {
	u := rawUrl
	if strings.HasPrefix(u, "git@") {
		parts := strings.SplitN(u, ":", 2)
		if len(parts) == 2 {
			u = parts[1]
		}
	} else if strings.HasPrefix(u, "http") {
		parsed, err := url.Parse(u)
		if err == nil {
			u = strings.TrimPrefix(parsed.Path, "/")
		}
	}
	u = strings.TrimSuffix(u, ".git")
	parts := strings.Split(u, "/")

	// Azure DevOps generic handling
	if strings.Contains(rawUrl, "dev.azure.com") || strings.Contains(rawUrl, "visualstudio.com") {
		if strings.HasPrefix(rawUrl, "git@") || strings.Contains(rawUrl, "ssh.dev.azure.com") {
			if len(parts) >= 3 {
				return parts[len(parts)-3] + "/" + parts[len(parts)-2], parts[len(parts)-1]
			}
		} else {
			// https://dev.azure.com/org/project/_git/repo
			for i, p := range parts {
				if p == "_git" && i >= 2 && i < len(parts)-1 {
					return parts[i-2] + "/" + parts[i-1], parts[i+1]
				}
			}
		}
	}

	if len(parts) >= 2 {
		return parts[len(parts)-2], parts[len(parts)-1]
	}
	return "", ""
}

func getMRURL(provider, owner, repo, branch string, rawUrl string) string {
	switch provider {
	case "GitHub":
		return fmt.Sprintf("https://github.com/%s/%s/pull/new/%s", owner, repo, branch)
	case "GitLab":
		return fmt.Sprintf("https://gitlab.com/%s/%s/-/merge_requests/new?merge_request%%5Bsource_branch%%5D=%s", owner, repo, branch)
	case "Bitbucket":
		return fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/new?source=%s", owner, repo, branch)
	case "AzureDevOps":
		return fmt.Sprintf("https://dev.azure.com/%s/_git/%s/pullrequestcreate?sourceRef=%s", owner, repo, branch)
	case "Gitea":
		host := "gitea.com"
		if parsed, err := url.Parse(rawUrl); err == nil && parsed.Host != "" {
			host = parsed.Host
		} else if strings.HasPrefix(rawUrl, "git@") {
			if parts := strings.SplitN(rawUrl, "@", 2); len(parts) == 2 {
				if hParts := strings.SplitN(parts[1], ":", 2); len(hParts) == 2 {
					host = hParts[0]
				}
			}
		}
		return fmt.Sprintf("https://%s/%s/%s/compare/main...%s", host, owner, repo, branch)
	}
	return ""
}

func gitMRCLI(args []string, out, errOut io.Writer, baseURL string) int {
	provider, rawUrl, _ := getGitProviderURL()
	if provider == "generic" {
		fmt.Fprintln(errOut, "Could not detect known Git provider (GitHub, GitLab, Bitbucket) from origin URL.")
		return 1
	}
	owner, repo := parseRepoOwnerName(rawUrl)
	if owner == "" || repo == "" {
		fmt.Fprintln(errOut, "Could not parse repository owner and name.")
		return 1
	}

	branchBytes, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		fmt.Fprintln(errOut, "Could not determine current branch.")
		return 1
	}
	branch := strings.TrimSpace(string(branchBytes))

	mrUrl := getMRURL(provider, owner, repo, branch, rawUrl)
	fmt.Fprintf(out, "Create Pull/Merge Request: %s\n", mrUrl)
	return 0
}

func gitHooksCLI(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 || (args[0] != "install" && args[0] != "uninstall") {
		fmt.Fprintln(errOut, "usage: tk git hooks <install|uninstall>")
		return 2
	}

	gitDirBytes, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		fmt.Fprintln(errOut, "Not a git repository.")
		return 1
	}
	hooksDir := strings.TrimSpace(string(gitDirBytes)) + "/hooks"
	_ = os.MkdirAll(hooksDir, 0755)

	hooks := map[string]string{
		"post-commit":   "tk commit --from-hook\n",
		"post-checkout": "tk git status --from-hook\n",
		"pre-push":      "tk git status --from-hook\n",
	}

	for hook, content := range hooks {
		hookPath := hooksDir + "/" + hook
		if args[0] == "install" {
			existing, _ := os.ReadFile(hookPath)
			if !strings.Contains(string(existing), "tk ") {
				newContent := "#!/bin/sh\n" + string(existing) + "\n" + content
				_ = os.WriteFile(hookPath, []byte(newContent), 0755)
			}
		} else {
			existing, _ := os.ReadFile(hookPath)
			lines := strings.Split(string(existing), "\n")
			var newLines []string
			for _, line := range lines {
				if !strings.HasPrefix(line, "tk ") {
					newLines = append(newLines, line)
				}
			}
			_ = os.WriteFile(hookPath, []byte(strings.Join(newLines, "\n")), 0755)
		}
	}

	fmt.Fprintf(out, "Git hooks %sed.\n", args[0])
	return 0
}
