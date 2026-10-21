// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func worktreeCLI(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		return wtList(args, out, errOut, baseURL)
	}
	switch args[0] {
	case "list", "ls":
		return wtList(args[1:], out, errOut, baseURL)
	case "new":
		return wtNew(args[1:], out, errOut, baseURL)
	case "path":
		return wtPath(args[1:], out, errOut, baseURL)
	case "remove", "rm":
		return wtRemove(args[1:], out, errOut, baseURL)
	case "prune":
		return wtPrune(args[1:], out, errOut, baseURL)
	default:
		// Shorthand: `tk wt SP-10665` or `tk wt branch-name`
		// If worktree exists, return its path; if not, create it.
		if wtPath(args, out, io.Discard, baseURL) == 0 {
			return 0
		}
		return wtNew(args, out, errOut, baseURL)
	}
}

type Worktree struct {
	Path   string `json:"path"`
	HEAD   string `json:"head"`
	Branch string `json:"branch"`
}

func getGitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func parseWorktreePorcelain(output string) []Worktree {
	var wts []Worktree
	var current Worktree
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			if current.Path != "" {
				wts = append(wts, current)
				current = Worktree{}
			}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	if current.Path != "" {
		wts = append(wts, current)
	}
	return wts
}

func loadTkrcProjectID() string {
	b, err := os.ReadFile(".tkrc")
	if err != nil {
		return ""
	}
	var data struct {
		ProjectID string `json:"project_id"`
	}
	_ = json.Unmarshal(b, &data)
	return data.ProjectID
}

