package model

type SupervisedLaunchInput struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
}
type SupervisedLaunch struct {
	RemainingSeconds int64 `json:"remaining_seconds"`
	Attempts         int   `json:"attempts"`
}
