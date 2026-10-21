package main

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/desktop"
	"github.com/JustSebNL/timekeeper/internal/version"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestStartupReportsUnconfiguredCore(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	app := newDesktopApp(desktop.Config{CoreURL: "http://127.0.0.1:" + strconv.Itoa(port)}, func(ctx context.Context, cfg desktop.Config) (coreSession, error) {
		return desktop.StartOrAttach(ctx, cfg, nil)
	})
	notifications := &fakeNotificationProvider{}
	app.notifications = notifications

	app.startup(context.Background())

	if got := app.Status().State; got != "configuration_required" {
		t.Fatalf("Status().State = %q, want configuration_required", got)
	}
	if got := notifications.sent; len(got) != 1 || got[0].title != startupFailureTitle {
		t.Fatalf("startup notifications = %+v, want one startup failure notice", got)
	}
}

func TestConnectionBindingProbeIsReadOnlyWithoutAnAttachedSession(t *testing.T) {
	var methods, paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/version":
			_, _ = fmt.Fprintf(w, `{"service":"TimeKeeper","version":%q}`, version.Get().Version)
		case "/api/v1/connection-info":
			_, _ = fmt.Fprintf(w, `{"service":"TimeKeeper","instance_id":"tk_instance_0123456789abcdef0123456789abcdef","version":%q,"api_version":"v1","capabilities":["projects.list","tasks.lifecycle"]}`, version.Get().Version)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := newDesktopApp(desktop.Config{CoreURL: server.URL}, nil)
	diagnostic, err := app.TestConnection()
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.ClientSurface != "desktop" || diagnostic.SelectedEndpoint != server.URL || diagnostic.UsedFallback || diagnostic.ConnectionInfo.InstanceID == "" {
		t.Fatalf("TestConnection() = %+v", diagnostic)
	}
	for i, method := range methods {
		if method != http.MethodGet {
			t.Fatalf("connection diagnostic request %d used %s %s; want GET-only", i, method, paths[i])
		}
	}
	if len(methods) < 3 {
		t.Fatalf("connection diagnostic made too few requests: methods=%v paths=%v", methods, paths)
	}
}

func TestStartupFailureNotificationUsesSanitizedStatus(t *testing.T) {
	notifications := &fakeNotificationProvider{}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return nil, fmt.Errorf("database unavailable at C:\\Users\\private\\core.db token=secret-value")
	})
	app.notifications = notifications
	app.startup(context.Background())

	if len(notifications.sent) != 1 {
		t.Fatalf("startup notifications = %+v, want one startup failure notice", notifications.sent)
	}
	body := notifications.sent[0].body
	for _, sensitive := range []string{"C:\\Users\\private", "secret-value"} {
		if strings.Contains(body, sensitive) {
			t.Fatalf("notification exposed sensitive diagnostic %q: %q", sensitive, body)
		}
	}
}

func TestCloseHidesWindowOnlyWhenExplicitlyEnabled(t *testing.T) {
	tests := []struct {
		name       string
		hide       bool
		wantCancel bool
		wantHides  int
	}{
		{name: "default exits", wantCancel: false, wantHides: 0},
		{name: "opt in hides", hide: true, wantCancel: true, wantHides: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window := &fakeWindowController{}
			app := newDesktopApp(desktop.Config{}, nil)
			app.window = window
			app.hideOnClose = tt.hide

			if got := app.beforeClose(context.Background()); got != tt.wantCancel {
				t.Fatalf("beforeClose() = %v, want %v", got, tt.wantCancel)
			}
			if window.hideCalls != tt.wantHides {
				t.Fatalf("window hide calls = %d, want %d", window.hideCalls, tt.wantHides)
			}
		})
	}
}

func TestTrayActionsShowWindowAndExitApplication(t *testing.T) {
	window := &fakeWindowController{}
	app := newDesktopApp(desktop.Config{}, nil)
	app.window = window
	app.runtimeCtx = context.Background()
	app.hideOnClose = true

	app.showWindow()
	app.exit()
	if canceled := app.beforeClose(context.Background()); canceled {
		t.Fatal("tray Exit was intercepted by hide-on-close, want application exit")
	}

	if window.showCalls != 1 || window.quitCalls != 1 {
		t.Fatalf("tray window actions = show:%d quit:%d, want show:1 quit:1", window.showCalls, window.quitCalls)
	}
}

