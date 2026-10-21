package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestHostsStatusNeedsAllManagedNames(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hosts")
	os.WriteFile(p, []byte("127.0.0.1 timekeeper.local api.timekeeper.local guardian.local\n"), 0600)
	var out, errs bytes.Buffer
	if hostsStatus(p, &out, &errs) == 0 {
		t.Fatal("missing vite.timekeeper.local reported success")
	}
	if !bytes.Contains(out.Bytes(), []byte("missing: 127.0.0.1 vite.timekeeper.local")) {
		t.Fatalf("status did not identify missing Vite alias: %s", out.String())
	}
}
func TestHostsAddRepairsIncompleteManagedBlock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hosts")
	os.WriteFile(p, []byte("127.0.0.1 localhost\n"+hostsBegin+"\n"+hostsEntry1+"\n"+hostsEntry2+"\n"+hostsEntry3+"\n"+hostsEnd+"\n"), 0600)
	var out, errs bytes.Buffer
	if hostsAdd(p, &out, &errs) != 0 {
		t.Fatal(errs.String())
	}
	b, _ := os.ReadFile(p)
	if !bytes.Contains(b, []byte(hostsEntry2)) || !bytes.Contains(b, []byte("127.0.0.1 vite.timekeeper.local")) || !bytes.Contains(b, []byte("127.0.0.1 localhost")) {
		t.Fatalf("not repaired/preserved: %s", b)
	}
	out.Reset()
	if hostsStatus(p, &out, &errs) != 0 {
		t.Fatalf("repaired hosts block is incomplete: %s", out.String())
	}
	before := append([]byte(nil), b...)
	out.Reset()
	if hostsAdd(p, &out, &errs) != 0 || !bytes.Contains(out.Bytes(), []byte("already present")) {
		t.Fatalf("complete hosts block was not recognized: %s", errs.String())
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatalf("idempotent hosts add changed the file:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestHostsRemoveRemovesTheManagedViteAliasOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hosts")
	body := "127.0.0.1 localhost\\n" + hostsBegin + "\\n" + hostsEntry1 + "\\n" + hostsEntry2 + "\\n" + hostsEntry3 + "\\n" + hostsEntry4 + "\\n" + hostsEnd + "\\n"
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if hostsRemove(p, &out, &errs) != 0 {
		t.Fatal(errs.String())
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(after, []byte("127.0.0.1 localhost")) || bytes.Contains(after, []byte("vite.timekeeper.local")) || hostsBlockExists(string(after)) {
		t.Fatalf("managed aliases were not removed safely: %s", after)
	}
}
