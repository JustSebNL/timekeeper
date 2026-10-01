package pricing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEstimateUsesOnlyConfiguredModels(t *testing.T) {
	catalog, err := NewCatalog("operator-file", "USD", map[string]Rate{
		"local-test": {InputPerMillionMicros: 2_000_000, OutputPerMillionMicros: 4_000_000},
	})
	if err != nil {
		t.Fatal(err)
	}
	priced := catalog.Estimate("local-test", 1, 2, 0, 0)
	if priced.Status != "priced" || priced.Currency != "USD" || priced.AmountMicros != 10 {
		t.Fatalf("priced estimate = %#v", priced)
	}
	unknown := catalog.Estimate("unknown", 100, 100, 0, 0)
	if unknown.Status != "unpriced" || unknown.Currency != "" || unknown.AmountMicros != 0 || unknown.Reason == "" {
		t.Fatalf("unknown estimate = %#v", unknown)
	}
}

func TestLoadFileRejectsUnknownFieldsAndAcceptsEmptyTrustedMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prices.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"timekeeper-model-prices/v1","source":"operator-file","currency":"USD","models":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Estimate("gpt-5", 1, 1, 0, 0); got.Status != "unpriced" {
		t.Fatalf("empty catalogue estimate = %#v", got)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":"timekeeper-model-prices/v1","source":"operator-file","currency":"USD","models":{},"extra":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("unknown catalogue fields must be rejected")
	}
}
