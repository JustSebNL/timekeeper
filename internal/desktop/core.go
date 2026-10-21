// Package desktop contains the Wails adapter's transport and Core lifecycle.
// It deliberately has no Store dependency: the server remains the sole owner
// of durable state and business rules.
package desktop

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/version"
)

var (
	ErrNotConfigured    = errors.New("desktop Core launch is not configured")
	ErrIncompatibleCore = errors.New("an incompatible TimeKeeper Core occupies the configured address")
	ErrCoreUnavailable  = errors.New("TimeKeeper Core is unavailable")
	ErrStartupTimeout   = errors.New("TimeKeeper Core did not become ready before the startup deadline")
	ErrChildExited      = errors.New("TimeKeeper Core child exited before readiness")
)

const (
	requestTimeout          = 2 * time.Second
	defaultStartup          = 20 * time.Second
	defaultShutdown         = 5 * time.Second
	pollInterval            = 150 * time.Millisecond
	maxResponseSize         = 4 << 20
	DefaultLocalPrimaryURL  = "http://timekeeper.local"
	DefaultLocalFallbackURL = "http://127.0.0.1:1618"
)

// Config contains explicit desktop Core configuration. CoreURL may be the
// friendly local primary or a numeric loopback origin. FallbackURL is applied
// only to the friendly local profile; a remote profile never falls back here.
type Config struct {
	CoreURL         string
	FallbackURL     string
	CoreExecutable  string
	DatabasePath    string
	DashboardPath   string
	StartupTimeout  time.Duration
	ShutdownTimeout time.Duration
}

type Health struct {
	Status string `json:"status"`
}

type Version struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

type ConnectionInfo struct {
	Service      string   `json:"service"`
	InstanceID   string   `json:"instance_id"`
	Version      string   `json:"version"`
	Commit       string   `json:"commit"`
	BuildDate    string   `json:"build_date"`
	GoVersion    string   `json:"go_version"`
	APIVersion   string   `json:"api_version"`
	Capabilities []string `json:"capabilities"`
	FriendlyURLs []string `json:"friendly_urls"`
}

type projectsResponse struct {
	Items []model.Project `json:"items"`
}

type agentWorkStatesResponse struct {
	Items []model.AgentWorkState `json:"items"`
}

// CoreClient makes bounded, typed reads against the existing Core HTTP API.
type CoreClient struct {
	baseURL string
	http    *http.Client
}

// NewCoreClient validates and constructs a client for a numeric loopback URL.
func NewCoreClient(rawURL string) (*CoreClient, error) {
	baseURL, _, err := parseCoreURL(rawURL)
	if err != nil {
		return nil, err
	}
	return &CoreClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout:       requestTimeout,
			Transport:     localCoreTransport(),
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func parseCoreURL(rawURL string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", "", fmt.Errorf("Core URL must be an HTTP local origin")
	}
	host := u.Hostname()
	port := u.Port()
	friendly := strings.EqualFold(host, "timekeeper.local")
	ip := net.ParseIP(host)
	if friendly {
		if port == "" {
			port = "80"
		}
	} else if ip == nil || !ip.IsLoopback() || port == "" {
		return "", "", fmt.Errorf("Core URL must use timekeeper.local or a numeric loopback IP with an explicit port")
	}
	portNumber, portErr := strconv.Atoi(port)
	if portErr != nil || portNumber < 1 || portNumber > 65535 {
		return "", "", fmt.Errorf("Core URL port must be between 1 and 65535")
	}
	dialHost := host
	if friendly {
		dialHost = "127.0.0.1"
	}
	addr := net.JoinHostPort(dialHost, port)
	return "http://" + u.Host, addr, nil
}

func localCoreTransport() *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err == nil && strings.EqualFold(host, "timekeeper.local") {
			address = net.JoinHostPort("127.0.0.1", port)
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	return base
}

func isFriendlyLocalURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && strings.EqualFold(u.Scheme, "http") && strings.EqualFold(u.Hostname(), "timekeeper.local")
}

func (c *CoreClient) get(ctx context.Context, route string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+route, nil)
	if err != nil {
		return fmt.Errorf("create Core request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCoreUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Core GET %s returned HTTP %d", route, resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("Core GET %s returned malformed JSON", route)
	}
	return nil
}

