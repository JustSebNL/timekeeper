// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package logging provides a process-wide structured logger for TimeKeeper
// (log/slog) with a rotating file sink. Two modes:
//
//   - normal  (info): command names, status, durations, top-level outcomes
//   - debug   (debug): every step, every byte of request body, raw inputs,
//                      and full stack traces on every error
//
// Crashes and request errors are written to both stderr and
// .timekeeper/log/app.log so we can bugfix them later.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

var (
	mu       sync.RWMutex
	active   *slog.Logger
	activeFh *os.File
	activeMd *Mode
)

// Mode controls how much detail the logger emits.
//   - ModeNormal: command names, status codes, durations, top-level outcomes
//   - ModeDebug:  every step, every byte of request body, raw inputs,
//                 and full stack traces on every error
type Mode int

const (
	ModeNormal Mode = iota
	ModeDebug
)

// String implements flag.Value.
func (m Mode) String() string {
	switch m {
	case ModeDebug:
		return "debug"
	default:
		return "normal"
	}
}

// Set implements flag.Value.
func (m *Mode) Set(s string) error {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug", "verbose", "trace":
		*m = ModeDebug
		return nil
	case "normal", "info", "":
		*m = ModeNormal
		return nil
	default:
		return fmt.Errorf("unknown log mode %q (use normal|debug)", s)
	}
}

// Config controls the file logger. Path "" disables the file sink.
type Config struct {
	Path       string
	Mode       Mode
	MaxSizeMiB int
	MaxBackups int
}

// Init configures the process-wide logger. Safe to call multiple times.
// Returns the active logger.
func Init(cfg Config) *slog.Logger {
	mu.Lock()
	defer mu.Unlock()

	if active != nil {
		return active
	}

	level := slog.LevelInfo
	if cfg.Mode == ModeDebug {
		level = slog.LevelDebug
	}

	var writers []io.Writer
	writers = append(writers, os.Stderr)

	if cfg.Path != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err == nil {
			fh, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				activeFh = fh
				writers = append(writers, &rotatingWriter{
					path:       cfg.Path,
					fh:         fh,
					maxBytes:   int64(cfg.MaxSizeMiB) * 1024 * 1024,
					maxBackups: cfg.MaxBackups,
				})
			}
		}
	}

	md := cfg.Mode
	activeMd = &md
	handler := slog.NewJSONHandler(io.MultiWriter(writers...), &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})
	active = slog.New(handler)
	slog.SetDefault(active)
	active.Info("logger initialised",
		slog.String("mode", md.String()),
		slog.String("path", cfg.Path),
		slog.String("level", level.String()),
	)
	return active
}

// L returns the process-wide logger, initialising a sane default if needed.
func L() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if active != nil {
		return active
	}
	return Init(Config{Path: ""})
}

// CurrentMode returns the active log mode (or ModeNormal if not initialised).
func CurrentMode() Mode {
	mu.RLock()
	defer mu.RUnlock()
	if activeMd == nil {
		return ModeNormal
	}
	return *activeMd
}

// IsDebug returns true when the active logger is in debug mode.
func IsDebug() bool { return CurrentMode() == ModeDebug }

// Close flushes and closes the underlying file sink (if any).
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if activeFh != nil {
		_ = activeFh.Close()
		activeFh = nil
	}
}

// Recover is a panic-recovery helper that writes a structured crash report.
// Includes the full request (method, path, headers, body) when ctx is an
// *http.Request or carries one. Stack trace is always included.
func Recover(ctx context.Context, where string, err any) bool {
	logger := L()
	stack := debug.Stack()
	attrs := []any{
		slog.String("where", where),
		slog.Any("panic", err),
		slog.String("stack", string(stack)),
		slog.String("go_version", runtime.Version()),
		slog.Time("at", time.Now().UTC()),
	}
	if req, ok := ctxRequest(ctx); ok {
		attrs = append(attrs, slog.String("method", req.Method))
		attrs = append(attrs, slog.String("path", req.URL.Path))
		attrs = append(attrs, slog.String("raw_query", req.URL.RawQuery))
		attrs = append(attrs, slog.String("remote", remoteAddr(req)))
		attrs = append(attrs, slog.String("ua", req.UserAgent()))
		attrs = append(attrs, slog.String("headers", dumpHeaders(req.Header)))
		if body, berr := readBody(req); berr == nil && len(body) > 0 {
			attrs = append(attrs, slog.String("body", truncateForLog(body, 8192)))
		}
	}
	logger.Error("panic recovered", attrs...)
	return true
}

// Step records a single named step in a long-running operation. In normal mode
// these are demoted; in debug mode they appear at Debug level with the full
// payload. Use this for the "every step taken" requirement.
func Step(ctx context.Context, op, step string, attrs ...any) {
	logger := L()
	all := append([]any{slog.String("op", op), slog.String("step", step)}, attrs...)
	if IsDebug() {
		logger.Debug("step", all...)
		return
	}
	// Normal mode: collapse steps into a single trace line per op (still logged).
	logger.Info("step", all...)
}

