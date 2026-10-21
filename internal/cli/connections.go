package cli

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"
)

func connectionURLs(base string) []string {
	fallback := []string{preferredBaseURL + "/", "http://api.timekeeper.local/"}
	client := newHTTPClient(2 * time.Second)
	resp, err := client.Get(strings.TrimRight(base, "/") + "/api/v1/connection-info")
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()
	var data struct {
		URLs []string `json:"friendly_urls"`
	}
	// The server also advertises guardian.local, so accept additional URLs;
	// the dashboard and API entries are the first two contractually.
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&data) != nil || len(data.URLs) < 2 {
		return fallback
	}
	for i, raw := range data.URLs[:2] {
		u, e := url.Parse(raw)
		expected := []string{"timekeeper.local", "api.timekeeper.local"}[i]
		if e != nil || u.Scheme != "http" || u.Hostname() != expected || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/" {
			return fallback
		}
	}
	return data.URLs[:2]
}

// resolvePreferredBaseURL picks the first reachable default endpoint.
//
// timekeeper.local is the intended point of contact, but a machine without the
// hosts entry cannot resolve it, and a stopped friendly-URL proxy answers on
// nothing. Rather than failing the command, fall back to the loopback address so
// the CLI keeps working on both kinds of machine.
//
// The probe is bounded (short timeout, /health only) and happens once per
// invocation. An explicitly configured endpoint (--url or TIMEKEEPER_URL) is
// never probed or rewritten: the operator asked for that address on purpose, so
// a failure there must surface instead of being silently redirected.
func resolvePreferredBaseURL() string {
	client := newHTTPClient(750 * time.Millisecond)
	for _, candidate := range defaultBaseURLs() {
		if ok, _, err := probeHealth(client, strings.TrimRight(candidate, "/")+"/health"); err == nil && ok {
			return candidate
		}
	}
	// Nothing answered. Return the preferred endpoint so the resulting error
	// names the address the operator is expected to use, not a fallback.
	return preferredBaseURL
}
