package model

type CodingOutcomeInput struct {
	SessionID     string `json:"session_id"`
	AgentID       string `json:"agent_id"`
	Provider      string `json:"provider"`
	Category      string `json:"category"`
	Difficulty    string `json:"difficulty"`
	Benchmark     string `json:"benchmark"`
	Configuration string `json:"configuration"`
	Result        string `json:"result"`
	Reviewer      string `json:"reviewer"`
	Evidence      string `json:"evidence"`
	ReworkCount   int64  `json:"rework_count"`
}
type CodingOutcome struct {
	CodingOutcomeInput
	Model               string `json:"model"`
	SprintID            string `json:"sprint_id"`
	ReviewedTurn        int64  `json:"reviewed_turn"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
}
type CodingComparison struct {
	Model               string  `json:"model"`
	Provider            string  `json:"provider"`
	Category            string  `json:"category"`
	Difficulty          string  `json:"difficulty"`
	Benchmark           string  `json:"benchmark"`
	Configuration       string  `json:"configuration"`
	Samples             int64   `json:"samples"`
	Accepted            int64   `json:"accepted"`
	FirstPassAccepted   int64   `json:"first_pass_accepted"`
	ReworkCount         int64   `json:"rework_count"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	AcceptanceRate      float64 `json:"acceptance_rate"`
	EvidenceStatus      string  `json:"evidence_status"`
}
type CodingComparisons struct {
	Items              []CodingComparison `json:"items"`
	Outcomes           []CodingOutcome    `json:"outcomes"`
	UnreviewedSessions int64              `json:"unreviewed_sessions"`
	Basis              string             `json:"basis"`
}
