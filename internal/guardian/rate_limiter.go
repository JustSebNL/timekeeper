// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package guardian

import (
	"sync"
	"time"
)

// TokenBucket represents a token bucket for one agent or session key.
type TokenBucket struct {
	capacity   float64
	refillRate float64
	tokens     float64
	lastRefill time.Time
}

// RateLimiter provides thread-safe token bucket rate limiting to prevent agent infinite loops.
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*TokenBucket
	capacity   float64
	refillRate float64
}

// NewRateLimiter creates a new RateLimiter with specified max capacity and refill tokens per second.
func NewRateLimiter(capacity float64, refillRate float64) *RateLimiter {
	if capacity <= 0 {
		capacity = 60
	}
	if refillRate <= 0 {
		refillRate = 1.0 // 1 token per second
	}
	return &RateLimiter{
		buckets:    make(map[string]*TokenBucket),
		capacity:   capacity,
		refillRate: refillRate,
	}
}

// Allow checks whether 1 token can be consumed for key.
func (r *RateLimiter) Allow(key string) bool {
	return r.AllowN(key, 1.0)
}

// AllowN checks whether n tokens can be consumed for key.
func (r *RateLimiter) AllowN(key string, n float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	b, exists := r.buckets[key]
	if !exists {
		b = &TokenBucket{
			capacity:   r.capacity,
			refillRate: r.refillRate,
			tokens:     r.capacity,
			lastRefill: now,
		}
		r.buckets[key] = b
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastRefill = now

	if b.tokens >= n {
		b.tokens -= n
		return true
	}
	return false
}

// Reset restores full token capacity for key (e.g., upon material progress renewal).
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.buckets[key]; ok {
		b.tokens = r.capacity
		b.lastRefill = time.Now()
	}
}