// Command records the entry/exit of a CLI command. Normal: command name +
// duration + status. Debug: full argv, stdin if any, env subset, step trace.
func Command(name string, fn func() error, attrs ...any) error {
	logger := L()
	start := time.Now()
	if IsDebug() {
		logger.Debug("command start", slog.String("cmd", name), slog.Any("args", attrs))
	} else {
		logger.Info("command", slog.String("cmd", name), slog.Any("args", attrs))
	}
	err := fn()
	dur := time.Since(start)
	if err != nil {
		logger.Error("command failed",
			slog.String("cmd", name),
			slog.Duration("duration", dur),
			slog.Any("err", err),
			slog.String("trace", string(debug.Stack())),
		)
		return err
	}
	if IsDebug() {
		logger.Debug("command ok", slog.String("cmd", name), slog.Duration("duration", dur))
	} else {
		logger.Info("command ok", slog.String("cmd", name), slog.Duration("duration", dur))
	}
	return nil
}

// RequestLogger writes one structured line per HTTP request. Normal mode logs
// method/path/status/duration/UA. Debug mode adds query, body, request id,
// response bytes, and per-step traces.
type RequestLogger struct {
	Logger *slog.Logger
}

// NewRequestLogger builds a RequestLogger using the process-wide logger.
func NewRequestLogger() *RequestLogger {
	return &RequestLogger{Logger: L()}
}

// Wrap returns an http.Handler-style wrapper. Use as:
//   h := logging.NewRequestLogger().Wrap(actualHandler)
func (r *RequestLogger) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, req)
		dur := time.Since(start)
		base := []any{
			slog.String("method", req.Method),
			slog.String("path", req.URL.Path),
			slog.Int("status", rw.status),
			slog.Int("bytes", rw.bytes),
			slog.Duration("duration", dur),
			slog.String("remote", remoteAddr(req)),
			slog.String("ua", req.UserAgent()),
			slog.String("rid", requestID(req)),
		}
		if IsDebug() {
			base = append(base, slog.String("query", req.URL.RawQuery))
			base = append(base, slog.String("headers", dumpHeaders(req.Header)))
			if body, err := readBody(req); err == nil && len(body) > 0 {
				base = append(base, slog.String("body", truncateForLog(body, 16384)))
			}
			r.Logger.Debug("http request", base...)
		} else {
			r.Logger.Info("http request", base...)
		}
	})
}

// statusRecorder wraps http.ResponseWriter to capture status code and bytes.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func remoteAddr(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}

func requestID(r *http.Request) string {
	if v := r.Header.Get("X-Request-Id"); v != "" {
		return v
	}
	return fmt.Sprintf("local-%d", time.Now().UnixNano())
}

func dumpHeaders(h http.Header) string {
	if len(h) == 0 {
		return ""
	}
	var b strings.Builder
	for k, vs := range h {
		for _, v := range vs {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteString("\n")
		}
	}
	return truncateForLog([]byte(b.String()), 4096)
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	// Read up to 1MB so we never blow up memory on a giant payload.
	const max = 1 << 20
	buf, err := io.ReadAll(io.LimitReader(r.Body, max))
	if err != nil {
		return nil, err
	}
	// Restore body for downstream handlers.
	r.Body = io.NopCloser(strings.NewReader(string(buf)))
	return buf, nil
}

func truncateForLog(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + fmt.Sprintf("...(+%d bytes)", len(b)-n)
}

func ctxRequest(ctx context.Context) (*http.Request, bool) {
	if ctx == nil {
		return nil, false
	}
	if r := ctx.Value(httpRequestKey{}); r != nil {
		if req, ok := r.(*http.Request); ok {
			return req, true
		}
	}
	return nil, false
}

// WithRequest attaches an *http.Request to a context so Recover() can dump it.
func WithRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestKey{}, r)
}

type httpRequestKey struct{}

// rotatingWriter rotates the underlying file when it exceeds maxBytes.
type rotatingWriter struct {
	path       string
	fh         *os.File
	maxBytes   int64
	maxBackups int
}

type backupEntry struct {
	name string
	mod  time.Time
}

func (r *rotatingWriter) Write(p []byte) (int, error) {
	if r.maxBytes <= 0 {
		return r.fh.Write(p)
	}
	info, err := r.fh.Stat()
	if err == nil && info.Size()+int64(len(p)) > r.maxBytes {
		_ = r.fh.Close()
		ts := time.Now().UTC().Format("20060102-150405.000")
		rotated := fmt.Sprintf("%s.%s", r.path, ts)
		_ = os.Rename(r.path, rotated)
		if r.maxBackups > 0 {
			r.pruneOld(rotated)
		}
		fh, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return 0, err
		}
		r.fh = fh
	}
	return r.fh.Write(p)
}

func (r *rotatingWriter) pruneOld(recent string) {
	dir := filepath.Dir(r.path)
	base := filepath.Base(r.path)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var backups []backupEntry
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), base+".") || e.Name() == filepath.Base(recent) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupEntry{e.Name(), info.ModTime()})
	}
	if len(backups) <= r.maxBackups {
		return
	}
	sortByMod(backups)
	toRemove := len(backups) - r.maxBackups
	for i := 0; i < toRemove; i++ {
		_ = os.Remove(filepath.Join(dir, backups[i].name))
	}
}

func sortByMod(es []backupEntry) {
	for i := 1; i < len(es); i++ {
		for j := i; j > 0 && es[j].mod.Before(es[j-1].mod); j-- {
			es[j], es[j-1] = es[j-1], es[j]
		}
	}
}
