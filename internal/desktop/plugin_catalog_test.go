package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCoreClientReadsReadOnlyPluginCatalog(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/plugins" {
			t.Fatalf("unexpected plugin catalog request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"schema_version":"timekeeper-plugin-index/v1","generated_at":"2026-10-18T20:00:00Z","plugins":[{"schema_version":"timekeeper-plugin-manifest/v1","id":"extension.example","version":"1.2.0","name":"Example extension","description":"Read-only manifest.","runtime":"wasi","entrypoint":"plugin.wasm","protocol":"timekeeper-plugin/v1","capabilities":["knowledge.search"],"requires":["files.read"],"permissions":{"files.read":true},"artifact":"extension.wasm","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","metadata":{"channel":"stable"}}]}`)
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := client.PluginCatalog(context.Background())
	if err != nil {
		t.Fatalf("PluginCatalog: %v", err)
	}
	if catalog.SchemaVersion != "timekeeper-plugin-index/v1" || len(catalog.Plugins) != 1 || catalog.Plugins[0].ID != "extension.example" || catalog.Plugins[0].Runtime != "wasi" {
		t.Fatalf("PluginCatalog() = %+v", catalog)
	}
	if fmt.Sprint(calls) != "[GET /api/v1/plugins]" {
		t.Fatalf("Core calls = %v", calls)
	}
}

func TestCoreClientPluginCatalogNormalizesEmptyIndexAndRejectsUnknownSchema(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			_, _ = fmt.Fprint(w, `{"schema_version":"timekeeper-plugin-index/v1","plugins":null}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"schema_version":"future/v2","plugins":[]}`)
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := client.PluginCatalog(context.Background())
	if err != nil || catalog.Plugins == nil || len(catalog.Plugins) != 0 {
		t.Fatalf("empty PluginCatalog() = %+v, %v", catalog, err)
	}
	if _, err := client.PluginCatalog(context.Background()); err == nil {
		t.Fatal("unsupported plugin catalog schema must be rejected")
	}
}
