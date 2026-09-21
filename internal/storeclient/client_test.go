package storeclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	dir := t.TempDir()
	client, err := New(server.URL, dir)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client, server
}

func TestClientRejectsANonHTTPSStoreURL(t *testing.T) {
	if _, err := New("http://api.timekeeper.example", t.TempDir()); err == nil {
		t.Fatal("expected a plain HTTP store URL to be refused")
	}
}

func TestClientAllowsALoopbackStoreURLForLocalDevelopment(t *testing.T) {
	if _, err := New("http://127.0.0.1:8787", t.TempDir()); err != nil {
		t.Fatalf("loopback must be allowed: %v", err)
	}
}

func TestLoginStoresCredentialsAndSendsTheInstallationID(t *testing.T) {
	var seenInstallation string
	var seenTimestamp string
	var seenNonce string

	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenInstallation = r.Header.Get("X-TK-Installation-Id")
		seenTimestamp = r.Header.Get("X-TK-Timestamp")
		seenNonce = r.Header.Get("X-TK-Nonce")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"account":         map[string]string{"public_id": "USER-1001", "email": "a@b.co"},
			"installation_id": seenInstallation,
			"tokens": map[string]interface{}{
				"access_token":  "access-1",
				"refresh_token": "refresh-1",
				"token_type":    "Bearer",
				"expires_in":    900,
			},
		})
	}))

	account, err := client.Login("a@b.co", "a-long-enough-password")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if account.PublicID != "USER-1001" {
		t.Fatalf("unexpected account: %+v", account)
	}
	if !client.IsAuthenticated() {
		t.Fatal("expected the client to be authenticated after login")
	}
	if seenInstallation != client.State.InstallationID {
		t.Fatalf("installation id not sent: %q", seenInstallation)
	}
	if seenTimestamp == "" || seenNonce == "" {
		t.Fatalf("mutating requests must carry a timestamp and nonce, got %q / %q", seenTimestamp, seenNonce)
	}

	// The session must survive a new client over the same state dir.
	reloaded, err := New(client.BaseURL, filepath.Dir(filepath.Join(client.State.dir, "x")))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.State.AccessToken != "access-1" {
		t.Fatalf("session was not persisted: %q", reloaded.State.AccessToken)
	}
}

func TestStateFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows ACLs are not visible through POSIX mode bits; Go
		// reports every writable file as 0666 there.
		t.Skip("POSIX file modes are not meaningful on Windows")
	}
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client.State.AccessToken = "access-1"
	if err := client.State.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(filepath.Join(client.State.dir, StateFileName))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("session file must not be readable by others, got %v", info.Mode().Perm())
	}
}

func TestExpiredAccessTokenIsRefreshedOnce(t *testing.T) {
	var calls []string
	var usedRefreshToken string

	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		calls = append(calls, path)
		switch path {
		case "/v1/auth/refresh":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			usedRefreshToken = body["refresh_token"]
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"tokens": map[string]string{"access_token": "access-2", "refresh_token": "refresh-2"},
			})
		case "/v1/account":
			if r.Header.Get("Authorization") != "Bearer access-2" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_token"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"public_id": "USER-1001", "email": "a@b.co"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	client.State.AccessToken = "access-1"
	client.State.RefreshToken = "refresh-1"

	account, err := client.Account()
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	if account.PublicID != "USER-1001" {
		t.Fatalf("unexpected account: %+v", account)
	}
	if usedRefreshToken != "refresh-1" {
		t.Fatalf("the stored refresh token was not used: %q", usedRefreshToken)
	}
	if client.State.RefreshToken != "refresh-2" {
		t.Fatalf("the rotated refresh token was not stored: %q", client.State.RefreshToken)
	}
	if strings.Join(calls, ",") != "/v1/account,/v1/auth/refresh,/v1/account" {
		t.Fatalf("unexpected call sequence: %v", calls)
	}
}

func TestProtectedCallWithoutCredentialsFailsLocally(t *testing.T) {
	called := false
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	client.State.AccessToken = ""

	if _, err := client.Account(); err == nil {
		t.Fatal("expected an error when no credentials are stored")
	}
	if called {
		t.Fatal("no request should be made without credentials")
	}
}

func TestStoreErrorsAreStructured(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "payment_required",
			"message": "This extension is not free.",
		})
	}))
	client.State.AccessToken = "access-1"

	_, err := client.Claim("extension.pro-thing")
	if err == nil {
		t.Fatal("expected an error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected an *APIError, got %T", err)
	}
	if apiErr.Status != 402 || apiErr.Code != "payment_required" {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestLogoutClearsLocalCredentials(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]bool{"revoked": true})
	}))
	client.State.AccessToken = "access-1"
	client.State.RefreshToken = "refresh-1"

	if err := client.Logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if client.State.AccessToken != "" || client.State.RefreshToken != "" {
		t.Fatal("credentials must be cleared on logout")
	}
	if client.IsAuthenticated() {
		t.Fatal("client must not be authenticated after logout")
	}
}
