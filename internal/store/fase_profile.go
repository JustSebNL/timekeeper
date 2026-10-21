package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/JustSebNL/timekeeper/internal/model"
	"time"
)

func (s *Store) populateFASEProfile(ctx context.Context, p *model.FASEProfile) error {
	p.Events = []model.ProjectEvent{}
	rows, e := s.db.QueryContext(ctx, `SELECT event_id,project_id,entity_type,entity_id,event_type,message,created_at,actor_json FROM project_events WHERE actor_json!='' AND json_extract(actor_json,'$.fase_id')=? ORDER BY created_at DESC,event_id DESC LIMIT 101`, p.Identity.FASEID)
	if e != nil {
		return e
	}
	for rows.Next() {
		var event model.ProjectEvent
		var at, raw string
		if e = rows.Scan(&event.EventID, &event.ProjectID, &event.EntityType, &event.EntityID, &event.EventType, &event.Message, &at, &raw); e != nil {
			rows.Close()
			return e
		}
		if event.CreatedAt, e = parseTimestamp(at); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal([]byte(raw), &event.Actor); e != nil {
			rows.Close()
			return e
		}
		p.Events = append(p.Events, event)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if len(p.Events) > 100 {
		p.EventsTruncated = true
		p.Events = p.Events[:100]
	}
	p.Claims = []model.FASEClaim{}
	rows, e = s.db.QueryContext(ctx, "SELECT item_id,project_id,agent_key,fase_id,claimed_at FROM fase_execution_claims WHERE agent_key=? ORDER BY item_id", p.Identity.AgentKey)
	if e != nil {
		return e
	}
	for rows.Next() {
		var c model.FASEClaim
		var at string
		if e = rows.Scan(&c.ItemID, &c.ProjectID, &c.AgentKey, &c.FASEID, &at); e != nil {
			rows.Close()
			return e
		}
		if c.ClaimedAt, e = parseTimestamp(at); e != nil {
			rows.Close()
			return e
		}
		p.Claims = append(p.Claims, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	state, e := s.AgentWorkStateAt(ctx, p.Identity.AgentID, time.Now())
	if e == nil {
		p.WorkState = &state
	} else if !errors.Is(e, ErrNotFound) {
		return e
	}
	// Attempt to fetch token usage for this agent from usage sessions
	tokenSummary, err := s.fetchAgentTokenUsage(ctx, p.Identity.AgentID)
	if err == nil {
		p.TokenSummary = tokenSummary
	}
	return nil
}

func (s *Store) fetchAgentTokenUsage(ctx context.Context, agentID string) (*model.FASETokens, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cache_creation_tokens),0), MIN(created_at) FROM usage_sessions WHERE agent_id=?`, agentID)
	if e != nil {
		return nil, e
	}
	var input, output, cached int64
	var createdAt int64
	rows.Next()
	if e = rows.Scan(&input, &output, &cached, &createdAt); e != nil {
		rows.Close()
		return nil, e
	}
	rows.Close()
	return &model.FASETokens{
		InputTokens:  input,
		OutputTokens: output,
		CachedTokens: cached,
		CreatedAt:    createdAt,
	}, nil
}
