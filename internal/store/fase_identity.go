package store

import (
	"container/list"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const faseCacheLimit = 4096

type faseCacheEntry struct {
	fingerprint string
	identity    model.FASEIdentity
}
type faseRegistry struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   list.List
}

func (s *Store) migrateFASE(ctx context.Context) error {
	if err := s.ensureColumn(ctx, "project_events", "actor_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS fase_reserved_keys (agent_key TEXT PRIMARY KEY, CHECK(agent_key NOT GLOB '*[^A-Z0-9]*' AND agent_key GLOB '*[A-Z]*' AND agent_key GLOB '*[0-9]*'))`,
		`CREATE TABLE IF NOT EXISTS fase_agents (agent_key TEXT PRIMARY KEY REFERENCES fase_reserved_keys(agent_key), fingerprint TEXT NOT NULL UNIQUE, runtime TEXT NOT NULL, identity_scope TEXT NOT NULL, metadata_json TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS fase_aliases (fase_id TEXT PRIMARY KEY, agent_key TEXT NOT NULL REFERENCES fase_agents(agent_key), year INTEGER NOT NULL, month INTEGER NOT NULL CHECK(month BETWEEN 1 AND 12), first_used_at TEXT NOT NULL, UNIQUE(agent_key,year,month))`,
		`CREATE TABLE IF NOT EXISTS fase_registry_guard (id INTEGER PRIMARY KEY CHECK(id=1))`,
		`CREATE TABLE IF NOT EXISTS fase_execution_claims (item_id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(project_id), agent_key TEXT NOT NULL REFERENCES fase_agents(agent_key), fase_id TEXT NOT NULL, claimed_at TEXT NOT NULL)`,
		`INSERT OR IGNORE INTO fase_registry_guard(id) VALUES(1)`,
		`CREATE TRIGGER IF NOT EXISTS fase_key_no_delete BEFORE DELETE ON fase_reserved_keys BEGIN SELECT RAISE(ABORT,'FASE keys are permanently reserved'); END`,
		`CREATE TRIGGER IF NOT EXISTS fase_key_no_update BEFORE UPDATE ON fase_reserved_keys BEGIN SELECT RAISE(ABORT,'FASE keys are immutable'); END`,
		`CREATE TRIGGER IF NOT EXISTS fase_identity_no_rebind BEFORE UPDATE OF agent_key,fingerprint ON fase_agents BEGIN SELECT RAISE(ABORT,'FASE fingerprints cannot be rebound'); END`,
		`CREATE TRIGGER IF NOT EXISTS fase_identity_no_delete BEFORE DELETE ON fase_agents BEGIN SELECT RAISE(ABORT,'FASE identity history cannot be deleted'); END`,
		`CREATE TRIGGER IF NOT EXISTS fase_alias_no_update BEFORE UPDATE ON fase_aliases BEGIN SELECT RAISE(ABORT,'FASE aliases are immutable'); END`,
		`CREATE TRIGGER IF NOT EXISTS fase_alias_no_delete BEFORE DELETE ON fase_aliases BEGIN SELECT RAISE(ABORT,'FASE aliases are immutable'); END`,
	} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate FASE: %w", err)
		}
	}
	return nil
}

