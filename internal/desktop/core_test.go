package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/version"
)

func TestStartOrAttach(t *testing.T) {
	tests := []struct {
		name           string
		healthCode     int
		healthBody     string
		version        string
		connectionInfo string
		wantErr        bool
		wantReason     string
	}{
		{name: "attaches to compatible core", healthCode: http.StatusOK, healthBody: `{"status":"ok"}`, version: version.Get().Version},
		{name: "rejects non-200 health", healthCode: http.StatusServiceUnavailable, healthBody: `{"status":"ok"}`, version: version.Get().Version, wantErr: true},
		{name: "rejects malformed health", healthCode: http.StatusOK, healthBody: `{`, version: version.Get().Version, wantErr: true},
		{name: "rejects wrong service", healthCode: http.StatusOK, healthBody: `{"status":"ok"}`, version: `{"service":"Other","version":"` + version.Get().Version + `"}`, wantErr: true},
		{name: "rejects incompatible version", healthCode: http.StatusOK, healthBody: `{"status":"ok"}`, version: `{"service":"TimeKeeper","version":"99.0.0"}`, wantErr: true},
		{name: "rejects malformed version", healthCode: http.StatusOK, healthBody: `{"status":"ok"}`, version: `{`, wantErr: true},
		{name: "rejects missing instance identity", healthCode: http.StatusOK, healthBody: `{"status":"ok"}`, version: version.Get().Version, connectionInfo: `{"service":"TimeKeeper","version":"` + version.Get().Version + `","api_version":"v1","capabilities":["projects.list","tasks.lifecycle"]}`, wantErr: true, wantReason: "instance ID"},
		{name: "rejects incompatible API version", healthCode: http.StatusOK, healthBody: `{"status":"ok"}`, version: version.Get().Version, connectionInfo: `{"service":"TimeKeeper","instance_id":"tk_instance_0123456789abcdef0123456789abcdef","version":"` + version.Get().Version + `","api_version":"v2","capabilities":["projects.list","tasks.lifecycle"]}`, wantErr: true, wantReason: "API version"},
		{name: "rejects missing required capability", healthCode: http.StatusOK, healthBody: `{"status":"ok"}`, version: version.Get().Version, connectionInfo: `{"service":"TimeKeeper","instance_id":"tk_instance_0123456789abcdef0123456789abcdef","version":"` + version.Get().Version + `","api_version":"v1","capabilities":["projects.list"]}`, wantErr: true, wantReason: "tasks.lifecycle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/health":
					w.WriteHeader(tt.healthCode)
					_, _ = w.Write([]byte(tt.healthBody))
				case "/api/v1/version":
					if strings.HasPrefix(tt.version, "{") {
						_, _ = w.Write([]byte(tt.version))
					} else {
						_ = json.NewEncoder(w).Encode(map[string]string{"service": "TimeKeeper", "version": tt.version})
					}
				case "/api/v1/connection-info":
					body := tt.connectionInfo
					if body == "" {
						body = `{"service":"TimeKeeper","instance_id":"tk_instance_0123456789abcdef0123456789abcdef","version":"` + version.Get().Version + `","api_version":"v1","capabilities":["projects.list","tasks.lifecycle"]}`
					}
					_, _ = w.Write([]byte(body))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			launcher := &fakeLauncher{}
			session, err := StartOrAttach(context.Background(), Config{CoreURL: server.URL}, launcher)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected incompatible Core error")
				}
				if tt.wantReason != "" && !strings.Contains(err.Error(), tt.wantReason) {
					t.Fatalf("error %q does not explain %q", err, tt.wantReason)
				}
				if launcher.startCount != 0 {
					t.Fatalf("launched %d child processes for an occupied endpoint", launcher.startCount)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if session.Owned() {
				t.Fatal("attached Core was marked as owned")
			}
			if launcher.startCount != 0 {
				t.Fatalf("attached Core started %d child processes", launcher.startCount)
			}
			if err := session.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCoreClientReadsTypedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/version":
			_, _ = w.Write([]byte(`{"service":"TimeKeeper","version":"` + version.Get().Version + `","commit":"abc"}`))
		case "/api/v1/connection-info":
			_, _ = w.Write([]byte(`{"service":"TimeKeeper","instance_id":"tk_instance_0123456789abcdef0123456789abcdef","version":"` + version.Get().Version + `","api_version":"v1","capabilities":["projects.list","tasks.lifecycle"],"friendly_urls":["http://timekeeper.local/"]}`))
		case "/api/v1/projects":
			_, _ = w.Write([]byte(`{"items":[{"project_id":"P-7","project_name":"Example","status":"Open"}]}`))
		case "/api/v1/agents/work-states":
			_, _ = w.Write([]byte(`{"items":[{"agent_id":"agent-1","session_id":"session-1","state":"working","reported_state":"working"}]}`))
		case "/api/v1/projects/P-7/handoff":
			_, _ = w.Write([]byte(`{"project":{"project_id":"P-7","project_name":"Example","status":"Open"},"recent_events":[{"event_id":3,"entity_type":"task","entity_id":"T-3","event_type":"task_started","message":"Task started","created_at":"2026-10-18T10:00:00Z"}],"recent_notes":[{"note_id":7,"project_id":"P-7","content":"Capture the decision","created_at":"2026-10-18T09:00:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	session, err := StartOrAttach(context.Background(), Config{CoreURL: server.URL}, &fakeLauncher{})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(context.Background())

	health, err := session.Client().Health(context.Background())
	if err != nil || health.Status != "ok" {
		t.Fatalf("Health() = %+v, %v", health, err)
	}
	versionInfo, err := session.Client().Version(context.Background())
	if err != nil || versionInfo.Service != "TimeKeeper" || versionInfo.Commit != "abc" {
		t.Fatalf("Version() = %+v, %v", versionInfo, err)
	}
	connectionInfo, err := session.Client().ConnectionInfo(context.Background())
	if err != nil || connectionInfo.InstanceID != "tk_instance_0123456789abcdef0123456789abcdef" || connectionInfo.APIVersion != "v1" || len(connectionInfo.FriendlyURLs) != 1 {
		t.Fatalf("ConnectionInfo() = %+v, %v", connectionInfo, err)
	}
	projects, err := session.Client().Projects(context.Background())
	if err != nil || len(projects) != 1 || projects[0].ProjectID != "P-7" || projects[0].ProjectName != "Example" {
		t.Fatalf("Projects() = %+v, %v", projects, err)
	}
	states, err := session.Client().AgentWorkStates(context.Background())
	if err != nil || len(states) != 1 || states[0].AgentID != "agent-1" || states[0].State != "working" {
		t.Fatalf("AgentWorkStates() = %+v, %v", states, err)
	}
	handoff, err := session.Client().ProjectHandoff(context.Background(), "P-7")
	if err != nil || handoff.Project.ProjectID != "P-7" || len(handoff.RecentEvents) != 1 || handoff.RecentEvents[0].EntityID != "T-3" || len(handoff.RecentNotes) != 1 {
		t.Fatalf("ProjectHandoff() = %+v, %v", handoff, err)
	}
}

func TestCoreClientGlobalSearchUsesVersionedQueryContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/search" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("q") != "queue" || query.Get("limit") != "2" || strings.Join(query["scope"], ",") != "projects,tasks" {
			t.Fatalf("search query = %v", query)
		}
		_, _ = fmt.Fprint(w, `{"query":"queue","scopes":["projects","tasks"],"items":[{"type":"task","project_id":"P-7","project_name":"Example","object_id":"T-1","ref":"T-1","title":"Queue task","snippet":"queue worker","score":200,"created_at":"2026-10-18T10:00:00Z","updated_at":"2026-10-18T10:00:00Z"}],"total":1,"limit":2,"has_more":false}`)
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.GlobalSearch(context.Background(), " queue ", 2, []string{"projects", "tasks"})
	if err != nil || response.Query != "queue" || response.Total != 1 || len(response.Items) != 1 || response.Items[0].ObjectID != "T-1" {
		t.Fatalf("GlobalSearch() = %+v, %v", response, err)
	}
}

func TestCoreClientDoesNotExposeErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/version":
			_, _ = fmt.Fprintf(w, `{"service":"TimeKeeper","version":%q}`, version.Get().Version)
		case "/api/v1/connection-info":
			_, _ = w.Write([]byte(`{"service":"TimeKeeper","instance_id":"tk_instance_0123456789abcdef0123456789abcdef","version":"` + version.Get().Version + `","api_version":"v1","capabilities":["projects.list","tasks.lifecycle"]}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("private response detail"))
		}
	}))
	defer server.Close()
	session, err := StartOrAttach(context.Background(), Config{CoreURL: server.URL}, &fakeLauncher{})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(context.Background())
	_, err = session.Client().Projects(context.Background())
	if err == nil {
		t.Fatal("expected the project request to fail")
	}
	if strings.Contains(err.Error(), "private response detail") {
		t.Fatalf("error exposed response body: %v", err)
	}
}