func TestLiveCoreReadOnlyBindings(t *testing.T) {
	coreURL := strings.TrimSpace(os.Getenv("TIMEKEEPER_DESKTOP_LIVE_CORE_URL"))
	if coreURL == "" {
		t.Skip("set TIMEKEEPER_DESKTOP_LIVE_CORE_URL to opt into the live Core integration check")
	}
	app := newDesktopApp(desktop.Config{CoreURL: coreURL}, func(ctx context.Context, cfg desktop.Config) (coreSession, error) {
		return desktop.StartOrAttach(ctx, cfg, nil)
	})
	app.startup(context.Background())
	if status := app.Status(); status.State != "connected" || status.Owned {
		t.Fatalf("live Core startup status = %+v, want attached connected Core", status)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	health, err := app.Health()
	if err != nil || health.Status != "ok" {
		t.Fatalf("desktop Health binding = %+v, %v; want ok", health, err)
	}
	projects, err := app.Projects()
	if err != nil {
		t.Fatalf("desktop Projects binding: %v", err)
	}
	if projects == nil {
		t.Fatal("desktop Projects binding returned nil, want the Core project collection")
	}
	found := false
	for _, project := range projects {
		if project.ProjectID == "P-10053" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("live Core project collection did not contain P-10053 (count %d)", len(projects))
	}
	if err := app.shutdown(ctx); err != nil {
		t.Fatalf("detach from live Core: %v", err)
	}
	client, err := desktop.NewCoreClient(coreURL)
	if err != nil {
		t.Fatalf("create post-detach Core client: %v", err)
	}
	health, err = client.Health(ctx)
	if err != nil || health.Status != "ok" {
		t.Fatalf("Core health after desktop detach = %+v, %v; want ok", health, err)
	}
}

func TestStartupFailureShowsSanitizedDiagnostic(t *testing.T) {
	err := fmt.Errorf("%w: database is locked at C:\\Users\\private\\core.db token=secret-value", desktop.ErrNotConfigured)
	status := statusForError(err)
	if !strings.Contains(status.Message, "database is locked") {
		t.Fatalf("status message omitted actionable reason: %q", status.Message)
	}
	for _, sensitive := range []string{"C:\\Users\\private", "secret-value"} {
		if strings.Contains(status.Message, sensitive) {
			t.Fatalf("status message exposed sensitive diagnostic %q: %q", sensitive, status.Message)
		}
	}
}

func TestSanitizeDiagnosticRedactsPathsContainingSpaces(t *testing.T) {
	raw := `failed to open C:\Program Files\TimeKeeper\core.db: permission denied; retry /home/Private User/timekeeper/core.db: access denied`
	got := sanitizeDiagnostic(raw)
	for _, sensitive := range []string{`C:\Program Files`, `TimeKeeper\core.db`, `/home/Private User`, `timekeeper/core.db`} {
		if strings.Contains(got, sensitive) {
			t.Fatalf("sanitized diagnostic leaked %q: %q", sensitive, got)
		}
	}
}

func TestEmbeddedAssetsContainProductionSvelteEntryAndBundles(t *testing.T) {
	index, err := embeddedAssets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read embedded frontend index: %v", err)
	}
	content := string(index)
	if !strings.Contains(content, `<div id="app"></div>`) {
		t.Fatal("embedded index does not contain the Svelte application mount")
	}
	if !strings.Contains(content, `src="./assets/index-`) || !strings.Contains(content, `.js"`) {
		t.Fatal("embedded index does not reference a production JavaScript bundle")
	}
	if !strings.Contains(content, `href="./assets/index-`) || !strings.Contains(content, `.css"`) {
		t.Fatal("embedded index does not reference a production stylesheet")
	}
	for _, pattern := range []string{"assets/assets/index-*.js", "assets/assets/index-*.css"} {
		matches, err := fs.Glob(embeddedAssets, pattern)
		if err != nil || len(matches) == 0 {
			t.Fatalf("embedded assets matching %q = %v, err = %v", pattern, matches, err)
		}
		for _, name := range matches {
			if _, err := embeddedAssets.ReadFile(name); err != nil {
				t.Errorf("embedded bundle %q is unreadable: %v", name, err)
			}
		}
	}
}

