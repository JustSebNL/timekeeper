package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/JustSebNL/timekeeper/internal/model"
	"strings"
	"time"
)

// ResolveFASEActor validates the attribution pair against Core's registry.
// This is not an authentication boundary: deployments still need access control.
func (s *Store) ResolveFASEActor(ctx context.Context, agentID, session string, at time.Time) (model.FASEIdentity, error) {
	key := strings.TrimPrefix(agentID, "FASE-")
	var in model.FASEIdentityInput
	s.fase.mu.Lock()
	for _, entry := range s.fase.entries {
		cached := entry.Value.(faseCacheEntry)
		if cached.identity.AgentKey == key {
			in = model.FASEIdentityInput{Fingerprint: cached.fingerprint, Runtime: cached.identity.Runtime, IdentityScope: cached.identity.IdentityScope}
			break
		}
	}
	s.fase.mu.Unlock()
	if in.Fingerprint == "" {
		e := s.db.QueryRowContext(ctx, "SELECT fingerprint,runtime,identity_scope FROM fase_agents WHERE agent_key=?", key).Scan(&in.Fingerprint, &in.Runtime, &in.IdentityScope)
		if errors.Is(e, sql.ErrNoRows) {
			return model.FASEIdentity{}, ErrNotFound
		}
		if e != nil {
			return model.FASEIdentity{}, e
		}
	}
	if model.FASEActivitySession(in.Fingerprint) != session {
		return model.FASEIdentity{}, fmt.Errorf("%w: FASE activity session does not match registered identity", ErrAgentSessionConflict)
	}
	return s.ResolveFASEIdentityAt(ctx, in, at)
}
