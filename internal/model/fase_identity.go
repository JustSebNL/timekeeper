package model

import "time"

// Metadata is reported by the runtime, never inferred from an agent label.
type FASEMetadata struct {
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	ModelVersion   string `json:"model_version,omitempty"`
	RuntimeVersion string `json:"runtime_version,omitempty"`
}
type FASEIdentityInput struct {
	Fingerprint   string       `json:"fingerprint"`
	Runtime       string       `json:"runtime"`
	IdentityScope string       `json:"identity_scope"`
	Metadata      FASEMetadata `json:"metadata"`
}
type FASEResumeInput struct {
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
}
type FASEIdentity struct {
	AgentKey          string       `json:"agent_key"`
	AgentID           string       `json:"agent_id"`
	FASEID            string       `json:"fase_id"`
	MaskedFingerprint string       `json:"masked_fingerprint"`
	Runtime           string       `json:"runtime"`
	IdentityScope     string       `json:"identity_scope"`
	Metadata          FASEMetadata `json:"metadata"`
	MetadataSource    string       `json:"metadata_source"`
	CreatedAt         time.Time    `json:"created_at"`
}
type FASEProfile struct {
	Identity        FASEIdentity             `json:"identity"`
	Events          []ProjectEvent           `json:"events"`
	EventsTruncated bool                     `json:"events_truncated"`
	Claims          []FASEClaim              `json:"execution_claims"`
	WorkState       *AgentWorkState          `json:"work_state,omitempty"`
	TokenSummary    *FASETokens              `json:"token_summary,omitempty"`
	// These are all actually used periods, including the latest one.
	PreviousIDs []string `json:"previous_fase_ids"`
}

type FASETokens struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CachedTokens int64 `json:"cached_tokens"`
	CreatedAt    int64 `json:"created_at"`
}
