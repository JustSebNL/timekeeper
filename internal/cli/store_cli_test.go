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
