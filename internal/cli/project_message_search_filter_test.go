package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectMessageSearchCLIForwardsMessageFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		for key, want := range map[string]string{"q": "queue", "limit": "1", "kind": "decision", "tags": "durable,queue", "after": "2026-10-01", "before": "2026-10-31"} {
			if query.Get(key) != want {
				t.Errorf("query[%s]=%q, want %q", key, query.Get(key), want)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"query":"queue","items":[]}`))
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := Run([]string{"msg", "P-10000", "search", "queue", "--limit", "1", "--kind", "decision", "--tags", "durable,queue", "--after", "2026-10-01", "--before", "2026-10-31"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("search code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
}
