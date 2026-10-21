package desktop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestCoreClientReadsProjectHistoryAndLargeVersionContent(t *testing.T) {
	largeContent := strings.Repeat("source-line\\n", (5<<20)/len("source-line\\n"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects/P-7/file-root":
			_, _ = w.Write([]byte(`{"project_id":"P-7","root_path":"D:\\\\src\\\\project"}`))
		case "/api/v1/projects/P-7/files":
			_, _ = w.Write([]byte(`{"items":[{"file_id":"F-1","project_id":"P-7","current_path":"src/main.go","file_type":"go","current_version_id":"FV-2","tracking_policy":"project_scan"}]}`))
		case "/api/v1/files/F-1/versions":
			_, _ = w.Write([]byte(`{"items":[{"version_id":"FV-1","file_id":"F-1","version_number":1,"content_hash":"sha256:a","change_kind":"file_added","snapshot_type":"full","blob_ref":"sha256/a","size_bytes":1,"status":"current"}]}`))
		case "/api/v1/files/F-1/versions/FV-1":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"version": model.FileVersion{VersionID: "FV-1", FileID: "F-1", VersionNumber: 1, ChangeKind: model.FileChangeFileAdded},
				"content": largeContent,
			}); err != nil {
				t.Errorf("encode historical version: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	root, err := client.ProjectFileRoot(context.Background(), "P-7")
	if err != nil || root.ProjectID != "P-7" || root.RootPath == "" {
		t.Fatalf("ProjectFileRoot() = %+v, %v", root, err)
	}
	files, err := client.ProjectFiles(context.Background(), "P-7")
	if err != nil || len(files) != 1 || files[0].FileID != "F-1" || files[0].CurrentPath != "src/main.go" {
		t.Fatalf("ProjectFiles() = %+v, %v", files, err)
	}
	versions, err := client.FileVersions(context.Background(), "F-1")
	if err != nil || len(versions) != 1 || versions[0].VersionID != "FV-1" {
		t.Fatalf("FileVersions() = %+v, %v", versions, err)
	}
	version, err := client.ReadFileVersion(context.Background(), "F-1", "FV-1")
	if err != nil || version.Version.VersionID != "FV-1" || version.Content != largeContent {
		t.Fatalf("ReadFileVersion() content length=%d version=%+v err=%v", len(version.Content), version.Version, err)
	}
}

func TestCoreClientConfiguresProjectFileRootAndScansExplicitly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "PUT /api/v1/projects/P-7/file-root":
			var body struct {
				RootPath string `json:"root_path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RootPath != `D:\\source\\project` {
				t.Errorf("root body = %+v, err=%v", body, err)
			}
			_, _ = w.Write([]byte(`{"project_id":"P-7","root_path":"D:\\\\source\\\\project"}`))
		case "POST /api/v1/projects/P-7/files/scan":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 0 {
				t.Errorf("scan body = %+v, err=%v", body, err)
			}
			_, _ = w.Write([]byte(`{"project_id":"P-7","root_path":"D:\\\\source\\\\project","files_discovered":2,"files_added":1,"versions_added":2,"files_skipped":3}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	root, err := client.SetProjectFileRoot(context.Background(), "P-7", `D:\\source\\project`)
	if err != nil || root.ProjectID != "P-7" || root.RootPath == "" {
		t.Fatalf("SetProjectFileRoot() = %+v, %v", root, err)
	}
	result, err := client.ScanProjectFiles(context.Background(), "P-7")
	if err != nil || result.FilesDiscovered != 2 || result.FilesAdded != 1 || result.FilesSkipped != 3 {
		t.Fatalf("ScanProjectFiles() = %+v, %v", result, err)
	}
}
