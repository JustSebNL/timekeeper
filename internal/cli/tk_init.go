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

// tkInit implements `tk init` — the one-command bootstrap that sets up
// an agent to use TimeKeeper with the guardian wake-up system.
//
// Usage:
//   tk init --agent <id> [--project <id>] [--lease <duration>] [--url <api-base>]
//
// This creates a .tkrc config file, registers the agent with the guardian,
// and starts the self-nudge receiver so the agent can be woken by the guardian.
func tkInit(args []string, out, errOut io.Writer, baseURL string) int {
	agentID := ""
	projectID := ""
	lease := "30m"
	ntfyServer := ""
	ntfyTopic := ""
	ntfyToken := ""
	bindAddr := "127.0.0.1:1619"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 < len(args) {
				agentID = args[i+1]
				i++
			}
		case "--project":
			if i+1 < len(args) {
				projectID = args[i+1]
				i++
			}
		case "--lease":
			if i+1 < len(args) {
				lease = args[i+1]
				i++
			}
		case "--url":
			if i+1 < len(args) {
				baseURL = args[i+1]
				i++
			}
		case "--ntfy-server":
			if i+1 < len(args) {
				ntfyServer = args[i+1]
				i++
			}
		case "--ntfy-topic":
			if i+1 < len(args) {
				ntfyTopic = args[i+1]
				i++
			}
		case "--ntfy-token":
			if i+1 < len(args) {
				ntfyToken = args[i+1]
				i++
			}
		case "--bind":
			if i+1 < len(args) {
				bindAddr = args[i+1]
				i++
			}
		}
	}

	if agentID == "" {
		_, _ = fmt.Fprintln(errOut, "usage: tk init --agent <id> [--project <id>] [--lease <duration>] [--url <api-base>] [--ntfy-server <url> --ntfy-topic <topic>]")
		return 1
	}

	// 1. Create .tkrc config file.
	config := map[string]any{
		"agent_id":    agentID,
		"project_id":  projectID,
		"base_url":    strings.TrimRight(baseURL, "/"),
		"lease":       lease,
		"bind_addr":   bindAddr,
		"ntfy_server": ntfyServer,
		"ntfy_topic":  ntfyTopic,
		"ntfy_token":  ntfyToken,
	}
	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode config: %v\n", err)
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "get cwd: %v\n", err)
		return 1
	}
	configPath := filepath.Join(cwd, ".tkrc")
	if err := os.WriteFile(configPath, append(configData, '\n'), 0o600); err != nil {
		_, _ = fmt.Fprintf(errOut, "write config: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "created %s\n", configPath)

	// 2. Register the agent with the guardian.
	guardianURL := "http://" + bindAddr + "/pulse"
	leaseDur, err := time.ParseDuration(lease)
	if err != nil || leaseDur < time.Second {
		_, _ = fmt.Fprintf(errOut, "lease must be a positive duration such as 30m or 1h\n")
		return 1
	}

	regInput := map[string]any{
		"lease_duration_seconds": int64(leaseDur.Seconds()),
		"guardian_url":          guardianURL,
	}
	regBody, err := json.Marshal(regInput)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "encode registration: %v\n", err)
		return 1
	}
	regURL := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + agentID + "/progress"
	regReq, err := http.NewRequest(http.MethodPost, regURL, bytes.NewReader(regBody))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "create request: %v\n", err)
		return 1
	}
	regReq.Header.Set("Content-Type", "application/json")
	regResp, err := (&http.Client{Timeout: 10 * time.Second}).Do(regReq)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "register agent: %v\n", err)
		return 1
	}
	regResp.Body.Close()
	if regResp.StatusCode != http.StatusOK && regResp.StatusCode != http.StatusCreated {
		_, _ = fmt.Fprintf(errOut, "register agent: %s\n", regResp.Status)
		return 1
	}
	_, _ = fmt.Fprintf(out, "registered agent %s with lease %s\n", agentID, lease)

	// 3. Start the self-nudge receiver.
	selfExe := os.Args[0]
	selfDir := filepath.Dir(selfExe)
	receiverExe := filepath.Join(selfDir, "tk-guardian-self.exe")
	if _, err := os.Stat(receiverExe); err != nil {
		_, _ = fmt.Fprintf(out, "tk-guardian-self.exe not found at %s\n", receiverExe)
		_, _ = fmt.Fprintf(out, "build it with: go build -o %s ./cmd/tk-guardian-self\n", receiverExe)
		return 0
	}

	cmd := exec.Command(receiverExe)
	cmd.Env = append(os.Environ(),
		"TK_AGENT_ID="+agentID,
		"TK_GUARDIAN_SELF_BIND="+bindAddr,
		"TK_GUARDIAN_SELF_WORK=scripts/guardian/self-nudge-work.sh",
		"TK_BASE_URL="+strings.TrimRight(baseURL, "/"),
	)
	if projectID != "" {
		cmd.Env = append(cmd.Env, "PROJECT_ID="+projectID)
	}
	cmd.Stdout = out
	cmd.Stderr = errOut
	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(errOut, "start receiver: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "self-nudge receiver started (pid %d)\n", cmd.Process.Pid)

	// 4. Print next steps.
	_, _ = fmt.Fprintf(out, "\nTimeKeeper initialized for agent %s\n", agentID)
	_, _ = fmt.Fprintf(out, "  config: %s\n", configPath)
	_, _ = fmt.Fprintf(out, "  guardian: %s\n", guardianURL)
	_, _ = fmt.Fprintf(out, "  receiver: %s\n", receiverExe)
	if ntfyServer != "" && ntfyTopic != "" {
		_, _ = fmt.Fprintf(out, "  ntfy: %s/%s\n", ntfyServer, ntfyTopic)
	}
	_, _ = fmt.Fprintf(out, "\nThe agent will now be woken by the guardian when work is available.\n")
	_, _ = fmt.Fprintf(out, "Use 'tk agent status %s' to check agent status.\n", agentID)

	return 0
}
