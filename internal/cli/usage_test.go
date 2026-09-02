// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestUsageCLIRecordsAndReadsSnapshot(t *testing.T) {
	database, err := store.Open(t.TempDir() + "/usage.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "CLI usage project"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.New(database))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"usage", "record", project.ProjectID, "session-1", "codex", "gpt-5", "1", "1200", "300", "10", "20", "2"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("record code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "session-1\t") || !strings.Contains(out.String(), "input=1200") {
		t.Fatalf("record output = %q", out.String())
	}

	out.Reset()
	errOut.Reset()
	code = Run([]string{"usage", project.ProjectID}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("summary code = %d, stderr = %s", code, errOut.String())
	}
	for _, marker := range []string{"sessions=1", "input=1200", "output=300", "cache-write=10", "cache-read=20", "messages=2"} {
		if !strings.Contains(out.String(), marker) {
			t.Fatalf("summary output %q missing %q", out.String(), marker)
		}
	}
}
