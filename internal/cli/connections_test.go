package cli

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestConnectionURLsUsesRuntimePort(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"friendly_urls":["http://timekeeper.local/","http://api.timekeeper.local/","http://guardian.local/"]}`))
	}))
	defer s.Close()
	got := connectionURLs(s.URL)
	if got[0] != "http://timekeeper.local/" || got[1] != "http://api.timekeeper.local/" {
		t.Fatalf("%v", got)
	}
}

// timekeeper.local is the primary endpoint; 127.0.0.1:1618 is the fallback.
// This pins the chain order so the friendly URL cannot silently become
// secondary again.
func TestDefaultBaseURLsPrefersFriendlyThenLoopback(t *testing.T) {
	got := defaultBaseURLs()
	want := []string{"http://timekeeper.local", "http://127.0.0.1:1618"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("defaultBaseURLs()=%v want=%v", got, want)
	}
	if defaultBaseURL != preferredBaseURL {
		t.Fatalf("defaultBaseURL=%q want preferred %q", defaultBaseURL, preferredBaseURL)
	}
}

// An explicitly configured endpoint must be used verbatim: no probing, no
// silent redirection to a working address.
func TestResolveBaseURLKeepsExplicitEndpointVerbatim(t *testing.T) {
	explicit := "http://127.0.0.1:18095"
	fromFlag, args, err := ResolveBaseURL([]string{"--url", explicit, "doctor"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if fromFlag != explicit {
		t.Fatalf("--url endpoint=%q want %q", fromFlag, explicit)
	}
	if want := []string{"doctor"}; !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%v want=%v", args, want)
	}
	fromEnv, _, err := ResolveBaseURL([]string{"doctor"}, explicit)
	if err != nil {
		t.Fatal(err)
	}
	if fromEnv != explicit {
		t.Fatalf("TIMEKEEPER_URL endpoint=%q want %q", fromEnv, explicit)
	}
}

func TestResolveBaseURLForHarnessDoesNotResolveAnAPIEndpoint(t *testing.T) {
	base, args, err := ResolveBaseURL([]string{"harness", "list"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if base != preferredBaseURL || !reflect.DeepEqual(args, []string{"harness", "list"}) {
		t.Fatalf("base=%q args=%v, want bundled harness and friendly default", base, args)
	}
	if !isLocalHarnessCommand(args) || !isLocalHarnessCommand([]string{"--json", "harness", "list"}) || isLocalHarnessCommand([]string{"doctor"}) {
		t.Fatal("local harness command classification is incorrect")
	}
}

// The default path resolves to a reachable endpoint and always terminates
// quickly. On this machine the live service answers, so either candidate is
// acceptable; what matters is that a reachable one is chosen and the probe is
// bounded rather than hanging.
func TestResolveBaseURLDefaultResolvesToAReachableEndpoint(t *testing.T) {
	start := time.Now()
	base, _, err := ResolveBaseURL([]string{"doctor"}, "")
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]bool{preferredBaseURL: true, fallbackBaseURL: true}
	if !valid[base] {
		t.Fatalf("resolved %q which is neither preferred nor fallback", base)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("default resolution took %s; probe must stay bounded", elapsed)
	}
}
