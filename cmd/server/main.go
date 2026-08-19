// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/guardian"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func recoveryPolicy() string {
	// The repo-local recovery receiver performs exactly one allowlisted action:
	// write a durable local recovery artifact and acknowledge the nudge. It
	// performs no process control. This string is status evidence, not a grant.
	return "local-artifact: durable recovery marker only; no process control"
}

func validateServerConfig(addr, uiPath string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("listen address %q must include host and port: %w", addr, err)
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber > 65535 {
		return fmt.Errorf("listen address %q has an invalid port", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen address %q must use a numeric loopback host; Time Keeper is local-first", addr)
	}
	absolute, err := filepath.Abs(strings.TrimSpace(uiPath))
	if err != nil || strings.TrimSpace(uiPath) == "" {
		return fmt.Errorf("dashboard path must be a valid file path")
	}
	for _, candidate := range []string{absolute, filepath.Join(filepath.Dir(absolute), "timekeeper.css"), filepath.Join(filepath.Dir(absolute), "timekeeper.js")} {
		info, err := os.Stat(candidate)
		if err != nil {
			return fmt.Errorf("dashboard asset unavailable: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("dashboard asset %q is not a regular file", candidate)
		}
		if info.Mode().Perm()&0o444 == 0 {
			return fmt.Errorf("dashboard asset %q is not readable", candidate)
		}
		file, err := os.Open(candidate)
		if err != nil {
			return fmt.Errorf("dashboard asset unavailable: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close dashboard validation file: %w", err)
		}
	}
	return nil
}

func main() {
	addr := flag.String("addr", "127.0.0.1:1618", "HTTP listen address")
	dbPath := flag.String("db", "timekeeper.db", "SQLite database path")
	uiPath := flag.String("ui", "web/timekeeper.html", "dashboard HTML path")
	backupTo := flag.String("backup-to", "", "create a SQLite backup at this new path, then exit")
	pulseGuardianInterval := flag.Duration("pulse-guardian-interval", 0, "run the local Pulse Guardian at this interval; 0 disables it")
	flag.Parse()
	if *pulseGuardianInterval != 0 && *pulseGuardianInterval < time.Second {
		log.Fatalf("Pulse Guardian interval must be at least %s", time.Second)
	}
	if *backupTo == "" {
		if err := validateServerConfig(*addr, *uiPath); err != nil {
			log.Fatalf("validate Time Keeper configuration: %v", err)
		}
	}

	database, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open Time Keeper database: %v", err)
	}
	defer database.Close()
	if *backupTo != "" {
		message, err := runBackup(context.Background(), database, *backupTo)
		if err != nil {
			log.Fatalf("Time Keeper backup: %v", err)
		}
		log.Print(message)
		return
	}
	if *pulseGuardianInterval > 0 {
		guardianContext, guardianStop := context.WithCancel(context.Background())
		defer guardianStop()
		go guardian.Run(guardianContext, database, *pulseGuardianInterval, func(err error) {
			log.Printf("Pulse Guardian: %v", err)
		})
		log.Printf("Pulse Guardian enabled with %s interval", *pulseGuardianInterval)
	}

	apiHandler := api.NewWithRuntime(database, api.RuntimeStatus{
		PulseGuardianEnabled:         *pulseGuardianInterval > 0,
		PulseGuardianIntervalSeconds: int64(pulseGuardianInterval.Seconds()),
		RecoveryPolicy:               recoveryPolicy(),
	})
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/health", apiHandler)
	mux.HandleFunc("/", dashboard(*uiPath))

	server := &http.Server{
		Addr:              *addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("Time Keeper listening at http://%s/", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Time Keeper server: %v", err)
	}
}

func runBackup(ctx context.Context, database *store.Store, destination string) (string, error) {
	if err := database.BackupTo(ctx, destination); err != nil {
		return "", err
	}
	return "Time Keeper backup created: " + destination, nil
}

func dashboard(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assets := map[string]string{
			"/":               path,
			"/timekeeper.css": filepath.Join(filepath.Dir(path), "timekeeper.css"),
			"/timekeeper.js":  filepath.Join(filepath.Dir(path), "timekeeper.js"),
		}
		asset, ok := assets[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		absolute, err := filepath.Abs(asset)
		if err != nil {
			http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
			return
		}
		if _, err := os.Stat(absolute); err != nil {
			http.Error(w, "dashboard unavailable", http.StatusServiceUnavailable)
			return
		}
		http.ServeFile(w, r, absolute)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
