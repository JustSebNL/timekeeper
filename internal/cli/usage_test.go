// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
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

func TestUsageCLIIngestsRedactionVerifiedEnvelopeAndRejectsSensitiveData(t *testing.T) {
	database, err := store.Open(t.TempDir() + "/usage-ingest.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "CLI ingest project"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.New(database))
	defer server.Close()

	envelope := `{"redaction_status":"verified","source":"harness","coverage":"partial_session","snapshot":{"session_id":"harness-session","agent_id":"xatia","model":"gpt-5","turn_seq":1,"input_tokens":1200,"output_tokens":300,"messages":2}}`
	path := t.TempDir() + "/usage.json"
	if err := os.WriteFile(path, []byte(envelope), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run([]string{"usage", "ingest", project.ProjectID, path}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("ingest code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "source=harness") || !strings.Contains(out.String(), "coverage=partial_session") {
		t.Fatalf("ingest output = %q", out.String())
	}

	summary, err := database.ProjectUsageSummary(context.Background(), project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Sessions) != 1 || summary.Sessions[0].Source != "harness" || summary.Sessions[0].Coverage != "partial_session" {
		t.Fatalf("ingested sessions = %#v", summary.Sessions)
	}

	sensitive := `{"redaction_status":"verified","source":"harness","coverage":"partial_session","snapshot":{"session_id":"bad","agent_id":"xatia","model":"gpt-5","turn_seq":1,"input_tokens":1,"output_tokens":1},"prompt_body":"do not persist"}`
	if err := os.WriteFile(path, []byte(sensitive), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	code = Run([]string{"usage", "ingest", project.ProjectID, path}, &out, &errOut, server.URL)
	if code != 2 || !strings.Contains(errOut.String(), "sensitive field") {
		t.Fatalf("sensitive ingest code=%d stderr=%q", code, errOut.String())
	}
}
