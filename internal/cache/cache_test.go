package cache

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	stats := c.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries, got %d", stats.Entries)
	}
	if stats.BytesUsed != 0 {
		t.Errorf("expected 0 bytes used, got %d", stats.BytesUsed)
	}
}

func TestSetAndGet(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	key := []byte("test-key")
	val := []byte("test-value")

	// Set
	evicted, err := c.Set(nil, key, val, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if evicted != nil {
		t.Errorf("expected no eviction, got %v", evicted)
	}

	// Get
	got, err := c.Get(nil, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !bytes.Equal(got, val) {
		t.Errorf("expected %q, got %q", val, got)
	}
}

func TestGetNotFound(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	_, err = c.Get(nil, []byte("nonexistent"))
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	key := []byte("delete-me")
	val := []byte("value")

	if _, err := c.Set(nil, key, val, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if !c.Delete(nil, key) {
		t.Error("expected Delete to return true")
	}

	if _, err := c.Get(nil, key); err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after delete, got %v", err)
	}
}

func TestUpdateInPlace(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	key := []byte("update-key")
	val1 := []byte("short")
	val2 := []byte("longer-value")

	if _, err := c.Set(nil, key, val1, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if _, err := c.Set(nil, key, val2, 0); err != nil {
		t.Fatalf("Set update failed: %v", err)
	}

	got, err := c.Get(nil, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !bytes.Equal(got, val2) {
		t.Errorf("expected %q, got %q", val2, got)
	}
}

func TestEviction(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NumShards = 1 // single shard for predictable test
	cfg.MaxEntries = 100
	cfg.MaxBytes = 16 * 1024 // 16KB (above 1024 min per shard)
	cfg.AdmissionEnabled = false

	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	// With 16KB per shard, entries of ~47 bytes each, we can fit ~340 entries.
	// Insert 500 unique entries to force eviction.
	for i := 0; i < 500; i++ {
		key := make([]byte, 8)
		val := make([]byte, 16)
		binary.LittleEndian.PutUint64(key, uint64(i))
		binary.LittleEndian.PutUint64(val, uint64(i))
		_, err := c.Set(nil, key, val, 0)
		if err == ErrCacheFull {
			break
		}
		if err != nil {
			t.Fatalf("Set %d failed: %v", i, err)
		}
	}

	stats := c.Stats()
	if stats.Evictions == 0 {
		t.Error("expected evictions to occur")
	}
	if stats.Entries == 0 {
		t.Error("expected some entries to remain")
	}
}

func TestTTL(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	key := []byte("ttl-key")
	val := []byte("ttl-value")

	if _, err := c.Set(nil, key, val, 100*time.Millisecond); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Should be available immediately
	if _, err := c.Get(nil, key); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should still be in cache (lazy expiration) but with 0 TTL
	val2, ttl, err := c.GetWithTTL(nil, key)
	if err != nil {
		t.Fatalf("GetWithTTL failed: %v", err)
	}
	if ttl != 0 {
		t.Errorf("expected TTL 0, got %d", ttl)
	}
	if !bytes.Equal(val2, val) {
		t.Errorf("expected %q, got %q", val, val2)
	}
}

func TestGetWithTTLMissingReturnsNotFound(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	if _, _, err := c.GetWithTTL(nil, []byte("missing-ttl-key")); err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestAdmission(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AdmissionEnabled = true
	cfg.MaxEntries = 100

	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	// First insert should succeed (admission is always-on, eviction on pressure)
	key1 := []byte("admit-key-1")
	val1 := []byte("value-1")
	if _, err := c.Set(nil, key1, val1, 0); err != nil {
		t.Fatalf("First Set failed: %v", err)
	}

	// Same key again should succeed (update in place)
	if _, err := c.Set(nil, key1, val1, 0); err != nil {
		t.Fatalf("Second Set failed: %v", err)
	}

	// Different key should also succeed (no admission rejection)
	key2 := []byte("admit-key-2")
	val2 := []byte("value-2")
	if _, err := c.Set(nil, key2, val2, 0); err != nil {
		t.Fatalf("Third Set failed: %v", err)
	}

	// Both keys should be retrievable
	if _, err := c.Get(nil, key1); err != nil {
		t.Errorf("Get key1 failed: %v", err)
	}
	if _, err := c.Get(nil, key2); err != nil {
		t.Errorf("Get key2 failed: %v", err)
	}
}

func TestStats(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	key := []byte("stats-key")
	val := []byte("stats-value")

	c.Set(nil, key, val, 0)
	c.Get(nil, key)                   // hit
	c.Get(nil, []byte("nonexistent")) // miss

	stats := c.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Inserts != 1 {
		t.Errorf("expected 1 insert, got %d", stats.Inserts)
	}
	if stats.Entries != 1 {
		t.Errorf("expected 1 entry, got %d", stats.Entries)
	}
}

func TestClose(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	key := []byte("close-key")
	val := []byte("close-value")
	c.Set(nil, key, val, 0)

	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	stats := c.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after close, got %d", stats.Entries)
	}
}

func TestLargeValue(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	key := []byte("large-key")
	val := make([]byte, 10*1024) // 10KB
	for i := range val {
		val[i] = byte(i % 256)
	}

	if _, err := c.Set(nil, key, val, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := c.Get(nil, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !bytes.Equal(got, val) {
		t.Errorf("value mismatch")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	// Run concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := []byte{byte(id), byte(j)}
				val := []byte{byte(id), byte(j), 0xff}
				c.Set(nil, key, val, 0)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify some entries exist
	stats := c.Stats()
	if stats.Entries == 0 {
		t.Error("expected some entries after concurrent writes")
	}
}

func TestKeyTooLarge(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	key := make([]byte, 2000) // > 1KB default
	val := []byte("value")

	_, err = c.Set(nil, key, val, 0)
	if err != ErrKeyTooLarge {
		t.Errorf("expected ErrKeyTooLarge, got %v", err)
	}
}

func TestValTooLarge(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	key := []byte("key")
	val := make([]byte, 100*1024) // > 64KB default

	_, err = c.Set(nil, key, val, 0)
	if err != ErrValTooLarge {
		t.Errorf("expected ErrValTooLarge, got %v", err)
	}
}

func BenchmarkSet(b *testing.B) {
	c, _ := New(DefaultConfig())
	defer c.Close()

	key := []byte("bench-key")
	val := []byte("bench-value-12345")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(nil, key, val, 0)
	}
}

func BenchmarkGet(b *testing.B) {
	c, _ := New(DefaultConfig())
	defer c.Close()

	key := []byte("bench-key")
	val := []byte("bench-value-12345")
	c.Set(nil, key, val, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(nil, key)
	}
}

// SP-11567 regression test: agent namespace must be honored by GetWithTTL and Delete.
func TestAgentNamespaceGetWithTTLAndDelete(t *testing.T) {
	c, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer c.Close()

	agentID := []byte("agent-test-1")
	key := []byte("namespaced-key")
	val := []byte("namespaced-value")

	if _, err := c.Set(agentID, key, val, 60*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// GetWithTTL must find the namespaced entry with correct agentID.
	got, ttl, err := c.GetWithTTL(agentID, key)
	if err != nil {
		t.Fatalf("GetWithTTL with agentID failed: %v", err)
	}
	if ttl <= 0 {
		t.Errorf("expected positive TTL, got %d", ttl)
	}
	if !bytes.Equal(got, val) {
		t.Errorf("expected %q, got %q", val, got)
	}

	// Without agentID, the namespaced entry must NOT be visible (collision prevention).
	_, _, err = c.GetWithTTL(nil, key)
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound for raw lookup of namespaced key, got %v", err)
	}

	// Delete with agentID must succeed.
	if !c.Delete(agentID, key) {
		t.Error("expected Delete with agentID to return true")
	}

	// After delete, GetWithTTL with agentID must return missing.
	_, _, err = c.GetWithTTL(agentID, key)
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after delete, got %v", err)
	}
}
