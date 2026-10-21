package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const instanceIdentityPrefix = "tk_instance_"

// InstanceID returns the stable identity persisted with this Core database.
// Copies of the same database retain the same logical-instance identity.
func (s *Store) InstanceID(ctx context.Context) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("read instance identity: store is closed")
	}
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT instance_id FROM instance_identity WHERE singleton=1`).Scan(&id); err != nil {
		return "", fmt.Errorf("read instance identity: %w", err)
	}
	if !validInstanceID(id) {
		return "", fmt.Errorf("read instance identity: stored ID has invalid format")
	}
	return id, nil
}

func (s *Store) ensureInstanceIdentity(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS instance_identity (
		singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
		instance_id TEXT NOT NULL UNIQUE CHECK (length(instance_id) = 44),
		created_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create instance identity table: %w", err)
	}
	var existing string
	err := s.db.QueryRowContext(ctx, `SELECT instance_id FROM instance_identity WHERE singleton=1`).Scan(&existing)
	if err == nil {
		if !validInstanceID(existing) {
			return fmt.Errorf("stored instance ID has invalid format")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read stored instance identity: %w", err)
	}
	id, err := newInstanceID()
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO instance_identity(singleton, instance_id, created_at) VALUES(1, ?, ?)`, id, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("initialize instance identity: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT instance_id FROM instance_identity WHERE singleton=1`).Scan(&existing); err != nil {
		return fmt.Errorf("verify instance identity: %w", err)
	}
	if !validInstanceID(existing) {
		return fmt.Errorf("initialized instance ID has invalid format")
	}
	return nil
}

func newInstanceID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate instance identity: %w", err)
	}
	return instanceIdentityPrefix + hex.EncodeToString(raw[:]), nil
}

func validInstanceID(id string) bool {
	if !strings.HasPrefix(id, instanceIdentityPrefix) {
		return false
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(id, instanceIdentityPrefix))
	return err == nil && len(raw) == 16
}
