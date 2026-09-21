// Package storeclient is the public half of the TimeKeeper Store
// protocol.
//
// It is deliberately thin and honest: every method submits a request
// and returns what the Store decided. Nothing here decides whether an
// action is permitted, because TimeKeeper is open source and a local
// build can be modified — the Store is the only authority on
// entitlements, and the only thing this client may verify locally is a
// cryptographic signature.
//
// No Store secret, no private key and no entitlement logic lives in
// this package. The only key it may hold is the public verification
// key the Store publishes at /v1/meta/public-key.
package storeclient

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is a placeholder. Point TIMEKEEPER_STORE_URL at the
// real gateway before shipping; the local application works fine with
// the Store unreachable.
const DefaultBaseURL = "https://api.timekeeper.example"

// APIError is a structured Store refusal.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("store: HTTP %d %s", e.Status, e.Code)
	}
	return fmt.Sprintf("store: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

// Client talks to the Store through the versioned /v1 API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	State      *State
}

// New builds a Store client rooted at baseURL (for example
// https://api.timekeeper.example). State is loaded from stateDir when
// one is supplied.
func New(baseURL string, stateDir string) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid store base URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Host != "127.0.0.1" && !strings.HasPrefix(parsed.Host, "127.0.0.1:") && parsed.Host != "localhost" && !strings.HasPrefix(parsed.Host, "localhost:") {
		return nil, fmt.Errorf("refusing a non-HTTPS store URL: %s", baseURL)
	}

	state := &State{BaseURL: strings.TrimSuffix(baseURL, "/")}
	if stateDir != "" {
		loaded, err := LoadState(stateDir)
		if err != nil {
			return nil, err
		}
		if loaded != nil {
			state = loaded
		}
		state.dir = stateDir
	}
	if state.BaseURL == "" {
		state.BaseURL = strings.TrimSuffix(baseURL, "/")
	}
	state.ensureInstallationID()

	return &Client{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		State:      state,
	}, nil
}

/* ------------------------------------------------------------------ *
 * Response types
 * ------------------------------------------------------------------ */

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	SessionID    int    `json:"session_id"`
}

type Account struct {
	PublicID    string `json:"public_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type Device struct {
	InstallationID string `json:"installation_id"`
	Label          string `json:"label,omitempty"`
	HasPublicKey   bool   `json:"has_public_key"`
	CreatedAt      string `json:"created_at"`
	LastSeenAt     string `json:"last_seen_at,omitempty"`
	RevokedAt      string `json:"revoked_at,omitempty"`
}

type Entitlement struct {
	EntitlementID string `json:"entitlement_id"`
	ProductID     string `json:"product_id"`
	Name          string `json:"name,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Source        string `json:"source"`
	GrantedAt     string `json:"granted_at"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	Active        bool   `json:"active"`
}

type Extension struct {
	ProductID     string   `json:"product_id"`
	Name          string   `json:"name"`
	Summary       string   `json:"summary,omitempty"`
	Kind          string   `json:"kind"`
	PriceMode     string   `json:"price_mode"`
	PriceCents    int      `json:"price_cents"`
	Currency      string   `json:"currency"`
	LatestVersion string   `json:"latest_version,omitempty"`
	MinAppVersion string   `json:"min_app_version,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
	Available     bool     `json:"available"`
}

type Catalogue struct {
	StoreAPI   string      `json:"store_api"`
	Extensions []Extension `json:"extensions"`
}

type ExtensionDetail struct {
	Extension   Extension    `json:"extension"`
	Versions    []Version    `json:"versions"`
	Entitlement *Entitlement `json:"entitlement,omitempty"`
}

type Version struct {
	Version       string `json:"version"`
	Channel       string `json:"channel"`
	MinAppVersion string `json:"min_app_version,omitempty"`
	Notes         string `json:"notes,omitempty"`
	PublishedAt   string `json:"published_at,omitempty"`
}

type ClaimResult struct {
	ProductID   string       `json:"product_id"`
	Entitlement *Entitlement `json:"entitlement,omitempty"`
	AlreadyOwned bool        `json:"already_owned"`
}

