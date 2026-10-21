package model

import "time"

const (
	RegistrationQueueStatusQueued     = "Queued"
	RegistrationQueueStatusRegistered = "Registered"
	RegistrationQueueStatusRejected   = "Rejected"
)

const (
	RegistrationQueueKindTask           = "task"
	RegistrationQueueKindSubtask        = "subtask"
	RegistrationQueueKindSprint         = "sprint"
	RegistrationQueueKindTaskBatch      = "task_batch"
	RegistrationQueueKindExecutionBatch = "execution_batch"
)

// RegistrationQueueItem is a durable API offer. Payload remains available
// until registration succeeds or is explicitly rejected, so accepted work is
// never only in a request buffer.
type RegistrationQueueItem struct {
	QueueSeq       int64                  `json:"-"`
	QueueID        string                 `json:"queue_id"`
	ProjectID      string                 `json:"project_id"`
	Kind           string                 `json:"kind"`
	Status         string                 `json:"status"`
	IdempotencyKey string                 `json:"idempotency_key,omitempty"`
	PayloadHash    string                 `json:"payload_hash"`
	Payload        *ExecutionBatchInput   `json:"payload,omitempty"`
	Counts         BatchCounts            `json:"counts"`
	Validation     *BatchValidationResult `json:"validation,omitempty"`
	Result         *ExecutionBatchResult  `json:"result,omitempty"`
	Attempts       int                    `json:"attempts"`
	LastError      string                 `json:"last_error,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	ProcessedAt    *time.Time             `json:"processed_at,omitempty"`
}