func newFASEKey() (string, error) {
	var entropy [5]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	// Fixed letter/digit guarantee mixed keys; the random portion supplies 40 bits.
	return "F" + strings.ToUpper(hex.EncodeToString(entropy[:])) + "4", nil
}
func faseAlias(key string, at time.Time) string {
	at = at.UTC()
	return fmt.Sprintf("FASE-ID.%s.%d.%d", key, int(at.Month()), at.Year())
}
func validateFASE(in model.FASEIdentityInput, at time.Time) error {
	decoded, err := hex.DecodeString(in.Fingerprint)
	if err != nil || len(decoded) != 32 || strings.ToLower(in.Fingerprint) != in.Fingerprint {
		return fmt.Errorf("%w: fingerprint must be 64 lowercase hexadecimal characters", ErrValidation)
	}
	if in.Runtime == "" || len(in.Runtime) > 64 || strings.TrimSpace(in.Runtime) != in.Runtime {
		return fmt.Errorf("%w: runtime must contain 1..64 characters", ErrValidation)
	}
	if in.IdentityScope != "runtime-session" && in.IdentityScope != "process-session" {
		return fmt.Errorf("%w: invalid identity scope", ErrValidation)
	}
	for _, value := range []string{in.Metadata.Provider, in.Metadata.Model, in.Metadata.ModelVersion, in.Metadata.RuntimeVersion} {
		if len(value) > 256 {
			return fmt.Errorf("%w: metadata fields must be at most 256 bytes", ErrValidation)
		}
	}
	if at.IsZero() || at.UTC().Year() < 1 || at.UTC().Year() > 9999 {
		return fmt.Errorf("%w: valid observation time required", ErrValidation)
	}
	return nil
}
func mergeFASEMetadata(old, incoming model.FASEMetadata) model.FASEMetadata {
	if incoming.Provider != "" {
		old.Provider = incoming.Provider
	}
	if incoming.Model != "" {
		old.Model = incoming.Model
	}
	if incoming.ModelVersion != "" {
		old.ModelVersion = incoming.ModelVersion
	}
	if incoming.RuntimeVersion != "" {
		old.RuntimeVersion = incoming.RuntimeVersion
	}
	return old
}
func (s *Store) ResolveFASEIdentityAt(ctx context.Context, in model.FASEIdentityInput, at time.Time) (model.FASEIdentity, error) {
	return s.resolveFASEIdentityAt(ctx, in, at, newFASEKey)
}
func (s *Store) resolveFASEIdentityAt(ctx context.Context, in model.FASEIdentityInput, at time.Time, nextKey func() (string, error)) (model.FASEIdentity, error) {
	if err := validateFASE(in, at); err != nil {
		return model.FASEIdentity{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.FASEIdentity{}, err
	}
	s.fase.mu.Lock()
	defer s.fase.mu.Unlock()
	if element := s.fase.entries[in.Fingerprint]; element != nil {
		id := element.Value.(faseCacheEntry).identity
		if id.Runtime != in.Runtime || id.IdentityScope != in.IdentityScope {
			return model.FASEIdentity{}, fmt.Errorf("%w: fingerprint metadata conflicts with assigned identity", ErrValidation)
		}
		if id.FASEID == faseAlias(id.AgentKey, at) && mergeFASEMetadata(id.Metadata, in.Metadata) == id.Metadata {
			s.fase.order.MoveToFront(element)
			return id, nil
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.FASEIdentity{}, err
	}
	defer tx.Rollback()
	// Acquire the database write lock before reading on cache miss. This also
	// serializes first-use requests from separate Store instances/processes.
	if _, err = tx.ExecContext(ctx, `UPDATE fase_registry_guard SET id=id WHERE id=1`); err != nil {
		return model.FASEIdentity{}, err
	}
	var id model.FASEIdentity
	var metadata, created string
	err = tx.QueryRowContext(ctx, `SELECT agent_key,runtime,identity_scope,metadata_json,created_at FROM fase_agents WHERE fingerprint=?`, in.Fingerprint).Scan(&id.AgentKey, &id.Runtime, &id.IdentityScope, &metadata, &created)
	if errors.Is(err, sql.ErrNoRows) {
		for attempt := 0; attempt < 32; attempt++ {
			key, e := nextKey()
			if e != nil {
				return id, e
			}
			result, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO fase_reserved_keys(agent_key) VALUES(?)`, key)
			if e != nil {
				return id, e
			}
			n, e := result.RowsAffected()
			if e != nil {
				return id, e
			}
			if n == 1 {
				id.AgentKey = key
				break
			}
		}
		if id.AgentKey == "" {
			return id, errors.New("could not reserve a unique FASE key")
		}
		id.Runtime, id.IdentityScope, id.Metadata, id.CreatedAt = in.Runtime, in.IdentityScope, in.Metadata, at.UTC()
		raw, _ := json.Marshal(id.Metadata)
		if _, err = tx.ExecContext(ctx, `INSERT INTO fase_agents(agent_key,fingerprint,runtime,identity_scope,metadata_json,created_at) VALUES(?,?,?,?,?,?)`, id.AgentKey, in.Fingerprint, id.Runtime, id.IdentityScope, string(raw), stamp(at)); err != nil {
			return id, err
		}
	} else if err != nil {
		return id, err
	} else {
		if id.Runtime != in.Runtime || id.IdentityScope != in.IdentityScope {
			return id, fmt.Errorf("%w: fingerprint metadata conflicts with assigned identity", ErrValidation)
		}
		if err = json.Unmarshal([]byte(metadata), &id.Metadata); err != nil {
			return id, err
		}
		if id.CreatedAt, err = parseTimestamp(created); err != nil {
			return id, err
		}
		merged := mergeFASEMetadata(id.Metadata, in.Metadata)
		if merged != id.Metadata {
			raw, _ := json.Marshal(merged)
			if _, err = tx.ExecContext(ctx, `UPDATE fase_agents SET metadata_json=? WHERE agent_key=?`, string(raw), id.AgentKey); err != nil {
				return id, err
			}
			id.Metadata = merged
		}
	}
	id.AgentID = "FASE-" + id.AgentKey
	id.FASEID = faseAlias(id.AgentKey, at)
	id.MaskedFingerprint = "*********" + in.Fingerprint[len(in.Fingerprint)-4:]
	id.MetadataSource = "unknown"
	if id.Metadata != (model.FASEMetadata{}) {
		id.MetadataSource = "runtime-reported"
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO fase_aliases(fase_id,agent_key,year,month,first_used_at) VALUES(?,?,?,?,?)`, id.FASEID, id.AgentKey, at.UTC().Year(), int(at.UTC().Month()), stamp(at)); err != nil {
		return id, err
	}
	if err = tx.Commit(); err != nil {
		return id, err
	}
	if s.fase.entries == nil {
		s.fase.entries = make(map[string]*list.Element)
	}
	if old := s.fase.entries[in.Fingerprint]; old != nil {
		s.fase.order.Remove(old)
	}
	s.fase.entries[in.Fingerprint] = s.fase.order.PushFront(faseCacheEntry{in.Fingerprint, id})
	if s.fase.order.Len() > faseCacheLimit {
		old := s.fase.order.Back()
		delete(s.fase.entries, old.Value.(faseCacheEntry).fingerprint)
		s.fase.order.Remove(old)
	}
	return id, nil
}

func (s *Store) FASEProfile(ctx context.Context, keyOrAlias string) (model.FASEProfile, error) {
	var result model.FASEProfile
	key := strings.TrimPrefix(keyOrAlias, "FASE-")
	if strings.HasPrefix(keyOrAlias, "FASE-ID.") {
		if err := s.db.QueryRowContext(ctx, `SELECT agent_key FROM fase_aliases WHERE fase_id=?`, keyOrAlias).Scan(&key); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return result, ErrNotFound
			}
			return result, err
		}
	}
	var fingerprint, raw, created string
	err := s.db.QueryRowContext(ctx, `SELECT agent_key,fingerprint,runtime,identity_scope,metadata_json,created_at FROM fase_agents WHERE agent_key=?`, key).Scan(&result.Identity.AgentKey, &fingerprint, &result.Identity.Runtime, &result.Identity.IdentityScope, &raw, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal([]byte(raw), &result.Identity.Metadata); err != nil {
		return result, err
	}
	if result.Identity.CreatedAt, err = parseTimestamp(created); err != nil {
		return result, err
	}
	result.Identity.AgentID = "FASE-" + key
	result.Identity.MaskedFingerprint = "*********" + fingerprint[len(fingerprint)-4:]
	result.Identity.MetadataSource = "unknown"
	if result.Identity.Metadata != (model.FASEMetadata{}) {
		result.Identity.MetadataSource = "runtime-reported"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT fase_id FROM fase_aliases WHERE agent_key=? ORDER BY year DESC,month DESC`, key)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.PreviousIDs = []string{}
	for rows.Next() {
		var alias string
		if err = rows.Scan(&alias); err != nil {
			return result, err
		}
		result.PreviousIDs = append(result.PreviousIDs, alias)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(result.PreviousIDs) > 0 {
		result.Identity.FASEID = result.PreviousIDs[0]
	}
	if strings.HasPrefix(keyOrAlias, "FASE-ID.") {
		result.Identity.FASEID = keyOrAlias
	}
	rows.Close()
	if err = s.populateFASEProfile(ctx, &result); err != nil {
		return result, err
	}
	return result, nil
}
