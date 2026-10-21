package main

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/JustSebNL/timekeeper/internal/desktop"
	"github.com/JustSebNL/timekeeper/internal/model"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const requestDeadline = 3 * time.Second
const startupShutdownTimeout = 6 * time.Second
const startupFailureTitle = "TimeKeeper Core is offline"

var (
	diagnosticSecretPattern = regexp.MustCompile(`(?i)\b(password|token|secret|api[_-]?key)(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;]+)`)
	diagnosticWindowsPath   = regexp.MustCompile(`(?i)\b[A-Z]:\\[^,;|<>"?*\r\n]+`)
	diagnosticUnixPath      = regexp.MustCompile(`(?:^|[ =])/(?:[^,;|<>"?*\r\n]+)`)
)

type coreSession interface {
	Client() *desktop.CoreClient
	Owned() bool
	Close(context.Context) error
}

type sessionStarter func(context.Context, desktop.Config) (coreSession, error)

type DialogFilter struct {
	DisplayName string `json:"displayName"`
	Pattern     string `json:"pattern"`
}

// DialogOptions contains only the path-selection settings exposed to the UI.
// The desktop adapter never reads or writes the selected file.
type DialogOptions struct {
	Title            string         `json:"title"`
	DefaultDirectory string         `json:"defaultDirectory"`
	DefaultFilename  string         `json:"defaultFilename"`
	Filters          []DialogFilter `json:"filters"`
}

type nativeDialogProvider interface {
	OpenFile(context.Context, wailsruntime.OpenDialogOptions) (string, error)
	OpenDirectory(context.Context, wailsruntime.OpenDialogOptions) (string, error)
	SaveFile(context.Context, wailsruntime.SaveDialogOptions) (string, error)
}

type wailsDialogProvider struct{}

func (wailsDialogProvider) OpenFile(ctx context.Context, options wailsruntime.OpenDialogOptions) (string, error) {
	return wailsruntime.OpenFileDialog(ctx, options)
}

type nativeNotificationProvider interface {
	Initialize(context.Context) error
	Send(context.Context, string, string) error
}

type wailsNotificationProvider struct{}

func (wailsNotificationProvider) Initialize(ctx context.Context) error {
	return wailsruntime.InitializeNotifications(ctx)
}

func (wailsNotificationProvider) Send(ctx context.Context, title, body string) error {
	return wailsruntime.SendNotification(ctx, wailsruntime.NotificationOptions{Title: title, Body: body})
}

type windowController interface {
	Hide(context.Context)
	Show(context.Context)
	Quit(context.Context)
}

type wailsWindowController struct{}

func (wailsWindowController) Hide(ctx context.Context) { wailsruntime.WindowHide(ctx) }
func (wailsWindowController) Show(ctx context.Context) { wailsruntime.WindowShow(ctx) }
func (wailsWindowController) Quit(ctx context.Context) { wailsruntime.Quit(ctx) }

func (wailsDialogProvider) OpenDirectory(ctx context.Context, options wailsruntime.OpenDialogOptions) (string, error) {
	return wailsruntime.OpenDirectoryDialog(ctx, options)
}

func (wailsDialogProvider) SaveFile(ctx context.Context, options wailsruntime.SaveDialogOptions) (string, error) {
	return wailsruntime.SaveFileDialog(ctx, options)
}

type DesktopStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
	Owned   bool   `json:"owned"`
}

// DesktopApp is the thin Wails-facing adapter. Its exported methods are
// context-free because Wails exposes them as JavaScript bindings.
type DesktopApp struct {
	cfg   desktop.Config
	start sessionStarter

	mu            sync.RWMutex
	runtimeCtx    context.Context
	dialogs       nativeDialogProvider
	notifications nativeNotificationProvider
	window        windowController
	hideOnClose   bool
	exitRequested bool
	session       coreSession
	status        DesktopStatus
	closing       bool
	startCancel   context.CancelFunc
	startDone     chan struct{}
}

func newDesktopApp(cfg desktop.Config, start sessionStarter) *DesktopApp {
	return &DesktopApp{
		cfg:     cfg,
		start:   start,
		dialogs: wailsDialogProvider{},
		status: DesktopStatus{
			State:   "starting",
			Message: "Connecting to TimeKeeper Core…",
		},
	}
}