func TestShutdownClosesOwnedSession(t *testing.T) {
	session := &fakeCoreSession{owned: true}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return session, nil
	})
	app.startup(context.Background())

	if err := app.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if session.closeCalls != 1 {
		t.Fatalf("owned session close calls = %d, want 1", session.closeCalls)
	}
}

func TestShutdownLeavesAttachedSessionRunning(t *testing.T) {
	session := &fakeCoreSession{owned: false}
	app := newDesktopApp(desktop.Config{}, func(context.Context, desktop.Config) (coreSession, error) {
		return session, nil
	})
	app.startup(context.Background())

	if err := app.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if session.closeCalls != 0 {
		t.Fatalf("attached session close calls = %d, want 0", session.closeCalls)
	}
}

func TestNativeDialogsUseWailsContextAndReturnSelectedPaths(t *testing.T) {
	wailsCtx := context.WithValue(context.Background(), dialogContextKey{}, "wails-runtime")
	provider := &fakeDialogProvider{
		openFileResult:      `C:\Users\me\project.json`,
		openDirectoryResult: `C:\Users\me\workspace`,
		saveFileResult:      `C:\Users\me\export.json`,
	}
	app := newDesktopApp(desktop.Config{}, nil)
	app.dialogs = provider
	app.startup(wailsCtx)

	options := DialogOptions{
		Title:            "Choose a project",
		DefaultDirectory: `C:\Users\me`,
		DefaultFilename:  "project.json",
		Filters:          []DialogFilter{{DisplayName: "JSON files", Pattern: "*.json"}},
	}

	gotFile, err := app.OpenFileDialog(options)
	if err != nil || gotFile != provider.openFileResult {
		t.Fatalf("OpenFileDialog() = %q, %v; want selected path %q", gotFile, err, provider.openFileResult)
	}
	gotFolder, err := app.OpenDirectoryDialog(options)
	if err != nil || gotFolder != provider.openDirectoryResult {
		t.Fatalf("OpenDirectoryDialog() = %q, %v; want selected path %q", gotFolder, err, provider.openDirectoryResult)
	}
	gotSave, err := app.SaveFileDialog(options)
	if err != nil || gotSave != provider.saveFileResult {
		t.Fatalf("SaveFileDialog() = %q, %v; want selected path %q", gotSave, err, provider.saveFileResult)
	}

	if len(provider.calls) != 3 {
		t.Fatalf("native dialog calls = %d, want 3", len(provider.calls))
	}
	for _, call := range provider.calls {
		if call.ctx.Value(dialogContextKey{}) != "wails-runtime" {
			t.Errorf("%s dialog did not receive the Wails runtime context", call.name)
		}
		if call.title != "Choose a project" || call.defaultDirectory != `C:\Users\me` || call.defaultFilename != "project.json" {
			t.Errorf("%s dialog options = %+v, want the supplied title and defaults", call.name, call)
		}
		if len(call.filters) != 1 || call.filters[0] != (wailsruntime.FileFilter{DisplayName: "JSON files", Pattern: "*.json"}) {
			t.Errorf("%s dialog filters = %+v, want the supplied JSON filter", call.name, call.filters)
		}
	}
}

func TestNativeDialogsReturnEmptyPathWhenCancelled(t *testing.T) {
	app := newDesktopApp(desktop.Config{}, nil)
	app.dialogs = &fakeDialogProvider{}
	app.startup(context.Background())

	for _, invoke := range []struct {
		name string
		call func() (string, error)
	}{
		{name: "open file", call: func() (string, error) { return app.OpenFileDialog(DialogOptions{}) }},
		{name: "open folder", call: func() (string, error) { return app.OpenDirectoryDialog(DialogOptions{}) }},
		{name: "save file", call: func() (string, error) { return app.SaveFileDialog(DialogOptions{}) }},
	} {
		t.Run(invoke.name, func(t *testing.T) {
			path, err := invoke.call()
			if err != nil || path != "" {
				t.Fatalf("cancelled dialog = %q, %v; want empty path and no error", path, err)
			}
		})
	}
}

