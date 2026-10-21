package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMessageAddContinuesWhenDuplicateSearchUnavailable(t *testing.T) {
	creates := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			http.NotFound(writer, request)
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/projects/P-10000/messages" {
			creates++
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"message_id":44,"ref":"M-44","project_id":"P-10000","kind":"note","body":"Four words make search happen."}`))
			return
		}
		t.Errorf("unexpected request %s %s", request.Method, request.URL.String())
		http.NotFound(writer, request)
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := Run([]string{"msg", "P-10000", "add", "Four words make search happen."}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("message add code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if creates != 1 || !strings.Contains(errOut.String(), "duplicate check unavailable") || !strings.Contains(out.String(), "M-44") {
		t.Fatalf("message add did not continue transparently: creates=%d stdout=%q stderr=%q", creates, out.String(), errOut.String())
	}
}

func TestMessageAddWarnsAboutNearDuplicateButStillPosts(t *testing.T) {
	searches, creates := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/projects/P-10000/messages/search":
			searches++
			if request.URL.Query().Get("limit") != "5" {
				t.Errorf("duplicate probe limit=%q", request.URL.Query().Get("limit"))
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"query":"queue offers must be persisted before delivery to","items":[{"message_id":42,"ref":"M-42","project_id":"P-10000","kind":"decision","body":"Queue offers must be persisted before delivery to Core."}]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/projects/P-10000/messages":
			creates++
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"message_id":43,"ref":"M-43","project_id":"P-10000","kind":"decision","body":"Queue offers must be persisted before delivery to Core again."}`))
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.String())
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"msg", "P-10000", "add", "--kind", "decision", "Queue offers must be persisted before delivery to Core again."}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("message add code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if searches != 1 || creates != 1 {
		t.Fatalf("searches=%d creates=%d", searches, creates)
	}
	if !strings.Contains(errOut.String(), "possible duplicate M-42") {
		t.Fatalf("missing duplicate warning: %q", errOut.String())
	}
	if !strings.Contains(out.String(), "M-43") {
		t.Fatalf("message was not posted: %q", out.String())
	}
}