func (a *DesktopApp) startup(ctx context.Context) {
	a.mu.Lock()
	if a.closing || a.startDone != nil {
		a.mu.Unlock()
		return
	}
	a.runtimeCtx = ctx
	notifications := a.notifications
	startCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	a.startCancel = cancel
	a.startDone = done
	a.mu.Unlock()
	if notifications != nil {
		if err := notifications.Initialize(ctx); err != nil {
			log.Printf("initialize desktop notifications: %v", err)
		}
	}

	if a.start == nil {
		a.finishStartup(done, nil, errors.New("desktop Core startup is unavailable"))
		cancel()
		return
	}
	session, err := a.start(startCtx, a.cfg)
	cancel()
	a.finishStartup(done, session, err)
}

func (a *DesktopApp) finishStartup(done chan struct{}, session coreSession, err error) {
	a.mu.Lock()
	a.startCancel = nil
	closing := a.closing
	if !closing {
		if err != nil {
			a.status = statusForError(err)
		} else if session == nil {
			a.status = DesktopStatus{State: "offline", Message: "Core startup returned no session."}
		} else {
			a.session = session
			a.status = DesktopStatus{State: "connected", Message: "Connected to TimeKeeper Core.", Owned: session.Owned()}
		}
	}
	a.mu.Unlock()

	if closing && session != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), startupShutdownTimeout)
		defer cancel()
		_ = session.Close(closeCtx)
	}
	close(done)
	if closing {
		return
	}
	if err != nil {
		a.notifyStartupFailure()
		return
	}
	if session == nil {
		a.notifyStartupFailure()
		return
	}
}

func (a *DesktopApp) notifyStartupFailure() {
	a.mu.RLock()
	ctx, notifications, status := a.runtimeCtx, a.notifications, a.status
	closing := a.closing
	a.mu.RUnlock()
	if closing || ctx == nil || ctx.Err() != nil || notifications == nil {
		return
	}
	if err := notifications.Send(ctx, startupFailureTitle, status.Message); err != nil {
		log.Printf("send desktop startup notification: %v", err)
	}
}

func (a *DesktopApp) beforeClose(ctx context.Context) bool {
	a.mu.RLock()
	hide, exiting, window := a.hideOnClose, a.exitRequested, a.window
	a.mu.RUnlock()
	if exiting || !hide || window == nil {
		return false
	}
	window.Hide(ctx)
	return true
}

func (a *DesktopApp) showWindow() {
	a.mu.RLock()
	ctx, window := a.runtimeCtx, a.window
	a.mu.RUnlock()
	if ctx != nil && ctx.Err() == nil && window != nil {
		window.Show(ctx)
	}
}

func (a *DesktopApp) exit() {
	a.mu.Lock()
	a.exitRequested = true
	ctx, window := a.runtimeCtx, a.window
	a.mu.Unlock()
	if ctx != nil && ctx.Err() == nil && window != nil {
		window.Quit(ctx)
	}
}

func statusForError(err error) DesktopStatus {
	switch {
	case errors.Is(err, desktop.ErrNotConfigured):
		return statusWithDiagnostic("configuration_required", "Core is offline. Configure the Core executable, database path, and dashboard UI path to start it.", err)
	case errors.Is(err, desktop.ErrIncompatibleCore):
		return statusWithDiagnostic("incompatible_core", "Another service is using this Core address, or its TimeKeeper version is incompatible.", err)
	case errors.Is(err, desktop.ErrCoreUnavailable), errors.Is(err, desktop.ErrStartupTimeout), errors.Is(err, desktop.ErrChildExited):
		return statusWithDiagnostic("offline", "TimeKeeper Core could not be reached or started. Check the Core URL and launch configuration.", err)
	default:
		return statusWithDiagnostic("offline", "TimeKeeper Core startup failed. Check the Core URL, launch configuration, and local logs.", err)
	}
}

func statusWithDiagnostic(state, message string, err error) DesktopStatus {
	detail := sanitizeDiagnostic(err.Error())
	if detail != "" {
		message += " Details: " + detail
	}
	return DesktopStatus{State: state, Message: message}
}