func TestNativeDialogsRejectCallsOutsideWailsLifecycle(t *testing.T) {
	provider := &fakeDialogProvider{}
	app := newDesktopApp(desktop.Config{}, nil)
	app.dialogs = provider
	if _, err := app.OpenFileDialog(DialogOptions{}); err == nil {
		t.Fatal("OpenFileDialog before startup succeeded, want lifecycle error")
	}

	app.startup(context.Background())
	if err := app.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if _, err := app.OpenFileDialog(DialogOptions{}); err == nil {
		t.Fatal("OpenFileDialog after shutdown succeeded, want lifecycle error")
	}
	if len(provider.calls) != 0 {
		t.Fatalf("native dialog calls outside lifecycle = %d, want 0", len(provider.calls))
	}
}

func TestShutdownCancelsStartupAndClosesLateOwnedSession(t *testing.T) {
	session := &fakeCoreSession{owned: true}
	started := make(chan struct{})
	app := newDesktopApp(desktop.Config{}, func(ctx context.Context, _ desktop.Config) (coreSession, error) {
		close(started)
		<-ctx.Done()
		return session, nil
	})

	startupCtx, cancelStartup := context.WithCancel(context.Background())
	defer cancelStartup()
	startupDone := make(chan struct{})
	go func() {
		app.startup(startupCtx)
		close(startupDone)
	}()
	<-started

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := app.shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case <-startupDone:
	case <-time.After(time.Second):
		cancelStartup()
		<-startupDone
		t.Fatal("shutdown did not cancel and wait for in-flight startup")
	}
	if session.closeCalls != 1 {
		t.Fatalf("late owned session close calls = %d, want 1", session.closeCalls)
	}
	if got := app.Status().State; got == "connected" {
		t.Fatalf("status after shutdown = %q, want non-connected", got)
	}
}

type fakeCoreSession struct {
	owned      bool
	closeCalls int
	closeErr   error
}

type sentNotification struct {
	title string
	body  string
}

type fakeNotificationProvider struct {
	initializeErr error
	sendErr       error
	sent          []sentNotification
}

func (p *fakeNotificationProvider) Initialize(context.Context) error { return p.initializeErr }
func (p *fakeNotificationProvider) Send(_ context.Context, title, body string) error {
	p.sent = append(p.sent, sentNotification{title: title, body: body})
	return p.sendErr
}

type fakeWindowController struct{ hideCalls, showCalls, quitCalls int }

func (w *fakeWindowController) Hide(context.Context) { w.hideCalls++ }
func (w *fakeWindowController) Show(context.Context) { w.showCalls++ }
func (w *fakeWindowController) Quit(context.Context) { w.quitCalls++ }

type dialogContextKey struct{}

type fakeDialogCall struct {
	name             string
	ctx              context.Context
	title            string
	defaultDirectory string
	defaultFilename  string
	filters          []wailsruntime.FileFilter
}

type fakeDialogProvider struct {
	calls               []fakeDialogCall
	openFileResult      string
	openDirectoryResult string
	saveFileResult      string
	err                 error
}

func (p *fakeDialogProvider) OpenFile(ctx context.Context, options wailsruntime.OpenDialogOptions) (string, error) {
	p.calls = append(p.calls, fakeDialogCall{name: "open file", ctx: ctx, title: options.Title, defaultDirectory: options.DefaultDirectory, defaultFilename: options.DefaultFilename, filters: options.Filters})
	return p.openFileResult, p.err
}

func (p *fakeDialogProvider) OpenDirectory(ctx context.Context, options wailsruntime.OpenDialogOptions) (string, error) {
	p.calls = append(p.calls, fakeDialogCall{name: "open folder", ctx: ctx, title: options.Title, defaultDirectory: options.DefaultDirectory, defaultFilename: options.DefaultFilename, filters: options.Filters})
	return p.openDirectoryResult, p.err
}

func (p *fakeDialogProvider) SaveFile(ctx context.Context, options wailsruntime.SaveDialogOptions) (string, error) {
	p.calls = append(p.calls, fakeDialogCall{name: "save file", ctx: ctx, title: options.Title, defaultDirectory: options.DefaultDirectory, defaultFilename: options.DefaultFilename, filters: options.Filters})
	return p.saveFileResult, p.err
}

func (s *fakeCoreSession) Client() *desktop.CoreClient { return nil }
func (s *fakeCoreSession) Owned() bool                 { return s.owned }
func (s *fakeCoreSession) Close(context.Context) error {
	s.closeCalls++
	return s.closeErr
}