func (c *CoreClient) Health(ctx context.Context) (Health, error) {
	var result Health
	err := c.get(ctx, "/health", &result)
	return result, err
}

func (c *CoreClient) Version(ctx context.Context) (Version, error) {
	var result Version
	err := c.get(ctx, "/api/v1/version", &result)
	return result, err
}

func (c *CoreClient) ConnectionInfo(ctx context.Context) (ConnectionInfo, error) {
	var result ConnectionInfo
	err := c.get(ctx, "/api/v1/connection-info", &result)
	return result, err
}

func (c *CoreClient) Projects(ctx context.Context) ([]model.Project, error) {
	var result projectsResponse
	if err := c.get(ctx, "/api/v1/projects", &result); err != nil {
		return nil, err
	}
	if result.Items == nil {
		return nil, fmt.Errorf("Core GET /api/v1/projects returned no items array")
	}
	return result.Items, nil
}

// AgentWorkStates reads the Core-derived activity snapshot without rebuilding
// agent state in the desktop client.
func (c *CoreClient) AgentWorkStates(ctx context.Context) ([]model.AgentWorkState, error) {
	var result agentWorkStatesResponse
	if err := c.get(ctx, "/api/v1/agents/work-states", &result); err != nil {
		return nil, err
	}
	if result.Items == nil {
		return nil, fmt.Errorf("Core GET /api/v1/agents/work-states returned no items array")
	}
	return result.Items, nil
}

func (c *CoreClient) probe(ctx context.Context) error {
	health, err := c.Health(ctx)
	if err != nil {
		if errors.Is(err, ErrCoreUnavailable) {
			return err
		}
		return fmt.Errorf("%w: health check failed", ErrIncompatibleCore)
	}
	if health.Status != "ok" {
		return fmt.Errorf("%w: health status is not ok", ErrIncompatibleCore)
	}
	info, err := c.Version(ctx)
	if err != nil {
		if errors.Is(err, ErrCoreUnavailable) {
			return err
		}
		return fmt.Errorf("%w: version check failed", ErrIncompatibleCore)
	}
	if info.Service != "TimeKeeper" || info.Version == "" || info.Version != version.Get().Version {
		return fmt.Errorf("%w: service/version mismatch", ErrIncompatibleCore)
	}
	connection, err := c.ConnectionInfo(ctx)
	if err != nil {
		if errors.Is(err, ErrCoreUnavailable) {
			return err
		}
		return fmt.Errorf("%w: connection metadata discovery failed", ErrIncompatibleCore)
	}
	if connection.Service != info.Service {
		return fmt.Errorf("%w: connection service does not match version metadata", ErrIncompatibleCore)
	}
	if connection.Version != info.Version {
		return fmt.Errorf("%w: connection version does not match version metadata", ErrIncompatibleCore)
	}
	if !validConnectionInstanceID(connection.InstanceID) {
		return fmt.Errorf("%w: connection metadata has a missing or invalid instance ID", ErrIncompatibleCore)
	}
	if connection.APIVersion != "v1" {
		return fmt.Errorf("%w: API version %q is unsupported; this client requires v1", ErrIncompatibleCore, connection.APIVersion)
	}
	for _, required := range []string{"projects.list", "tasks.lifecycle"} {
		if !hasConnectionCapability(connection.Capabilities, required) {
			return fmt.Errorf("%w: required capability %q is unavailable", ErrIncompatibleCore, required)
		}
	}
	return nil
}

func validConnectionInstanceID(id string) bool {
	const prefix = "tk_instance_"
	if !strings.HasPrefix(id, prefix) || len(id) != len(prefix)+32 {
		return false
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(id, prefix))
	return err == nil && len(raw) == 16
}

func hasConnectionCapability(capabilities []string, required string) bool {
	for _, capability := range capabilities {
		if capability == required {
			return true
		}
	}
	return false
}

// Session represents an attachment to an external Core or an owned child.
type Session struct {
	client          *CoreClient
	owned           bool
	child           ManagedProcess
	childDone       <-chan error
	shutdownTimeout time.Duration
}

func (s *Session) Client() *CoreClient { return s.client }
func (s *Session) Owned() bool         { return s.owned }