type Manifest struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	MinAppVersion string   `json:"min_app_version,omitempty"`
	Runtime       string   `json:"runtime,omitempty"`
	Entrypoint    string   `json:"entrypoint,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
}

type Download struct {
	TicketID string `json:"ticket_id"`
	URL      string `json:"url"`
	ExpiresAt string `json:"expires_at"`
	ExpiresIn int    `json:"expires_in,omitempty"`
}

type InstallResult struct {
	ProductID string   `json:"product_id"`
	Version   string   `json:"version"`
	Manifest  Manifest `json:"manifest"`
	SHA256    string   `json:"sha256"`
	SizeBytes int64    `json:"size_bytes"`
	Signature string   `json:"signature"`
	SigningKeyFingerprint string `json:"signing_key_fingerprint"`
	Download  Download `json:"download"`
}

type DownloadTicket struct {
	TicketID              string `json:"ticket_id"`
	URL                   string `json:"url"`
	ExpiresAt             string `json:"expires_at"`
	ExpiresIn             int    `json:"expires_in,omitempty"`
	SHA256                string `json:"sha256"`
	SizeBytes             int64  `json:"size_bytes"`
	Signature             string `json:"signature"`
	SigningKeyFingerprint string `json:"signing_key_fingerprint"`
}

type PublicKey struct {
	Algorithm    string `json:"algorithm"`
	Format       string `json:"format"`
	PublicKey    string `json:"public_key"`
	Fingerprint  string `json:"fingerprint"`
	SignedObject string `json:"signed_object"`
}

/* ------------------------------------------------------------------ *
 * Authentication
 * ------------------------------------------------------------------ */

type authResult struct {
	Account Account `json:"account"`
	InstallationID string `json:"installation_id,omitempty"`
	Tokens  Tokens  `json:"tokens"`
}

// Register creates a Store account and logs in on success.
func (c *Client) Register(email, password, displayName string) (*Account, error) {
	body := map[string]string{
		"email":           email,
		"password":        password,
		"installation_id": c.State.InstallationID,
		"device_label":    c.State.DeviceLabel,
	}
	if displayName != "" {
		body["display_name"] = displayName
	}
	var out authResult
	if err := c.post("/v1/auth/register", body, &out, false); err != nil {
		return nil, err
	}
	c.applyAuth(out)
	return &out.Account, nil
}

// Login authenticates and stores short-lived credentials.
func (c *Client) Login(email, password string) (*Account, error) {
	var out authResult
	err := c.post("/v1/auth/login", map[string]string{
		"email":           email,
		"password":        password,
		"installation_id": c.State.InstallationID,
		"device_label":    c.State.DeviceLabel,
	}, &out, false)
	if err != nil {
		return nil, err
	}
	c.applyAuth(out)
	return &out.Account, nil
}

// Logout revokes the refresh token held locally and clears it.
func (c *Client) Logout() error {
	if c.State == nil || c.State.RefreshToken == "" {
		return nil
	}
	err := c.post("/v1/auth/logout", map[string]string{"refresh_token": c.State.RefreshToken}, nil, false)
	c.State.AccessToken = ""
	c.State.RefreshToken = ""
	c.State.AccountPublicID = ""
	c.State.Email = ""
	_ = c.State.Save()
	return err
}

func (c *Client) applyAuth(out authResult) {
	c.State.AccessToken = out.Tokens.AccessToken
	c.State.RefreshToken = out.Tokens.RefreshToken
	c.State.AccountPublicID = out.Account.PublicID
	c.State.Email = out.Account.Email
	if out.InstallationID != "" {
		c.State.InstallationID = out.InstallationID
	}
	_ = c.State.Save()
}

// IsAuthenticated reports whether a short-lived access token exists.
// It never asks the client to decide whether access is valid; that is
// the Store's job.
func (c *Client) IsAuthenticated() bool {
	return c.State != nil && c.State.AccessToken != ""
}

// refresh exchanges the refresh token for a new pair. The Store
// rotates the refresh token, so a stolen one stops working after use.
func (c *Client) refresh() error {
	if c.State == nil || c.State.RefreshToken == "" {
		return &APIError{Status: 401, Code: "unauthenticated", Message: "not logged in"}
	}
	var out struct {
		Tokens Tokens `json:"tokens"`
	}
	if err := c.post("/v1/auth/refresh", map[string]string{"refresh_token": c.State.RefreshToken}, &out, false); err != nil {
		return err
	}
	c.State.AccessToken = out.Tokens.AccessToken
	c.State.RefreshToken = out.Tokens.RefreshToken
	return c.State.Save()
}

/* ------------------------------------------------------------------ *
 * Account
 * ------------------------------------------------------------------ */

func (c *Client) Account() (*Account, error) {
	var out Account
	if err := c.get("/v1/account", &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Devices() ([]Device, error) {
	var out struct {
		Devices []Device `json:"devices"`
	}
	if err := c.get("/v1/account/devices", &out, true); err != nil {
		return nil, err
	}
	return out.Devices, nil
}

func (c *Client) Entitlements() ([]Entitlement, error) {
	var out struct {
		Entitlements []Entitlement `json:"entitlements"`
	}
	if err := c.get("/v1/account/entitlements", &out, true); err != nil {
		return nil, err
	}
	return out.Entitlements, nil
}

/* ------------------------------------------------------------------ *
 * Store
 * ------------------------------------------------------------------ */

func (c *Client) Catalogue() (*Catalogue, error) {
	var out Catalogue
	if err := c.get("/v1/store/extensions", &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Extension(productID string) (*ExtensionDetail, error) {
	var out ExtensionDetail
	path := "/v1/store/extensions/" + url.PathEscape(strings.ToLower(strings.TrimSpace(productID)))
	if err := c.get(path, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Claim(productID string) (*ClaimResult, error) {
	var out ClaimResult
	err := c.post("/v1/store/claim", map[string]string{"product_id": productID}, &out, true)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Install(productID, version string) (*InstallResult, error) {
	body := map[string]string{"product_id": productID}
	if version != "" {
		body["version"] = version
	}
	var out InstallResult
	if err := c.post("/v1/store/install", body, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DownloadRequest(productID, version string) (*DownloadTicket, error) {
	body := map[string]string{"product_id": productID}
	if version != "" {
		body["version"] = version
	}
	var out DownloadTicket
	if err := c.post("/v1/download/request", body, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadPackage streams the package bytes for a ticket. The response
// carries the checksum and the detached ed25519 signature the caller
// must verify before the package is trusted.
func (c *Client) DownloadPackage(ticketID string) (io.ReadCloser, *DownloadHeaders, error) {
	path := "/v1/download/" + url.PathEscape(ticketID)
	resp, err := c.request(http.MethodGet, path, nil, true)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, nil, err
	}
	headers := &DownloadHeaders{
		SHA256:     resp.Header.Get("X-TK-SHA256"),
		Signature:  resp.Header.Get("X-TK-Signature"),
		Algorithm:  resp.Header.Get("X-TK-Signature-Algorithm"),
		KeyFingerprint: resp.Header.Get("X-TK-Signing-Key"),
		ProductID:  resp.Header.Get("X-TK-Product"),
	}
	return resp.Body, headers, nil
}

// DownloadHeaders are the verification values returned with a package.
type DownloadHeaders struct {
	SHA256         string
	Signature      string
	Algorithm      string
	KeyFingerprint string
	ProductID      string
}

// PublicKey fetches the Store's package verification key. This is the
// only key material the client is allowed to receive.
func (c *Client) PublicKey() (*PublicKey, error) {
	var out PublicKey
	if err := c.get("/v1/meta/public-key", &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Health() error {
	return c.get("/health", nil, false)
}

/* ------------------------------------------------------------------ *
 * Transport
 * ------------------------------------------------------------------ */

func (c *Client) get(path string, out interface{}, auth bool) error {
	resp, err := c.request(http.MethodGet, path, nil, auth)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return err
	}
	defer resp.Body.Close()
	return decodeInto(resp.Body, out)
}

func (c *Client) post(path string, body interface{}, out interface{}, auth bool) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}
	resp, err := c.request(http.MethodPost, path, payload, auth)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return err
	}
	defer resp.Body.Close()
	return decodeInto(resp.Body, out)
}

// request performs one call, retrying once after a token refresh when
// the Store reports an expired session.
func (c *Client) request(method, path string, payload []byte, auth bool) (*http.Response, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := c.do(method, path, payload, auth)
	if err == nil || !auth {
		return resp, err
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 401 || c.State == nil || c.State.RefreshToken == "" {
		return resp, err
	}
	if refreshErr := c.refresh(); refreshErr != nil {
		return resp, err
	}
	return c.do(method, path, payload, auth)
}

func (c *Client) do(method, path string, payload []byte, auth bool) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Identity and freshness headers. The gateway rejects mutating
	// requests without them.
	req.Header.Set("X-TK-Installation-Id", c.State.InstallationID)
	if method != http.MethodGet {
		req.Header.Set("X-TK-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
		nonce, nerr := randomNonce()
		if nerr != nil {
			return nil, nerr
		}
		req.Header.Set("X-TK-Nonce", nonce)
	}
	if auth {
		if c.State == nil || c.State.AccessToken == "" {
			return nil, &APIError{Status: 401, Code: "unauthenticated", Message: "run: tk store login"}
		}
		req.Header.Set("Authorization", "Bearer "+c.State.AccessToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("store request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		apiErr := &APIError{Status: resp.StatusCode}
		var parsed struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if resp.Body != nil {
			_ = json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&parsed)
		}
		apiErr.Code = parsed.Error
		apiErr.Message = parsed.Message
		if apiErr.Code == "" {
			apiErr.Code = http.StatusText(resp.StatusCode)
		}
		return resp, apiErr
	}
	return resp, nil
}

func decodeInto(r io.Reader, out interface{}) error {
	if out == nil {
		_, _ = io.Copy(io.Discard, r)
		return nil
	}
	if err := json.NewDecoder(r).Decode(out); err != nil && err != io.EOF {
		return fmt.Errorf("decode store response: %w", err)
	}
	return nil
}

func randomNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
