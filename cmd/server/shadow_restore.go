// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/instanceguard"
	"github.com/JustSebNL/timekeeper/internal/shadowcopy"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func resolveShadowCopyDirectory(databasePath, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		databasePath = strings.TrimSpace(databasePath)
		if databasePath == "" || databasePath == ":memory:" || strings.HasPrefix(databasePath, "file:") {
			return "", errors.New("shadow copy requires a file-backed primary or an explicit directory")
		}
		configured = filepath.Join(filepath.Dir(databasePath), "recovery", "sqlite-shadow")
	}
	absolute, err := filepath.Abs(configured)
	if err != nil {
		return "", fmt.Errorf("resolve shadow copy directory: %w", err)
	}
	return absolute, nil
}

func runShadowRestore(ctx context.Context, databasePath, shadowDirectory, slot, listenerAddress string, confirmed bool) (shadowcopy.RestoreResult, error) {
	if !confirmed {
		return shadowcopy.RestoreResult{}, errors.New("shadow restore requires -confirm-shadow-restore")
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(listenerAddress))
	if err != nil {
		return shadowcopy.RestoreResult{}, fmt.Errorf("shadow restore listener address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return shadowcopy.RestoreResult{}, errors.New("shadow restore requires a numeric loopback listener address")
	}
	lock, err := instanceguard.Acquire(databasePath)
	if err != nil {
		return shadowcopy.RestoreResult{}, fmt.Errorf("refuse shadow restore while Core may be running: %w", err)
	}
	defer lock.Close()
	listener, err := net.Listen("tcp", listenerAddress)
	if err != nil {
		return shadowcopy.RestoreResult{}, fmt.Errorf("refuse shadow restore while Core listener is unavailable at %s: %w", listenerAddress, err)
	}
	defer listener.Close()
	return shadowcopy.Restore(ctx, databasePath, shadowDirectory, slot, store.CheckDatabaseFileIntegrity)
}