func sanitizeDiagnostic(raw string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, raw)
	clean = strings.Join(strings.Fields(clean), " ")
	clean = diagnosticSecretPattern.ReplaceAllString(clean, "$1$2[REDACTED]")
	clean = diagnosticWindowsPath.ReplaceAllString(clean, "[path]")
	clean = diagnosticUnixPath.ReplaceAllString(clean, " [path]")
	runes := []rune(clean)
	if len(runes) > 400 {
		clean = string(runes[:400]) + "…"
	}
	return clean
}

func (a *DesktopApp) shutdown(ctx context.Context) error {
	a.mu.Lock()
	a.closing = true
	a.runtimeCtx = nil
	startCancel := a.startCancel
	startDone := a.startDone
	session := a.session
	a.session = nil
	a.mu.Unlock()
	if startCancel != nil {
		startCancel()
	}
	if startDone != nil {
		select {
		case <-startDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if session == nil {
		return nil
	}
	if !session.Owned() {
		return nil
	}
	if err := session.Close(ctx); err != nil {
		a.setStatus(DesktopStatus{State: "offline", Message: "The owned TimeKeeper Core did not shut down cleanly."})
		return err
	}
	a.setStatus(DesktopStatus{State: "offline", Message: "Desktop session closed."})
	return nil
}

func (a *DesktopApp) setStatus(status DesktopStatus) {
	a.mu.Lock()
	a.status = status
	a.mu.Unlock()
}

func (a *DesktopApp) Status() DesktopStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

func (a *DesktopApp) client() (*desktop.CoreClient, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.session == nil || a.session.Client() == nil {
		return nil, errors.New("TimeKeeper Core is not connected")
	}
	return a.session.Client(), nil
}

func (a *DesktopApp) Health() (desktop.Health, error) {
	client, err := a.client()
	if err != nil {
		return desktop.Health{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.Health(ctx)
}

func (a *DesktopApp) Version() (desktop.Version, error) {
	client, err := a.client()
	if err != nil {
		return desktop.Version{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.Version(ctx)
}

func (a *DesktopApp) Projects() ([]model.Project, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestDeadline)
	defer cancel()
	return client.Projects(ctx)
}

func (a *DesktopApp) dialogRuntime() (context.Context, nativeDialogProvider, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closing || a.runtimeCtx == nil {
		return nil, nil, errors.New("native dialogs are unavailable outside the active desktop lifecycle")
	}
	if err := a.runtimeCtx.Err(); err != nil {
		return nil, nil, err
	}
	if a.dialogs == nil {
		return nil, nil, errors.New("native dialog provider is unavailable")
	}
	return a.runtimeCtx, a.dialogs, nil
}

func toWailsFilters(filters []DialogFilter) []wailsruntime.FileFilter {
	if len(filters) == 0 {
		return nil
	}
	result := make([]wailsruntime.FileFilter, len(filters))
	for i, filter := range filters {
		result[i] = wailsruntime.FileFilter{DisplayName: filter.DisplayName, Pattern: filter.Pattern}
	}
	return result
}

func (a *DesktopApp) OpenFileDialog(options DialogOptions) (string, error) {
	ctx, dialogs, err := a.dialogRuntime()
	if err != nil {
		return "", err
	}
	return dialogs.OpenFile(ctx, wailsruntime.OpenDialogOptions{
		Title: options.Title, DefaultDirectory: options.DefaultDirectory,
		DefaultFilename: options.DefaultFilename, Filters: toWailsFilters(options.Filters),
	})
}

func (a *DesktopApp) OpenDirectoryDialog(options DialogOptions) (string, error) {
	ctx, dialogs, err := a.dialogRuntime()
	if err != nil {
		return "", err
	}
	return dialogs.OpenDirectory(ctx, wailsruntime.OpenDialogOptions{
		Title: options.Title, DefaultDirectory: options.DefaultDirectory,
		DefaultFilename: options.DefaultFilename, Filters: toWailsFilters(options.Filters),
	})
}

func (a *DesktopApp) SaveFileDialog(options DialogOptions) (string, error) {
	ctx, dialogs, err := a.dialogRuntime()
	if err != nil {
		return "", err
	}
	return dialogs.SaveFile(ctx, wailsruntime.SaveDialogOptions{
		Title: options.Title, DefaultDirectory: options.DefaultDirectory,
		DefaultFilename: options.DefaultFilename, Filters: toWailsFilters(options.Filters),
	})
}
