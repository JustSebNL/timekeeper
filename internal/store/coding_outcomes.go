package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/JustSebNL/timekeeper/internal/model"
	"sort"
	"strings"
	"time"
)

func (s *Store) migrateCodingOutcomes(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS coding_outcomes(usage_id INTEGER PRIMARY KEY REFERENCES agent_usage_sessions(usage_id) ON DELETE RESTRICT,project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,payload TEXT NOT NULL,created_at TEXT NOT NULL)`)
	return err
}
func (s *Store) RecordCodingOutcome(ctx context.Context, p string, in model.CodingOutcomeInput) (model.CodingOutcome, bool, error) {
	for _, v := range []string{in.SessionID, in.AgentID, in.Provider, in.Category, in.Difficulty, in.Benchmark, in.Configuration, in.Reviewer, in.Evidence} {
		if strings.TrimSpace(v) == "" || len(v) > 4000 {
			return model.CodingOutcome{}, false, errors.New("identity, comparison cohort, reviewer and evidence are required (maximum 4000 bytes each)")
		}
	}
	if (in.Result != "accepted" && in.Result != "rejected") || in.ReworkCount < 0 {
		return model.CodingOutcome{}, false, errors.New("result must be accepted or rejected and rework_count non-negative")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.CodingOutcome{}, false, err
	}
	defer tx.Rollback()
	row, err := readUsageSession(ctx, tx, p, in.AgentID, in.SessionID)
	if err != nil {
		return model.CodingOutcome{}, false, fmt.Errorf("a recorded usage session in this project is required: %w", err)
	}
	if row.model == "" || !row.sprintID.Valid || row.sprintID.String == "" {
		return model.CodingOutcome{}, false, errors.New("usage must have a known model and linked Sprint before review")
	}
	var previous string
	err = tx.QueryRowContext(ctx, "SELECT payload FROM coding_outcomes WHERE usage_id=?", row.usageID).Scan(&previous)
	if err == nil {
		var old model.CodingOutcome
		if err = json.Unmarshal([]byte(previous), &old); err != nil {
			return old, false, err
		}
		if old.CodingOutcomeInput != in {
			return old, false, errors.New("outcome already recorded; conflicting reviews cannot overwrite evidence")
		}
		return old, true, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return model.CodingOutcome{}, false, err
	}
	result := model.CodingOutcome{CodingOutcomeInput: in, Model: row.model, SprintID: row.sprintID.String, ReviewedTurn: row.turnSeq, InputTokens: row.inputTokens, OutputTokens: row.outputTokens, CacheReadTokens: row.cacheReadTokens, CacheCreationTokens: row.cacheCreationTokens}
	payload, err := json.Marshal(result)
	if err != nil {
		return result, false, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO coding_outcomes(usage_id,project_id,payload,created_at) VALUES(?,?,?,?)", row.usageID, p, string(payload), stamp(time.Now().UTC()))
	if err != nil {
		return result, false, err
	}
	return result, false, tx.Commit()
}
func (s *Store) CodingComparisons(ctx context.Context, p string) (model.CodingComparisons, error) {
	out := model.CodingComparisons{Items: []model.CodingComparison{}, Outcomes: []model.CodingOutcome{}, Basis: "Reported reviewer evidence at a frozen usage turn; compare only matching category/difficulty/benchmark/configuration cohorts. Counts are runs, not whole tasks. No independent proof or cost is inferred."}
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id=?", p).Scan(&exists); err != nil {
		return out, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT payload FROM coding_outcomes WHERE project_id=? ORDER BY created_at,usage_id", p)
	if err != nil {
		return out, err
	}
	groups := map[string]*model.CodingComparison{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			rows.Close()
			return out, err
		}
		var v model.CodingOutcome
		if err := json.Unmarshal([]byte(payload), &v); err != nil {
			rows.Close()
			return out, err
		}
		out.Outcomes = append(out.Outcomes, v)
		keyBytes, _ := json.Marshal([]string{v.Category, v.Difficulty, v.Benchmark, v.Configuration, v.Provider, v.Model})
		key := string(keyBytes)
		g := groups[key]
		if g == nil {
			g = &model.CodingComparison{Model: v.Model, Provider: v.Provider, Category: v.Category, Difficulty: v.Difficulty, Benchmark: v.Benchmark, Configuration: v.Configuration, EvidenceStatus: "reported evidence; limited sample"}
			groups[key] = g
		}
		g.Samples++
		if v.Result == "accepted" {
			g.Accepted++
			if v.ReworkCount == 0 {
				g.FirstPassAccepted++
			}
		}
		g.ReworkCount += v.ReworkCount
		g.InputTokens += v.InputTokens
		g.OutputTokens += v.OutputTokens
		g.CacheReadTokens += v.CacheReadTokens
		g.CacheCreationTokens += v.CacheCreationTokens
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_usage_sessions u LEFT JOIN coding_outcomes o ON o.usage_id=u.usage_id WHERE u.project_id=? AND o.usage_id IS NULL`, p).Scan(&out.UnreviewedSessions); err != nil {
		return out, err
	}
	keys := []string{}
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		g.AcceptanceRate = float64(g.Accepted) / float64(g.Samples)
		out.Items = append(out.Items, *g)
	}
	// Stable ranking within like-for-like cohorts, never across difficulty or harnesses.
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		ka, _ := json.Marshal([]string{a.Category, a.Difficulty, a.Benchmark, a.Configuration})
		kb, _ := json.Marshal([]string{b.Category, b.Difficulty, b.Benchmark, b.Configuration})
		if string(ka) != string(kb) {
			return string(ka) < string(kb)
		}
		return a.AcceptanceRate > b.AcceptanceRate
	})
	return out, nil
}
