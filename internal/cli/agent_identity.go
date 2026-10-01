package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

type agentIdentity struct {
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
	Runtime   string `json:"runtime"`
}

// Any runtime can supply the TIMEKEEPER_* contract. Codex's actual session
// variables are a convenience, never a process-name or shared-workspace guess.
func resolveAgentIdentity() agentIdentity {
	if tracked := currentFASE.Load(); tracked != nil {
		// Public activity state carries a separate, one-way session token, never
		// the registry fingerprint shown masked on the Agent Profile.
		if tracked.resumeSession != "" {
			return agentIdentity{AgentID: tracked.identity.AgentID, SessionID: tracked.resumeSession, Runtime: tracked.identity.Runtime}
		}
		digest := sha256.Sum256([]byte("fase-activity-session-v1\x00" + tracked.fingerprint))
		return agentIdentity{AgentID: tracked.identity.AgentID, SessionID: hex.EncodeToString(digest[:]), Runtime: tracked.identity.Runtime}
	}
	id := agentIdentity{strings.TrimSpace(os.Getenv("TIMEKEEPER_AGENT_ID")), strings.TrimSpace(os.Getenv("TIMEKEEPER_SESSION_ID")), strings.TrimSpace(os.Getenv("TIMEKEEPER_AGENT_RUNTIME"))}
	if id.SessionID == "" {
		id.SessionID = strings.TrimSpace(os.Getenv("CODEX_THREAD_ID"))
		if id.SessionID == "" {
			id.SessionID = strings.TrimSpace(os.Getenv("CODEX_SESSION_ID"))
		}
		if id.SessionID != "" && id.Runtime == "" {
			id.Runtime = "codex"
		}
	}
	if id.SessionID == "" {
		// Hermes is the supervised runtime for the local harness. Prefer its
		// session contract over process names or a shared-workspace guess; the
		// latter would merge concurrent workers under one identity.
		id.SessionID = strings.TrimSpace(os.Getenv("HERMES_SESSION_ID"))
		if id.SessionID == "" {
			id.SessionID = strings.TrimSpace(os.Getenv("HERMES_SESSION_THREAD_ID"))
		}
		if id.SessionID != "" && id.Runtime == "" {
			id.Runtime = "hermes"
		}
	}
	if id.SessionID != "" && id.Runtime == "" {
		id.Runtime = "generic"
	}
	if id.AgentID == "" && id.SessionID != "" {
		// Digest arbitrary runtime/session input into a stable, header-safe identifier.
		digest := sha256.Sum256([]byte(id.Runtime + "\x00" + id.SessionID))
		id.AgentID = "agent-" + hex.EncodeToString(digest[:16])
	}
	return id
}

func agentActivityCommand(args []string, out, errOut io.Writer, baseURL string) int {
	id := resolveAgentIdentity()
	if args[0] == "identity" {
		if len(args) != 1 {
			return 2
		}
		if tracked := currentFASE.Load(); tracked != nil {
			_ = json.NewEncoder(out).Encode(tracked.identity)
		} else {
			_ = json.NewEncoder(out).Encode(id)
		}
		return 0
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/agents/"
	method := http.MethodGet
	var body []byte
	if args[0] == "states" {
		if len(args) != 1 {
			return 2
		}
		endpoint += "work-states"
	} else if args[0] == "state" {
		if len(args) > 2 {
			return 2
		}
		if len(args) == 2 {
			id.AgentID = args[1]
		}
		if id.AgentID == "" {
			fmt.Fprintln(errOut, "set TIMEKEEPER_AGENT_ID and TIMEKEEPER_SESSION_ID or supply an agent ID")
			return 2
		}
		endpoint += url.PathEscape(id.AgentID) + "/work-state"
	} else {
		if id.AgentID == "" || id.SessionID == "" {
			fmt.Fprintln(errOut, "set TIMEKEEPER_AGENT_ID and TIMEKEEPER_SESSION_ID for this individual agent session")
			return 2
		}
		input := model.AgentActivityInput{SessionID: id.SessionID, Runtime: id.Runtime, Event: args[0]}
		switch args[0] {
		case "work":
			if len(args) != 3 {
				fmt.Fprintln(errOut, "usage: tk agent work <sprint-id> <lease>")
				return 2
			}
			d, err := time.ParseDuration(args[2])
			if err != nil || d < time.Second || d > 24*time.Hour || d%time.Second != 0 {
				fmt.Fprintln(errOut, "lease must be whole seconds from 1s to 24h")
				return 2
			}
			input.SprintID = args[1]
			input.LeaseDurationSeconds = int64(d / time.Second)
		case "wait":
			if len(args) < 3 {
				fmt.Fprintln(errOut, "usage: tk agent wait <duration> <reason>")
				return 2
			}
			d, err := time.ParseDuration(args[1])
			if err != nil || d < time.Second || d > 24*time.Hour || d%time.Second != 0 {
				fmt.Fprintln(errOut, "wait must be whole seconds from 1s to 24h")
				return 2
			}
			input.WaitDurationSeconds = int64(d / time.Second)
			input.Reason = strings.Join(args[2:], " ")
		case "idle", "stop":
			input.Reason = strings.Join(args[1:], " ")
		default:
			return 2
		}
		body, _ = json.Marshal(input)
		method = http.MethodPost
		endpoint += url.PathEscape(id.AgentID) + "/activity"
	}
	request, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := newHTTPClient(10 * time.Second).Do(request)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		reportHTTPStatus(errOut, "agent activity", response)
		return 1
	}
	_, err = io.Copy(out, response.Body)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}