func TestStartOrAttachRejectsNonLoopbackURL(t *testing.T) {
	launcher := &fakeLauncher{}
	_, err := StartOrAttach(context.Background(), Config{CoreURL: "http://example.com:1618"}, launcher)
	if err == nil {
		t.Fatal("expected a non-loopback Core URL to be rejected")
	}
	if launcher.startCount != 0 {
		t.Fatalf("launched child for non-loopback URL: starts=%d", launcher.startCount)
	}
}

func TestStartOrAttachDoesNotLaunchWhenOccupiedEndpointIsMalformed(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("not an HTTP response\r\n\r\n"))
	}()

	cfg := validLaunchConfig(t, "http://"+listener.Addr().String())
	launcher := &fakeLauncher{}
	_, err = StartOrAttach(context.Background(), cfg, launcher)
	if launcher.startCount != 0 {
		t.Fatalf("launched %d child processes while a non-HTTP listener occupied the Core address", launcher.startCount)
	}
	if !errors.Is(err, ErrIncompatibleCore) {
		t.Fatalf("StartOrAttach() error = %v, want ErrIncompatibleCore", err)
	}
}

func TestStartOrAttachRejectsRedirectsOutsideCoreOrigin(t *testing.T) {
	for _, redirectPath := range []string{"/health", "/api/v1/version"} {
		t.Run(redirectPath, func(t *testing.T) {
			var redirectedHits atomic.Int32
			redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirectedHits.Add(1)
				switch r.URL.Path {
				case "/health":
					_, _ = w.Write([]byte(`{"status":"ok"}`))
				case "/api/v1/version":
					_, _ = fmt.Fprintf(w, `{"service":"TimeKeeper","version":%q}`, version.Get().Version)
				default:
					http.NotFound(w, r)
				}
			}))
			defer redirected.Close()

			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == redirectPath {
					http.Redirect(w, r, redirected.URL+r.URL.Path, http.StatusFound)
					return
				}
				switch r.URL.Path {
				case "/health":
					_, _ = w.Write([]byte(`{"status":"ok"}`))
				case "/api/v1/version":
					_, _ = fmt.Fprintf(w, `{"service":"TimeKeeper","version":%q}`, version.Get().Version)
				default:
					http.NotFound(w, r)
				}
			}))
			defer core.Close()

			launcher := &fakeLauncher{}
			_, err := StartOrAttach(context.Background(), Config{CoreURL: core.URL}, launcher)
			if !errors.Is(err, ErrIncompatibleCore) {
				t.Fatalf("StartOrAttach() error = %v, want ErrIncompatibleCore", err)
			}
			if hits := redirectedHits.Load(); hits != 0 {
				t.Fatalf("followed %d redirect requests outside the configured Core origin", hits)
			}
			if launcher.startCount != 0 {
				t.Fatalf("launched %d child processes for a redirecting listener", launcher.startCount)
			}
		})
	}
}

