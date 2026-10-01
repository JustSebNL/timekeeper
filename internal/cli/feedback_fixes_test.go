// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
)

// Seeding writes real immutable messages into a real project and there is no
// delete subcommand, so a bare `tk msg <project> seed` must refuse to write.
func TestMsgSeedRefusesWithoutConfirmation(t *testing.T) {
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message_id":1}`))
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"msg", "P-10370", "seed"}, &out, &errOut, server.URL)
	if code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if requested {
		t.Fatal("seed issued a write without --yes")
	}
	for _, want := range []string{"would write", "refusing to write", "--yes"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout=%q missing %q", out.String(), want)
		}
	}
}

func TestMsgSeedDryRunDoesNotWrite(t *testing.T) {
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message_id":1}`))
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"msg", "P-10370", "seed", "--yes", "--dry-run"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	if requested {
		t.Fatal("dry run issued a write")
	}
	if !strings.Contains(out.String(), "no messages were written") {
		t.Fatalf("stdout=%q", out.String())
	}
}

// A bare status was the whole error an agent saw. The unknown-command path must
// point at the closest real command instead.
func TestUnknownCommandSuggestsClosestCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"serach"}, &out, &errOut, "http://127.0.0.1:1")
	if code != 2 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), `unknown command "serach"`) {
		t.Fatalf("stderr=%q", errOut.String())
	}
	if !strings.Contains(errOut.String(), `did you mean "msg <project-id> search"?`) {
		t.Fatalf("stderr=%q missing a suggestion", errOut.String())
	}
}

func TestUnknownCommandFarFromAnythingSuggestsNothing(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"zzzzzzzzzzzz"}, &out, &errOut, "http://127.0.0.1:1")
	if code != 2 {
		t.Fatalf("code=%d", code)
	}
	if strings.Contains(errOut.String(), "did you mean") {
		t.Fatalf("unrelated input produced a misleading suggestion: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "Run tk help for usage.") {
		t.Fatalf("stderr=%q", errOut.String())
	}
}
