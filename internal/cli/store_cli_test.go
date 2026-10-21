package cli

import (
	"strings"
	"testing"
)

func TestStorePathComponentsRejectTraversalAndUnsafeCharacters(t *testing.T) {
	valid := []string{"extension.memory-galaxy", "1.2.3", "v1_0-rc1"}
	for _, value := range valid {
		if err := validateStorePathComponent(value, "component"); err != nil {
			t.Errorf("valid component %q rejected: %v", value, err)
		}
	}
	invalid := []string{"../escape", `..\\escape`, "extension/other", "", ".", "..", "a..b", "bad space"}
	for _, value := range invalid {
		if err := validateStorePathComponent(value, "component"); err == nil {
			t.Errorf("unsafe component %q was accepted", value)
		}
	}
}

func TestStoreHelpFlagsDoNotContactStore(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		var out, errOut strings.Builder
		if code := storeCommand(args, &out, &errOut, "http://127.0.0.1:1"); code != 0 {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, errOut.String())
		}
		if !strings.Contains(out.String(), "Usage: tk store <command>") {
			t.Fatalf("args=%v output=%q", args, out.String())
		}
		if errOut.Len() != 0 {
			t.Fatalf("args=%v stderr=%q", args, errOut.String())
		}
	}
}

func TestStoreCommandsUseStoreGatewayInsteadOfCoreEndpoint(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_URL", "http://127.0.0.1:18787")
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())

	var out, errOut strings.Builder
	if code := storeStatus(&out, &errOut, "http://timekeeper.local"); code != 0 {
		t.Fatalf("store status code=%d stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "store url:       http://127.0.0.1:18787") {
		t.Fatalf("store command used the Core endpoint: %q", out.String())
	}
	if strings.Contains(out.String(), "http://timekeeper.local") {
		t.Fatalf("Core endpoint leaked into Store client output: %q", out.String())
	}
}

func TestGuardianClientKeepsExplicitCoreEndpoint(t *testing.T) {
	t.Setenv("TIMEKEEPER_STORE_URL", "http://127.0.0.1:18787")
	t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())

	client, err := newCoreClient("http://127.0.0.1:1618")
	if err != nil {
		t.Fatalf("new Core client: %v", err)
	}
	if client.BaseURL != "http://127.0.0.1:1618" {
		t.Fatalf("Core client endpoint=%q", client.BaseURL)
	}
}
