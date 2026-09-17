package cli

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func connectionURLs(base string) []string {
	fallback := []string{"http://timekeeper.local/", "http://api.timekeeper.local/"}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(base, "/") + "/api/v1/connection-info")
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()
	var data struct {
		URLs []string `json:"friendly_urls"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&data) != nil || len(data.URLs) != 2 {
		return fallback
	}
	for i, raw := range data.URLs {
		u, e := url.Parse(raw)
		expected := []string{"timekeeper.local", "api.timekeeper.local"}[i]
		if e != nil || u.Scheme != "http" || u.Hostname() != expected || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/" {
			return fallback
		}
	}
	return data.URLs
}
