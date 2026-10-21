package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoveryRegistrationURLPrefersAdvertisedCanonicalRoute(t *testing.T) {
	var canonical string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/guardian/status" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		canonical = "http://" + r.Host + "/api/v1/guardian/recover"
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"guardian_recovery_url":"` + canonical + `"}`))
	}))
	t.Cleanup(server.Close)

	got, err := recoveryRegistrationURL(server.Client(), server.URL, "127.0.0.1:1619")
	if err != nil {
		t.Fatalf("discover canonical URL: %v", err)
	}
	if got != canonical {
		t.Fatalf("registration URL = %q, want canonical %q", got, canonical)
	}
}

func TestRecoveryRegistrationURLFallsBackWhenCanonicalNotAdvertised(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pulse_guardian_enabled":true}`))
	}))
	t.Cleanup(server.Close)

	got, err := recoveryRegistrationURL(server.Client(), server.URL, "127.0.0.1:1619")
	if err != nil {
		t.Fatalf("missing canonical URL must remain a compatibility fallback: %v", err)
	}
	if got != "http://127.0.0.1:1619/v1/recover" {
		t.Fatalf("registration URL = %q, want legacy receiver", got)
	}
}

func TestRecoveryRegistrationURLFallsBackWhenStatusUnavailable(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("status unavailable")
	})}

	got, err := recoveryRegistrationURL(client, "http://127.0.0.1:1618", "127.0.0.1:1619")
	if err == nil {
		t.Fatal("expected diagnostic error for unavailable Guardian status")
	}
	if got != "http://127.0.0.1:1619/v1/recover" {
		t.Fatalf("registration URL = %q, want legacy receiver", got)
	}
}

func TestRecoveryRegistrationURLRejectsUnsafeCanonicalAdvertisement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"guardian_recovery_url":"http://192.0.2.10:1618/api/v1/guardian/recover"}`))
	}))
	t.Cleanup(server.Close)

	got, err := recoveryRegistrationURL(server.Client(), server.URL, "127.0.0.1:1619")
	if err == nil || !strings.Contains(err.Error(), "invalid canonical") {
		t.Fatalf("expected invalid canonical diagnostic, got %v", err)
	}
	if got != "http://127.0.0.1:1619/v1/recover" {
		t.Fatalf("registration URL = %q, want compatibility fallback", got)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
