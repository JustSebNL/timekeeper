package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/storeclient"
)

func validWakeAgentID(id string) bool {
	if len(id) == 0 || len(id) > 128 || id == "." || id == ".." {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// Read only the non-secret binding, and only when it belongs to this agent.
func guardianConfiguredProject(agentID string) string {
	data, err := os.ReadFile(".tkrc")
	if err != nil {
		return ""
	}
	var binding struct {
		AgentID   string `json:"agent_id"`
		ProjectID string `json:"project_id"`
	}
	if json.Unmarshal(data, &binding) != nil || binding.AgentID != agentID {
		return ""
	}
	return binding.ProjectID
}

// The cache is a hint, never ownership evidence. Resolve the selected pending
// nudge against Core before passing scope to a trusted runtime adapter.
func prepareGuardianWake(tk *storeclient.Client, ctx *AgentContext, nudge model.PulseNudge) error {
	stateURL := strings.TrimRight(tk.BaseURL, "/") + "/api/v1/agents/" + url.PathEscape(nudge.AgentID) + "/work-state"
	stateResponse, err := tk.HTTPClient.Get(stateURL)
	if err != nil {
		return err
	}
	if stateResponse.StatusCode == http.StatusOK {
		var state model.AgentWorkState
		err = json.NewDecoder(io.LimitReader(stateResponse.Body, 64<<10)).Decode(&state)
		stateResponse.Body.Close()
		if err != nil {
			return err
		}
		if state.AgentID != nudge.AgentID || state.SessionID == "" {
			return errors.New("work-state identity does not match the watched agent")
		}
		if nudge.Kind != "broadcast" && (state.State != "idle" || state.ActiveSprintID != nudge.ActiveSprintID) {
			return errors.New("agent is no longer idle in the selected nudge scope")
		}
		if ctx.LastProjectID == "" {
			ctx.LastProjectID = state.ProjectID
		}
		ctx.SessionID, ctx.Runtime = state.SessionID, state.Runtime
	} else {
		stateResponse.Body.Close()
		if stateResponse.StatusCode != http.StatusNotFound {
			return fmt.Errorf("work-state lookup returned %s", stateResponse.Status)
		}
	}
	if ctx.LastProjectID == "" {
		return errors.New("no project binding; pass --project P-ID")
	}
	endpoint := strings.TrimRight(tk.BaseURL, "/") + "/api/v1/projects/" + url.PathEscape(ctx.LastProjectID) + "/execution-tree"
	resp, err := tk.HTTPClient.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("project binding returned %s", resp.Status)
	}
	var tree model.ExecutionTree
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&tree); err != nil {
		return err
	}
	if tree.Project.ProjectID == "" {
		return errors.New("project binding has no canonical project id")
	}
	if nudge.ActiveSprintID != "" {
		sprint, ok := guardianFindSprint(tree.Categories, nudge.ActiveSprintID)
		if !ok || sprint.ProjectID != tree.Project.ProjectID {
			return errors.New("pending nudge Sprint does not belong to the bound project")
		}
		if sprint.Status != "Active" {
			return fmt.Errorf("pending nudge Sprint is %s; refusing to resume inactive work", sprint.Status)
		}
	}
	ctx.BaseURL = tk.BaseURL
	ctx.LastProjectID = tree.Project.ProjectID
	ctx.LastNudgeID = nudge.NudgeID
	ctx.LastSprintID = nudge.ActiveSprintID
	return nil
}

func guardianFindSprint(categories []model.ExecutionCategory, id string) (model.Sprint, bool) {
	for _, category := range categories {
		for _, task := range category.Tasks {
			for _, sprint := range task.Sprints {
				if sprint.SprintID == id {
					return sprint, true
				}
			}
			for _, subtask := range task.Subtasks {
				for _, sprint := range subtask.Sprints {
					if sprint.SprintID == id {
						return sprint, true
					}
				}
			}
		}
		if sprint, ok := guardianFindSprint(category.Categories, id); ok {
			return sprint, true
		}
	}
	return model.Sprint{}, false
}

func acknowledgeGuardianWake(tk *storeclient.Client, agentID string, nudgeID int64) error {
	endpoint := fmt.Sprintf("%s/api/v1/agents/%s/nudges/%d/ack", strings.TrimRight(tk.BaseURL, "/"), url.PathEscape(agentID), nudgeID)
	resp, err := tk.HTTPClient.Post(endpoint, "application/json", strings.NewReader("{}"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("acknowledgement returned %s", resp.Status)
	}
	var result model.PulseNudge
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result); err != nil {
		return err
	}
	if result.NudgeID != nudgeID || result.AgentID != agentID || result.Status != "Acknowledged" {
		return errors.New("acknowledgement did not confirm the selected agent and nudge")
	}
	return nil
}