func TestStartOrAttachRequiresExplicitAbsolutePaths(t *testing.T) {
	addr := reserveLoopbackAddress(t)
	coreURL := "http://" + addr
	base := validLaunchConfig(t, coreURL)
	tests := []struct {
		name string
		edit func(*Config)
	}{
		{name: "missing executable", edit: func(cfg *Config) { cfg.CoreExecutable = "" }},
		{name: "relative executable", edit: func(cfg *Config) { cfg.CoreExecutable = "timekeeper.exe" }},
		{name: "missing database", edit: func(cfg *Config) { cfg.DatabasePath = "" }},
		{name: "relative database", edit: func(cfg *Config) { cfg.DatabasePath = "data.db" }},
		{name: "missing dashboard", edit: func(cfg *Config) { cfg.DashboardPath = "" }},
		{name: "relative dashboard", edit: func(cfg *Config) { cfg.DashboardPath = "web/index.html" }},
		{name: "absent dashboard asset", edit: func(cfg *Config) { cfg.DashboardPath = filepath.Join(t.TempDir(), "index.html") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.edit(&cfg)
			launcher := &fakeLauncher{}
			_, err := StartOrAttach(context.Background(), cfg, launcher)
			if err == nil {
				t.Fatal("expected missing or invalid launch path to fail")
			}
			if launcher.startCount != 0 {
				t.Fatalf("launched child despite invalid config: starts=%d", launcher.startCount)
			}
		})
	}
}

func TestCoreClientProjectsRejectsMalformedEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if r.URL.Path == "/api/v1/version" {
			_, _ = fmt.Fprintf(w, `{"service":"TimeKeeper","version":%q}`, version.Get().Version)
			return
		}
		if r.URL.Path == "/api/v1/connection-info" {
			_, _ = w.Write([]byte(`{"service":"TimeKeeper","instance_id":"tk_instance_0123456789abcdef0123456789abcdef","version":"` + version.Get().Version + `","api_version":"v1","capabilities":["projects.list","tasks.lifecycle"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":"not-an-array"}`))
	}))
	defer server.Close()
	session, err := StartOrAttach(context.Background(), Config{CoreURL: server.URL}, &fakeLauncher{})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(context.Background())
	if _, err := session.Client().Projects(context.Background()); err == nil {
		t.Fatal("expected malformed projects response to fail")
	}
}

func validLaunchConfig(t *testing.T, coreURL string) Config {
	t.Helper()
	root := t.TempDir()
	dashboard := filepath.Join(root, "web", "index.html")
	if err := os.MkdirAll(filepath.Dir(dashboard), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"index.html":     "<html>desktop</html>",
		"timekeeper.css": "body{}",
		"timekeeper.js":  "void 0;",
	} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(dashboard), name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		CoreURL:         coreURL,
		CoreExecutable:  executable,
		DatabasePath:    filepath.Join(root, "db", "core.db"),
		DashboardPath:   dashboard,
		StartupTimeout:  300 * time.Millisecond,
		ShutdownTimeout: 100 * time.Millisecond,
	}
}

func reserveLoopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

type fakeLauncher struct {
	startCount int
	executable string
	args       []string
	process    *fakeProcess
	start      func(string, []string) (ManagedProcess, error)
}

func (f *fakeLauncher) Start(_ context.Context, executable string, args []string) (ManagedProcess, error) {
	f.startCount++
	f.executable = executable
	f.args = append([]string(nil), args...)
	if f.start != nil {
		return f.start(executable, args)
	}
	if f.process == nil {
		f.process = newFakeProcess(nil)
	}
	return f.process, nil
}

type fakeProcess struct {
	mu          sync.Mutex
	done        chan struct{}
	waitErr     error
	diagnostic  string
	closeN      int
	killN       int
	ignoreClose bool
	onClose     func() error
	onKill      func() error
}

func (p *fakeProcess) StartupDiagnostics() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.diagnostic
}

func newFakeProcess(onClose func() error) *fakeProcess {
	return &fakeProcess{done: make(chan struct{}), onClose: onClose}
}

func (p *fakeProcess) Wait() error {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

func (p *fakeProcess) CloseInput() error {
	p.mu.Lock()
	p.closeN++
	fn := p.onClose
	p.mu.Unlock()
	if fn != nil {
		if err := fn(); err != nil {
			return err
		}
	}
	if !p.ignoreClose {
		p.finish(nil)
	}
	return nil
}

func (p *fakeProcess) Kill() error {
	p.mu.Lock()
	p.killN++
	fn := p.onKill
	p.mu.Unlock()
	if fn != nil {
		if err := fn(); err != nil {
			return err
		}
	}
	p.finish(errors.New("process killed"))
	return nil
}

func (p *fakeProcess) finish(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.done:
		return
	default:
		p.waitErr = err
		close(p.done)
	}
}