// StartOrAttach attaches to a compatible Core, or starts a child only when no
// listener is present and explicit child paths have been validated. The local
// friendly endpoint may use only its configured numeric-loopback fallback.
func StartOrAttach(ctx context.Context, cfg Config, launcher Launcher) (*Session, error) {
	selection, err := selectLocalCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	client := selection.client
	addr := selection.addr
	launchCfg := selection.launchConfig
	probeErr := selection.probeErr
	if probeErr == nil {
		return &Session{client: client}, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !errors.Is(probeErr, ErrCoreUnavailable) {
		return nil, probeErr
	}

	available, err := coreAddressAvailable(ctx, addr)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: could not confirm whether the configured address is available: %v", ErrIncompatibleCore, err)
	}
	if !available {
		return nil, fmt.Errorf("%w: a listener occupies the configured address but did not pass the Core probe", ErrIncompatibleCore)
	}
	if launcher == nil {
		return nil, fmt.Errorf("%w: no child launcher was provided", ErrNotConfigured)
	}
	if err := validateLaunchConfig(launchCfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	if launchCfg.StartupTimeout <= 0 {
		launchCfg.StartupTimeout = defaultStartup
	}
	if launchCfg.ShutdownTimeout <= 0 {
		launchCfg.ShutdownTimeout = defaultShutdown
	}
	args := []string{"-addr", addr, "-db", launchCfg.DatabasePath, "-ui", launchCfg.DashboardPath, "-shutdown-on-stdin-close"}
	child, err := launcher.Start(ctx, launchCfg.CoreExecutable, args)
	if err != nil {
		return nil, fmt.Errorf("start TimeKeeper Core child: %w", err)
	}
	childDone := make(chan error, 1)
	go func() { childDone <- child.Wait() }()
	session := &Session{client: client, owned: true, child: child, childDone: childDone, shutdownTimeout: launchCfg.ShutdownTimeout}

	readyCtx, cancel := context.WithTimeout(ctx, launchCfg.StartupTimeout)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		probeErr = client.probe(readyCtx)
		if probeErr == nil {
			return session, nil
		}
		if !errors.Is(probeErr, ErrCoreUnavailable) {
			_ = session.stopOwned(context.Background())
			return nil, withStartupDiagnostics(probeErr, child)
		}
		select {
		case childErr := <-childDone:
			_ = child.CloseInput()
			return nil, withStartupDiagnostics(fmt.Errorf("%w: %v", ErrChildExited, childErr), child)
		case <-readyCtx.Done():
			_ = session.stopOwned(context.Background())
			if errors.Is(readyCtx.Err(), context.DeadlineExceeded) {
				return nil, withStartupDiagnostics(ErrStartupTimeout, child)
			}
			return nil, withStartupDiagnostics(readyCtx.Err(), child)
		case <-ticker.C:
		}
	}
}

func isNumericLoopbackURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Port() == "" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}

func coreAddressAvailable(ctx context.Context, addr string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false, err
	}
	if err := listener.Close(); err != nil {
		return false, err
	}
	return true, nil
}

func validateLaunchConfig(cfg Config) error {
	if !filepath.IsAbs(cfg.CoreExecutable) {
		return fmt.Errorf("Core executable path must be absolute")
	}
	executableInfo, err := os.Stat(cfg.CoreExecutable)
	if err != nil || !executableInfo.Mode().IsRegular() {
		return fmt.Errorf("Core executable must be an existing regular file")
	}
	if !filepath.IsAbs(cfg.DatabasePath) {
		return fmt.Errorf("database path must be absolute")
	}
	if info, err := os.Stat(filepath.Dir(cfg.DatabasePath)); err != nil || !info.IsDir() {
		return fmt.Errorf("database parent directory must already exist")
	}
	if info, err := os.Stat(cfg.DatabasePath); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("database path must name a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("database path is unavailable")
	}
	if !filepath.IsAbs(cfg.DashboardPath) {
		return fmt.Errorf("dashboard path must be absolute")
	}
	for _, path := range []string{cfg.DashboardPath, filepath.Join(filepath.Dir(cfg.DashboardPath), "timekeeper.css"), filepath.Join(filepath.Dir(cfg.DashboardPath), "timekeeper.js")} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o444 == 0 {
			return fmt.Errorf("dashboard asset is missing, unreadable, or not a regular file")
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("dashboard asset is not readable")
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("dashboard asset could not be closed")
		}
	}
	return nil
}
