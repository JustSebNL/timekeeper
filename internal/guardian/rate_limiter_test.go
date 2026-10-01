// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package guardian_test

import (
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/guardian"
)

func TestTokenBucketRateLimiter(t *testing.T) {
	limiter := guardian.NewRateLimiter(3, 10) // 3 tokens max capacity, 10 tokens/sec refill

	key := "agent-infinite-loop-test"

	// 1. Consume 3 tokens -> allowed
	for i := 0; i < 3; i++ {
		if !limiter.Allow(key) {
			t.Fatalf("expected token %d to be allowed", i+1)
		}
	}

	// 2. 4th token immediately -> should be denied
	if limiter.Allow(key) {
		t.Fatalf("expected 4th token to be denied due to rate limit")
	}

	// 3. Reset bucket -> should allow immediately
	limiter.Reset(key)
	if !limiter.Allow(key) {
		t.Fatalf("expected token to be allowed after reset")
	}

	// 4. Sleep 200ms -> should refill tokens
	limiter.Allow(key)
	limiter.Allow(key)
	time.Sleep(200 * time.Millisecond)
	if !limiter.Allow(key) {
		t.Fatalf("expected token to be allowed after refill")
	}
}