func getSprintStatus(baseURL, projectID, sprintID string) string {
	if projectID == "" {
		return ""
	}
	url := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/execution-tree"
	resp, err := newHTTPClient(10 * time.Second).Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	// Try parsing enough to find the status of this sprint
	var tree struct {
		Tasks []struct {
			TaskID  string `json:"task_id"`
			Sprints []struct {
				SprintID string `json:"sprint_id"`
				Status   string `json:"status"`
			} `json:"sprints"`
		} `json:"tasks"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&tree)
	for _, t := range tree.Tasks {
		if t.TaskID == sprintID {
			return "Task"
		}
		for _, s := range t.Sprints {
			if s.SprintID == sprintID {
				return s.Status
			}
		}
	}
	return ""
}

func wtList(args []string, out, errOut io.Writer, baseURL string) int {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "git worktree list failed: %v\n", err)
		return 1
	}
	wts := parseWorktreePorcelain(string(output))

	if jsonOutputEnabled {
		writeJSONResponse(out, wts)
		return 0
	}

	projectID := loadTkrcProjectID()

	_, _ = fmt.Fprintln(out, "WORKTREE\tBRANCH\tCOMMIT\tTIMEKEEPER BINDING\tSTATUS")
	for _, wt := range wts {
		binding := ""
		status := ""
		if strings.Contains(wt.Branch, "SP-") {
			idx := strings.Index(wt.Branch, "SP-")
			if idx >= 0 {
				binding = wt.Branch[idx:]
				if spIdx := strings.Index(binding, "-"); spIdx >= 0 {
					parts := strings.SplitN(binding, "-", 3)
					if len(parts) >= 2 {
						binding = parts[0] + "-" + parts[1]
					}
				}
			}
		} else if strings.Contains(wt.Branch, "T-") {
			idx := strings.Index(wt.Branch, "T-")
			if idx >= 0 {
				binding = wt.Branch[idx:]
				if spIdx := strings.Index(binding, "-"); spIdx >= 0 {
					parts := strings.SplitN(binding, "-", 3)
					if len(parts) >= 2 {
						binding = parts[0] + "-" + parts[1]
					}
				}
			}
		}

		if binding != "" && projectID != "" {
			status = getSprintStatus(baseURL, projectID, binding)
		}

		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", wt.Path, wt.Branch, wt.HEAD, binding, status)
	}
	return 0
}

func wtNew(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 1 {
		_, _ = fmt.Fprintln(errOut, "usage: tk wt new <sprint-id|task-id|branch-name> [base-branch]")
		return 2
	}
	idOrBranch := args[0]
	baseBranch := ""
	if len(args) >= 2 {
		baseBranch = args[1]
	}

	repoRoot, err := getGitRepoRoot()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "failed to get git repo root: %v\n", err)
		return 1
	}

	branchName := idOrBranch
	if strings.HasPrefix(idOrBranch, "SP-") {
		branchName = "sprint/" + idOrBranch
	} else if strings.HasPrefix(idOrBranch, "T-") {
		branchName = "task/" + idOrBranch
	}

	wtDir := filepath.Join(repoRoot, ".worktrees")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		_, _ = fmt.Fprintf(errOut, "failed to create .worktrees dir: %v\n", err)
		return 1
	}

	// Create safe dir name
	safeDir := strings.ReplaceAll(branchName, "/", "-")
	safeDir = strings.ReplaceAll(safeDir, "\\", "-")
	wtPath := filepath.Join(wtDir, safeDir)

	var gitArgs []string
	gitArgs = append(gitArgs, "worktree", "add", wtPath, "-b", branchName)
	if baseBranch != "" {
		gitArgs = append(gitArgs, baseBranch)
	}

	cmd := exec.Command("git", gitArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Fprintf(errOut, "git worktree add failed: %v\n", err)
		return 1
	}

	envFiles := []string{".env", ".env.local", ".env.development", ".env.test"}
	for _, env := range envFiles {
		src := filepath.Join(repoRoot, env)
		dst := filepath.Join(wtPath, env)
		if data, err := os.ReadFile(src); err == nil {
			_ = os.WriteFile(dst, data, 0644)
		}
	}

	if strings.HasPrefix(idOrBranch, "SP-") {
		projectID := loadTkrcProjectID()
		if projectID != "" {
			msgUrl := strings.TrimRight(baseURL, "/") + "/api/v1/projects/" + projectID + "/messages"
			body, _ := json.Marshal(map[string]string{
				"content": fmt.Sprintf("Worktree created at .worktrees/%s", safeDir),
			})
			req, _ := http.NewRequest(http.MethodPost, msgUrl, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := newHTTPClient(10 * time.Second).Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
	}

	_, _ = fmt.Fprintln(out, wtPath)
	return 0
}

func wtPath(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 1 {
		_, _ = fmt.Fprintln(errOut, "usage: tk wt path <sprint-id|branch-name>")
		return 2
	}
	idOrBranch := args[0]

	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "git worktree list failed: %v\n", err)
		return 1
	}
	wts := parseWorktreePorcelain(string(output))
	for _, wt := range wts {
		if wt.Branch == idOrBranch || strings.Contains(wt.Branch, idOrBranch) {
			_, _ = fmt.Fprintln(out, wt.Path)
			return 0
		}
	}
	return 1
}

func wtRemove(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 1 {
		_, _ = fmt.Fprintln(errOut, "usage: tk wt remove <sprint-id|branch-name> [--force]")
		return 2
	}
	idOrBranch := args[0]
	force := false
	if len(args) >= 2 && args[1] == "--force" {
		force = true
	}

	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "git worktree list failed: %v\n", err)
		return 1
	}
	wts := parseWorktreePorcelain(string(output))
	var targetWt *Worktree
	for _, wt := range wts {
		if wt.Branch == idOrBranch || strings.Contains(wt.Branch, idOrBranch) {
			targetWt = &wt
			break
		}
	}

	if targetWt == nil {
		_, _ = fmt.Fprintf(errOut, "worktree for %s not found\n", idOrBranch)
		return 1
	}

	rmArgs := []string{"worktree", "remove"}
	if force {
		rmArgs = append(rmArgs, "--force")
	}
	rmArgs = append(rmArgs, targetWt.Path)

	rmCmd := exec.Command("git", rmArgs...)
	if err := rmCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(errOut, "git worktree remove failed: %v\n", err)
		return 1
	}

	branchDelArgs := []string{"branch", "-d"}
	if force {
		branchDelArgs = []string{"branch", "-D"}
	}
	branchDelArgs = append(branchDelArgs, targetWt.Branch)
	branchCmd := exec.Command("git", branchDelArgs...)
	_ = branchCmd.Run()

	return 0
}

func wtPrune(args []string, out, errOut io.Writer, baseURL string) int {
	cmd := exec.Command("git", "worktree", "prune")
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Fprintf(errOut, "git worktree prune failed: %v\n", err)
		return 1
	}

	// Also scan .worktrees and prune stale
	repoRoot, err := getGitRepoRoot()
	if err == nil {
		wtDir := filepath.Join(repoRoot, ".worktrees")
		projectID := loadTkrcProjectID()

		entries, err := os.ReadDir(wtDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				name := entry.Name()
				binding := ""
				if strings.Contains(name, "SP-") {
					idx := strings.Index(name, "SP-")
					parts := strings.Split(name[idx:], "-")
					if len(parts) >= 2 {
						binding = parts[0] + "-" + parts[1]
					}
				}
				if binding != "" && projectID != "" {
					status := getSprintStatus(baseURL, projectID, binding)
					if status == "Completed" || status == "Cancelled" {
						_ = exec.Command("git", "worktree", "remove", "--force", filepath.Join(wtDir, name)).Run()
						_ = exec.Command("git", "branch", "-D", strings.ReplaceAll(name, "-", "/")).Run()
					}
				}
			}
		}
	}

	_, _ = fmt.Fprintln(out, "Pruned git worktrees.")
	return 0
}
