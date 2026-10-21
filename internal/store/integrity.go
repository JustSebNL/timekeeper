// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// IntegrityCheck performs SQLite's full integrity_check against the live Store
// connection. A non-ok result is returned as an error with all diagnostics.
func (s *Store) IntegrityCheck(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("check Time Keeper database integrity: store is closed")
	}
	return sqliteIntegrityCheck(ctx, s.db, "live Time Keeper database")
}

// CheckDatabaseFileIntegrity checks a standalone SQLite file through a read-only
// connection. It does not migrate or otherwise modify the candidate.
func CheckDatabaseFileIntegrity(ctx context.Context, path string) error {
	path = strings.TrimSpace(path)
	if path == "" || path == ":memory:" {
		return errors.New("check SQLite candidate integrity: provide a filesystem path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve SQLite candidate path: %w", err)
	}
	uri := url.URL{Scheme: "file"}
	if filepath.VolumeName(absolute) != "" {
		// SQLite's URI parser treats file:///C:/... as an authority named C:.
		// Opaque form keeps the Windows volume as part of the path: file:C:/....
		uri.Opaque = filepath.ToSlash(absolute)
	} else {
		uri.Path = filepath.ToSlash(absolute)
	}
	query := url.Values{}
	query.Set("mode", "ro")
	database, err := sql.Open("sqlite", uri.String()+"?"+query.Encode())
	if err != nil {
		return fmt.Errorf("open SQLite candidate read-only: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	if err := database.PingContext(ctx); err != nil {
		return fmt.Errorf("open SQLite candidate %q: %w", absolute, err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		return fmt.Errorf("enable read-only SQLite candidate check: %w", err)
	}
	return sqliteIntegrityCheck(ctx, database, absolute)
}

func sqliteIntegrityCheck(ctx context.Context, database *sql.DB, label string) error {
	rows, err := database.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("SQLite integrity_check for %s: %w", label, err)
	}
	defer rows.Close()
	var diagnostics []string
	for rows.Next() {
		var diagnostic string
		if err := rows.Scan(&diagnostic); err != nil {
			return fmt.Errorf("read SQLite integrity_check for %s: %w", label, err)
		}
		diagnostics = append(diagnostics, diagnostic)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate SQLite integrity_check for %s: %w", label, err)
	}
	if len(diagnostics) == 1 && diagnostics[0] == "ok" {
		return nil
	}
	if len(diagnostics) == 0 {
		return fmt.Errorf("SQLite integrity_check for %s returned no diagnostics", label)
	}
	return fmt.Errorf("SQLite integrity_check for %s failed: %s", label, strings.Join(diagnostics, "; "))
}
