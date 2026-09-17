package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestHostsStatusNeedsBothNames(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hosts")
	os.WriteFile(p, []byte("127.0.0.1 timekeeper.local\n"), 0600)
	var out, errs bytes.Buffer
	if hostsStatus(p, &out, &errs) == 0 {
		t.Fatal("partial setup reported success")
	}
}
func TestHostsAddRepairsIncompleteManagedBlock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hosts")
	os.WriteFile(p, []byte("127.0.0.1 localhost\n"+hostsBegin+"\n127.0.0.1 timekeeper.local\n"+hostsEnd+"\n"), 0600)
	var out, errs bytes.Buffer
	if hostsAdd(p, &out, &errs) != 0 {
		t.Fatal(errs.String())
	}
	b, _ := os.ReadFile(p)
	if !bytes.Contains(b, []byte(hostsEntry2)) || !bytes.Contains(b, []byte("127.0.0.1 localhost")) {
		t.Fatalf("not repaired/preserved: %s", b)
	}
}
