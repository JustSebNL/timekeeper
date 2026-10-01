package store

import (
	"github.com/JustSebNL/timekeeper/internal/model"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFASEMonthlyIdentityRetainsKeyAndSkipsUnusedMonths(t *testing.T) {
	s := openPulseStore(t)
	ctx := t.Context()
	input := model.FASEIdentityInput{Fingerprint: strings.Repeat("a", 64), Runtime: "custom", IdentityScope: "runtime-session"}
	september := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	first, err := s.ResolveFASEIdentityAt(ctx, input, september)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Z0-9]+$`).MatchString(first.AgentKey) || !regexp.MustCompile(`[A-Z]`).MatchString(first.AgentKey) || !regexp.MustCompile(`[0-9]`).MatchString(first.AgentKey) {
		t.Fatalf("key must mix letters/digits: %+v", first)
	}
	if first.FASEID != "FASE-ID."+first.AgentKey+".9.2026" {
		t.Fatalf("wrong monthly alias: %+v", first)
	}
	again, err := s.ResolveFASEIdentityAt(ctx, input, september.Add(time.Second))
	if err != nil || again.AgentKey != first.AgentKey || again.FASEID != "FASE-ID."+first.AgentKey+".10.2026" {
		t.Fatalf("rollover: %+v %v", again, err)
	}
	december, err := s.ResolveFASEIdentityAt(ctx, input, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := s.FASEProfile(ctx, first.FASEID)
	if err != nil || len(profile.PreviousIDs) != 3 {
		t.Fatalf("profile: %+v %v", profile, err)
	}
	if profile.Identity.AgentKey != december.AgentKey || profile.Identity.MaskedFingerprint != "*********aaaa" {
		t.Fatalf("profile identity: %+v", profile)
	}
	for _, id := range profile.PreviousIDs {
		if strings.Contains(id, ".11.2026") {
			t.Fatal("invented unused month")
		}
	}
}

func TestFASEConcurrentEnrollmentAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fase.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	input := model.FASEIdentityInput{Fingerprint: strings.Repeat("b", 64), Runtime: "custom", IdentityScope: "runtime-session"}
	at := time.Now()
	ids := make(chan string, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := s.ResolveFASEIdentityAt(t.Context(), input, at)
			if e != nil {
				t.Error(e)
				return
			}
			ids <- id.AgentKey
		}()
	}
	wg.Wait()
	close(ids)
	var key string
	for id := range ids {
		if key != "" && id != key {
			t.Fatal("concurrent first use split identity")
		}
		key = id
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id, err := s.ResolveFASEIdentityAt(t.Context(), input, at)
	if err != nil || id.AgentKey != key {
		t.Fatalf("restart: %+v %v", id, err)
	}
	input.Fingerprint = strings.Repeat("c", 64)
	other, err := s.ResolveFASEIdentityAt(t.Context(), input, at)
	if err != nil || other.AgentKey == key {
		t.Fatalf("different fingerprint reused key: %+v %v", other, err)
	}
}

func TestFASECacheHitDoesNotReadRegistry(t *testing.T) {
	s := openPulseStore(t)
	in := model.FASEIdentityInput{Fingerprint: strings.Repeat("d", 64), Runtime: "custom", IdentityScope: "runtime-session"}
	at := time.Now()
	first, err := s.ResolveFASEIdentityAt(t.Context(), in, at)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.db.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := s.ResolveFASEIdentityAt(t.Context(), in, at)
	if err != nil || again.AgentKey != first.AgentKey {
		t.Fatalf("identity cache queried closed registry: %+v %v", again, err)
	}
}

func TestFASEIssuedKeysCannotBeReusedOrRebound(t *testing.T) {
	s := openPulseStore(t)
	ctx := t.Context()
	at := time.Now()
	input := model.FASEIdentityInput{Fingerprint: strings.Repeat("e", 64), Runtime: "custom", IdentityScope: "runtime-session"}
	first, err := s.resolveFASEIdentityAt(ctx, input, at, func() (string, error) { return "A2034", nil })
	if err != nil {
		t.Fatal(err)
	}
	input.Fingerprint = strings.Repeat("f", 64)
	attempt := 0
	second, err := s.resolveFASEIdentityAt(ctx, input, at, func() (string, error) {
		attempt++
		if attempt == 1 {
			return "A2034", nil
		}
		return "B2034", nil
	})
	if err != nil || second.AgentKey == first.AgentKey || second.AgentKey != "B2034" {
		t.Fatalf("collision not rejected: %+v %v", second, err)
	}
	if _, err = s.db.ExecContext(ctx, `DELETE FROM fase_reserved_keys WHERE agent_key=?`, first.AgentKey); err == nil {
		t.Fatal("issued key was released")
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE fase_agents SET fingerprint=? WHERE agent_key=?`, strings.Repeat("1", 64), first.AgentKey); err == nil {
		t.Fatal("fingerprint rebound")
	}
}

func TestFASEMetadataValidationAndUTC(t *testing.T) {
	s := openPulseStore(t)
	ctx := t.Context()
	at := time.Date(2026, 10, 1, 0, 30, 0, 0, time.FixedZone("east", 2*3600))
	in := model.FASEIdentityInput{Fingerprint: strings.Repeat("1", 64), Runtime: "custom", IdentityScope: "runtime-session", Metadata: model.FASEMetadata{Provider: "reported-provider", Model: "reported-model"}}
	first, err := s.ResolveFASEIdentityAt(ctx, in, at)
	if err != nil || !strings.HasSuffix(first.FASEID, ".9.2026") || first.MetadataSource != "runtime-reported" {
		t.Fatalf("%+v %v", first, err)
	}
	in.Metadata = model.FASEMetadata{}
	again, err := s.ResolveFASEIdentityAt(ctx, in, at)
	if err != nil || again.Metadata.Model != "reported-model" {
		t.Fatalf("empty metadata erased known values: %+v %v", again, err)
	}
	in.Runtime = "different"
	if _, err = s.ResolveFASEIdentityAt(ctx, in, at); err == nil {
		t.Fatal("fingerprint runtime changed")
	}
	in.Fingerprint = "bad"
	if _, err = s.ResolveFASEIdentityAt(ctx, in, at); err == nil {
		t.Fatal("invalid fingerprint accepted")
	}
}
