// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package semantic

import (
	"math"
	"testing"
)

func TestEmbedIsDeterministicAndRoundTrips(t *testing.T) {
	anchor := Embed("queue checkpoint persists state before dispatch")
	if got := Cosine(anchor, Embed("queue checkpoint persists state before dispatch")); got < 0.999 {
		t.Fatalf("same text cosine = %v", got)
	}
	if related, unrelated := Cosine(anchor, Embed("durable queue checkpoint recovery")), Cosine(anchor, Embed("blue dashboard category accent")); related <= unrelated || related <= 0 {
		t.Fatalf("similarity ordering related=%v unrelated=%v", related, unrelated)
	}
	decoded, err := Decode(Encode(anchor), Dimension)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := Cosine(anchor, decoded); math.Abs(got-1) > 0.00001 {
		t.Fatalf("round-trip cosine = %v", got)
	}
	if _, err := Decode([]byte{1, 2}, Dimension); err == nil {
		t.Fatal("expected invalid vector encoding to fail")
	}
}
