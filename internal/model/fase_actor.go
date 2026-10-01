package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type faseActorKey struct{}

func WithFASEActor(ctx context.Context, actor FASEIdentity) context.Context {
	return context.WithValue(ctx, faseActorKey{}, actor)
}
func FASEActorFromContext(ctx context.Context) (FASEIdentity, bool) {
	a, ok := ctx.Value(faseActorKey{}).(FASEIdentity)
	return a, ok && a.AgentKey != ""
}

// FASEActivitySession is an attribution token, not an authentication credential.
func FASEActivitySession(fingerprint string) string {
	sum := sha256.Sum256([]byte("fase-activity-session-v1\x00" + fingerprint))
	return hex.EncodeToString(sum[:])
}

type FASEClaim struct {
	ItemID    string    `json:"item_id"`
	ProjectID string    `json:"project_id"`
	AgentKey  string    `json:"agent_key"`
	FASEID    string    `json:"fase_id"`
	ClaimedAt time.Time `json:"claimed_at"`
}