func TestStartOrAttachUsesLoopbackFallbackWhenFriendlyEndpointIsUnavailable(t *testing.T) {
	primaryAddr := reserveLoopbackAddress(t)
	_, primaryPort, err := net.SplitHostPort(primaryAddr)
	if err != nil {
		t.Fatal(err)
	}
	primaryURL := "http://timekeeper.local:" + primaryPort
	fallback := httptest.NewServer(coreHandler())
	defer fallback.Close()

	launcher := &fakeLauncher{}
	session, err := StartOrAttach(context.Background(), Config{
		CoreURL:     primaryURL,
		FallbackURL: fallback.URL,
	}, launcher)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(context.Background())
	if session.Owned() || launcher.startCount != 0 {
		t.Fatalf("fallback attach should not launch a child: owned=%v launches=%d", session.Owned(), launcher.startCount)
	}
	if session.client.baseURL != fallback.URL {
		t.Fatalf("attached base URL=%q, want fallback %q", session.client.baseURL, fallback.URL)
	}
}

func TestStartOrAttachAttachesToFriendlyLocalPrimaryThroughLoopback(t *testing.T) {
	var observedHost, observedRemote string
	core := coreHandler()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedHost = r.Host
		observedRemote, _, _ = net.SplitHostPort(r.RemoteAddr)
		core.ServeHTTP(w, r)
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	primaryURL := "http://timekeeper.local:" + parsed.Port()

	launcher := &fakeLauncher{}
	session, err := StartOrAttach(context.Background(), Config{CoreURL: primaryURL}, launcher)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(context.Background())
	if session.Owned() || launcher.startCount != 0 {
		t.Fatalf("friendly primary attach should not launch a child: owned=%v launches=%d", session.Owned(), launcher.startCount)
	}
	if session.client.baseURL != primaryURL {
		t.Fatalf("attached base URL=%q, want preferred primary %q", session.client.baseURL, primaryURL)
	}
	if observedHost != "timekeeper.local:"+parsed.Port() || observedRemote != "127.0.0.1" {
		t.Fatalf("friendly local request host=%q remote=%q", observedHost, observedRemote)
	}
}

func TestStartOrAttachRejectsRemoteURLDespiteLocalFallback(t *testing.T) {
	launcher := &fakeLauncher{}
	_, err := StartOrAttach(context.Background(), Config{
		CoreURL:     "https://tk.example.com",
		FallbackURL: DefaultLocalFallbackURL,
	}, launcher)
	if err == nil {
		t.Fatal("remote profile must not be redirected to the local fallback")
	}
	if launcher.startCount != 0 {
		t.Fatalf("remote URL launched a local child: starts=%d", launcher.startCount)
	}
}

func TestDefaultLocalConnectionEndpoints(t *testing.T) {
	if DefaultLocalPrimaryURL != "http://timekeeper.local" {
		t.Fatalf("default local primary=%q", DefaultLocalPrimaryURL)
	}
	if DefaultLocalFallbackURL != "http://127.0.0.1:1618" {
		t.Fatalf("default local fallback=%q", DefaultLocalFallbackURL)
	}
	baseURL, addr, err := parseCoreURL(DefaultLocalPrimaryURL)
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != DefaultLocalPrimaryURL || addr != "127.0.0.1:80" {
		t.Fatalf("default local endpoint resolved to base=%q dial=%q", baseURL, addr)
	}
}

func TestProbeConnectionReportsSelectedEndpointAndCompatibilityMetadata(t *testing.T) {
	server := httptest.NewServer(coreHandler())
	defer server.Close()

	diagnostic, err := ProbeConnection(context.Background(), Config{CoreURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.ClientSurface != "desktop" || diagnostic.SelectedEndpoint != server.URL || diagnostic.UsedFallback {
		t.Fatalf("connection selection=%+v", diagnostic)
	}
	if diagnostic.ConnectionInfo.APIVersion != "v1" || diagnostic.ConnectionInfo.InstanceID == "" {
		t.Fatalf("connection metadata=%+v", diagnostic.ConnectionInfo)
	}
}

func TestProbeConnectionReportsNumericFallbackWhenFriendlyPrimaryIsUnavailable(t *testing.T) {
	primaryAddr := reserveLoopbackAddress(t)
	_, primaryPort, err := net.SplitHostPort(primaryAddr)
	if err != nil {
		t.Fatal(err)
	}
	fallback := httptest.NewServer(coreHandler())
	defer fallback.Close()

	diagnostic, err := ProbeConnection(context.Background(), Config{
		CoreURL:     "http://timekeeper.local:" + primaryPort,
		FallbackURL: fallback.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.SelectedEndpoint != fallback.URL || !diagnostic.UsedFallback {
		t.Fatalf("fallback connection selection=%+v", diagnostic)
	}
}

func TestStartOrAttachLaunchesChildOnNumericLocalFallback(t *testing.T) {
	primaryAddr := reserveLoopbackAddress(t)
	_, primaryPort, err := net.SplitHostPort(primaryAddr)
	if err != nil {
		t.Fatal(err)
	}
	fallbackAddr := reserveLoopbackAddress(t)
	fallbackURL := "http://" + fallbackAddr
	cfg := validLaunchConfig(t, "http://timekeeper.local:"+primaryPort)
	cfg.FallbackURL = fallbackURL

	var server *http.Server
	process := newFakeProcess(func() error {
		if server == nil {
			return nil
		}
		return server.Close()
	})
	launcher := &fakeLauncher{process: process}
	launcher.start = func(_ string, args []string) (ManagedProcess, error) {
		assertArg(t, args, "-addr", fallbackAddr)
		var startErr error
		server, startErr = serveFakeCore(fallbackAddr, process)
		return process, startErr
	}

	session, err := StartOrAttach(context.Background(), cfg, launcher)
	if err != nil {
		t.Fatal(err)
	}
	if !session.Owned() || launcher.startCount != 1 {
		t.Fatalf("fallback launch state: owned=%v launches=%d", session.Owned(), launcher.startCount)
	}
	if session.client.baseURL != fallbackURL {
		t.Fatalf("child client base URL=%q, want fallback %q", session.client.baseURL, fallbackURL)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("close fallback child: %v", err)
	}
}

func TestStartOrAttachDoesNotHideIncompatibleFriendlyPrimaryWithFallback(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/version":
			_, _ = w.Write([]byte(`{"service":"TimeKeeper","version":"99.0.0"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer primary.Close()
	parsed, err := url.Parse(primary.URL)
	if err != nil {
		t.Fatal(err)
	}
	primaryURL := "http://timekeeper.local:" + parsed.Port()

	var fallbackHits atomic.Int32
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits.Add(1)
		coreHandler().ServeHTTP(w, r)
	}))
	defer fallback.Close()
	launcher := &fakeLauncher{}
	_, err = StartOrAttach(context.Background(), Config{CoreURL: primaryURL, FallbackURL: fallback.URL}, launcher)
	if !errors.Is(err, ErrIncompatibleCore) {
		t.Fatalf("StartOrAttach error=%v, want ErrIncompatibleCore", err)
	}
	if hits := fallbackHits.Load(); hits != 0 {
		t.Fatalf("incompatible primary was hidden by %d fallback requests", hits)
	}
	if launcher.startCount != 0 {
		t.Fatalf("incompatible primary started %d child processes", launcher.startCount)
	}
}

func TestStartOrAttachRejectsNonLoopbackFallback(t *testing.T) {
	primaryAddr := reserveLoopbackAddress(t)
	_, primaryPort, err := net.SplitHostPort(primaryAddr)
	if err != nil {
		t.Fatal(err)
	}
	launcher := &fakeLauncher{}
	_, err = StartOrAttach(context.Background(), Config{
		CoreURL:     "http://timekeeper.local:" + primaryPort,
		FallbackURL: "http://example.com:1618",
	}, launcher)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("StartOrAttach error=%v, want ErrNotConfigured", err)
	}
	if launcher.startCount != 0 {
		t.Fatalf("invalid fallback launched %d child processes", launcher.startCount)
	}
}
