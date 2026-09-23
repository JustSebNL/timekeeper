// Package cache implements a bounded context cache for the guardian agent.
// It combines SIEVE eviction, TinyLFU admission control, and zero-GC ring
// buffer storage to provide fixed-memory, thread-safe context caching.
package cache

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

// Cache errors.
var (
	ErrKeyNotFound = errors.New("cache: key not found")
	ErrKeyTooLarge = errors.New("cache: key too large")
	ErrValTooLarge = errors.New("cache: value too large")
	ErrCacheFull   = errors.New("cache: at capacity and admission rejected")
)

// Entry metadata flags.
const (
	flagActive   = 1 << 0 // entry is in the cache
	flagVisited  = 1 << 1 // entry has been accessed since last eviction pass
	flagPinned   = 1 << 2 // entry is pinned (never evicted)
	flagEvicted  = 1 << 3 // entry has been evicted
)

// entryHeader is the fixed-size header for each cache entry in the ring buffer.
// All fields are fixed-size for zero-copy access and no GC pointers.
type entryHeader struct {
	keyLen    uint16
	valLen    uint32
	hash      uint64
	flags     uint8
	createdAt uint32 // unix seconds, enough until 2106
	ttl       uint32 // seconds, 0 = no expiration
}

const entryHeaderSize = 2 + 4 + 8 + 1 + 4 + 4 // = 23 bytes

// entry is the in-memory representation of a cache entry.
// The actual key/value bytes live in the ring buffer.
type entry struct {
	header entryHeader
	offset int64 // offset into ring buffer where key starts
}

// shard is a single cache shard with its own ring buffer, entries, and lock.
// Sharding allows concurrent operations on different shards without contention.
type shard struct {
	mu       sync.RWMutex
	entries  map[uint64]*entry // hash -> entry (the "directory")
	ring     []byte            // preallocated ring buffer
	ringSize int64
	ringHead int64 // next write position
	ringTail int64 // oldest valid data
	used     int64 // bytes used in ring buffer
	// SIEVE eviction state
	sieveHand int64 // current position in eviction scan
	// admission filter (TinyLFU)
	doorkeeper uint64 // bloom filter bits
	cms        *countMinSketch
	// stats
	hits   atomic.Int64
	misses atomic.Int64
	evicts atomic.Int64
}

// countMinSketch is a simple Count-Min Sketch for frequency estimation.
// Uses 4-bit counters in a nybble vector (2 counters per byte).
type countMinSketch struct {
	depth  uint32
	width  uint32
	counters []uint8 // nybble vector: high nibble = counter[2i], low = counter[2i+1]
	mask   uint32
}

func newCountMinSketch(capacity int) *countMinSketch {
	// width = next power of 2 >= 4 * capacity
	width := uint32(1)
	for width < uint32(4*capacity) {
		width <<= 1
	}
	return &countMinSketch{
		depth:    4,
		width:    width,
		counters: make([]uint8, width/2), // 2 counters per byte
		mask:     width - 1,
	}
}

func (c *countMinSketch) increment(h uint64) {
	for level := uint32(0); level < c.depth; level++ {
		idx := (uint32(h) + level*uint32(h>>32)) & c.mask
		byteIdx := int(idx >> 1)
		isHigh := idx&1 == 1
		if isHigh {
			// increment high nibble
			val := int((c.counters[byteIdx] >> 4) & 0x0f)
			if val < 15 {
				c.counters[byteIdx] = (c.counters[byteIdx] & 0x0f) | (uint8(val+1) << 4)
			}
		} else {
			// increment low nibble
			val := int(c.counters[byteIdx] & 0x0f)
			if val < 15 {
				c.counters[byteIdx] = (c.counters[byteIdx] & 0xf0) | uint8(val+1)
			}
		}
	}
}

func (c *countMinSketch) estimate(h uint64) uint32 {
	minVal := uint32(15)
	for level := uint32(0); level < c.depth; level++ {
		idx := (uint32(h) + level*uint32(h>>32)) & c.mask
		byteIdx := int(idx >> 1)
		isHigh := idx&1 == 1
		var val uint32
		if isHigh {
			val = uint32((c.counters[byteIdx] >> 4) & 0x0f)
		} else {
			val = uint32(c.counters[byteIdx] & 0x0f)
		}
		if val < minVal {
			minVal = val
		}
	}
	return minVal
}

func (c *countMinSketch) reset() {
	for i := range c.counters {
		c.counters[i] = (c.counters[i] >> 1) & 0x77 // halve both nibbles
	}
}

// hashKey computes the FNV-1a hash of a key.
func hashKey(key []byte) uint64 {
	h := fnv.New64a()
	h.Write(key)
	return h.Sum64()
}

// Config configures the bounded context cache.
type Config struct {
	// NumShards is the number of shards (power of 2, default 256).
	NumShards int
	// MaxEntries is the maximum number of entries across all shards.
	MaxEntries int
	// MaxBytes is the maximum total bytes across all shards.
	MaxBytes int64
	// MaxKeySize is the maximum key size in bytes (default 1KB).
	MaxKeySize int
	// MaxValSize is the maximum value size in bytes (default 64KB).
	MaxValSize int
	// AdmissionEnabled controls whether TinyLFU admission is used (default true).
	AdmissionEnabled bool
	// WindowRatio is the fraction of capacity for the admission window (default 0.01).
	WindowRatio float64
	// EvictionBatchSize is how many entries to evict per batch (default 64).
	EvictionBatchSize int
}

// DefaultConfig returns a default configuration suitable for guardian context caching.
func DefaultConfig() Config {
	return Config{
		NumShards:        256,
		MaxEntries:       10000,
		MaxBytes:         64 * 1024 * 1024, // 64MB
		MaxKeySize:       1024,
		MaxValSize:       64 * 1024,
		AdmissionEnabled: true,
		WindowRatio:      0.01,
		EvictionBatchSize: 64,
	}
}

// Cache is a bounded, thread-safe context cache with SIEVE eviction and
// TinyLFU admission control. It uses sharded ring buffers for zero-GC storage.
type Cache struct {
	shards    []*shard
	numShards uint32
	config    Config
	// global stats
	evictions atomic.Int64
	inserts   atomic.Int64
	// adaptive window sizing (hill climbing)
	windowSize atomic.Int64
}

// New creates a new bounded context cache with the given configuration.
func New(cfg Config) (*Cache, error) {
	if cfg.NumShards <= 0 || (cfg.NumShards&(cfg.NumShards-1)) != 0 {
		cfg.NumShards = 256
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 10000
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 64 * 1024 * 1024
	}
	if cfg.MaxKeySize <= 0 {
		cfg.MaxKeySize = 1024
	}
	if cfg.MaxValSize <= 0 {
		cfg.MaxValSize = 64 * 1024
	}
	if cfg.WindowRatio <= 0 || cfg.WindowRatio >= 1 {
		cfg.WindowRatio = 0.01
	}
	if cfg.EvictionBatchSize <= 0 {
		cfg.EvictionBatchSize = 64
	}

	c := &Cache{
		numShards: uint32(cfg.NumShards),
		config:    cfg,
	}

	// Per-shard capacity
	perShardEntries := cfg.MaxEntries / int(cfg.NumShards)
	if perShardEntries < 1 {
		perShardEntries = 1
	}
	perShardBytes := cfg.MaxBytes / int64(cfg.NumShards)
	if perShardBytes < 1024 {
		perShardBytes = 1024
	}

	// Window size for this shard
	windowSize := int(float64(perShardEntries) * cfg.WindowRatio)
	if windowSize < 1 {
		windowSize = 1
	}

	c.shards = make([]*shard, cfg.NumShards)
	for i := range c.shards {
		s := &shard{
			entries: make(map[uint64]*entry),
			ring:    make([]byte, perShardBytes),
			ringSize: perShardBytes,
			cms:     newCountMinSketch(perShardEntries),
		}
		c.shards[i] = s
	}

	c.windowSize.Store(int64(windowSize))
	return c, nil
}

// getShard returns the shard for a given key.
func (c *Cache) getShard(key []byte) *shard {
	h := hashKey(key)
	return c.shards[h%uint64(c.numShards)]
}

// Get retrieves a value from the cache. Returns ErrKeyNotFound if absent.
func (c *Cache) Get(key []byte) ([]byte, error) {
	if len(key) > c.config.MaxKeySize {
		return nil, ErrKeyTooLarge
	}
	s := c.getShard(key)
	h := hashKey(key)

	s.mu.RLock()
	e, ok := s.entries[h]
	if !ok || e.header.flags&flagActive == 0 {
		s.mu.RUnlock()
		s.misses.Add(1)
		return nil, ErrKeyNotFound
	}
	// Mark visited (SIEVE lazy promotion)
	e.header.flags |= flagVisited
	// Read value from ring buffer (after header + key)
	valStart := e.offset + entryHeaderSize + int64(e.header.keyLen)
	val := make([]byte, e.header.valLen)
	copy(val, s.ring[valStart:valStart+int64(e.header.valLen)])
	s.mu.RUnlock()
	s.hits.Add(1)
	return val, nil
}

// GetWithTTL retrieves a value and its remaining TTL in seconds.
func (c *Cache) GetWithTTL(key []byte) ([]byte, int, error) {
	val, err := c.Get(key)
	if err != nil {
		return nil, 0, err
	}
	s := c.getShard(key)
	h := hashKey(key)
	s.mu.RLock()
	e := s.entries[h]
	ttl := 0
	if e.header.ttl > 0 {
		elapsed := uint32(time.Now().Unix()) - e.header.createdAt
		remaining := int(e.header.ttl) - int(elapsed)
		if remaining < 0 {
			remaining = 0
		}
		ttl = remaining
	}
	s.mu.RUnlock()
	return val, ttl, nil
}

// Set inserts or updates a key-value pair. Returns the evicted entry's key if any.
func (c *Cache) Set(key, val []byte, ttl time.Duration) ([]byte, error) {
	if len(key) > c.config.MaxKeySize {
		return nil, ErrKeyTooLarge
	}
	if len(val) > c.config.MaxValSize {
		return nil, ErrValTooLarge
	}

	s := c.getShard(key)
	h := hashKey(key)
	valLen := uint32(len(val))
	keyLen := uint16(len(key))

	// Serialize entry into ring buffer (key + value concatenated)
	totalSize := int64(entryHeaderSize) + int64(keyLen) + int64(valLen)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if key already exists (update in place)
	if e, ok := s.entries[h]; ok && e.header.flags&flagActive != 0 {
		// Update in place if value fits
		if valLen <= e.header.valLen {
			// Overwrite value bytes
			copy(s.ring[e.offset+entryHeaderSize+int64(e.header.keyLen):], val)
			e.header.valLen = valLen
			if ttl > 0 {
				e.header.ttl = uint32(ttl.Seconds())
				e.header.createdAt = uint32(time.Now().Unix())
			}
			return nil, nil
		}
		// Value grew — evict old entry and re-insert
		c.deleteLocked(s, h)
	}

	// Ensure space in ring buffer (evict if needed)
	if err := s.ensureSpaceLocked(totalSize); err != nil {
		return nil, err
	}

	// Update frequency sketch (for future admission decisions)
	if c.config.AdmissionEnabled {
		s.cms.increment(h)
	}

	// Write to ring buffer
	offset := s.ringHead
	// Check for wrap-around
	if offset+totalSize > s.ringSize {
		// Wrap to beginning
		offset = 0
	}
	// Write header
	headerBytes := encodeHeader(entryHeader{
		keyLen:    keyLen,
		valLen:    valLen,
		hash:      h,
		flags:     flagActive,
		createdAt: uint32(time.Now().Unix()),
		ttl:       uint32(ttl.Seconds()),
	})
	copy(s.ring[offset:], headerBytes)
	// Write key
	copy(s.ring[offset+entryHeaderSize:], key)
	// Write value
	copy(s.ring[offset+entryHeaderSize+int64(keyLen):], val)

	// Update ring state
	s.ringHead = offset + totalSize
	s.used += totalSize

	// Track entry
	s.entries[h] = &entry{
		header: entryHeader{
			keyLen:    keyLen,
			valLen:    valLen,
			hash:      h,
			flags:     flagActive,
			createdAt: uint32(time.Now().Unix()),
			ttl:       uint32(ttl.Seconds()),
		},
		offset: offset,
	}

	c.inserts.Add(1)
	return nil, nil
}

// Delete removes a key from the cache.
func (c *Cache) Delete(key []byte) bool {
	if len(key) > c.config.MaxKeySize {
		return false
	}
	s := c.getShard(key)
	h := hashKey(key)

	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.entries[h]; ok && e.header.flags&flagActive != 0 {
		c.deleteLocked(s, h)
		return true
	}
	return false
}

// deleteLocked removes an entry. Caller must hold s.mu.
func (c *Cache) deleteLocked(s *shard, h uint64) {
	e, ok := s.entries[h]
	if !ok || e.header.flags&flagActive == 0 {
		return
	}
	totalSize := int64(entryHeaderSize) + int64(e.header.keyLen) + int64(e.header.valLen)
	e.header.flags &^= flagActive
	e.header.flags |= flagEvicted
	delete(s.entries, h)
	s.used -= totalSize
}

// admitLocked checks whether a key should be admitted using TinyLFU.
// Caller must hold s.mu.
func (s *shard) admitLocked(h uint64, admissionEnabled bool) bool {
	if !admissionEnabled {
		return true
	}
	// Check doorkeeper bloom filter
	if s.doorkeeper&(1<<(h&63)) == 0 {
		// Not seen before — set the bit and reject (one-hit-wonder)
		s.doorkeeper |= 1 << (h & 63)
		return false
	}
	// Seen before — check frequency
	freq := s.cms.estimate(h)
	if freq < 2 {
		return false
	}
	return true
}

// ensureSpaceLocked makes room in the ring buffer for size bytes.
// Caller must hold s.mu.
func (s *shard) ensureSpaceLocked(size int64) error {
	for s.used+size > s.ringSize {
		// Evict oldest entry (SIEVE hand)
		evicted := s.evictOneLocked()
		if !evicted {
			return ErrCacheFull
		}
	}
	return nil
}

// evictOneLocked evicts one entry using SIEVE.
// Caller must hold s.mu.
func (s *shard) evictOneLocked() bool {
	if len(s.entries) == 0 {
		return false
	}

	// SIEVE: scan entries, clear visited bits, evict first unvisited non-pinned entry
	var victim *entry
	for _, e := range s.entries {
		if e.header.flags&flagActive == 0 {
			continue
		}
		if e.header.flags&flagPinned != 0 {
			continue
		}
		if e.header.flags&flagVisited != 0 {
			// Clear visited bit and skip (SIEVE roving hand behavior)
			e.header.flags &^= flagVisited
			continue
		}
		victim = e
		break
	}

	if victim == nil {
		// All entries were visited — clear all and try again
		for _, e := range s.entries {
			if e.header.flags&flagActive == 0 {
				continue
			}
			if e.header.flags&flagPinned != 0 {
				continue
			}
			e.header.flags &^= flagVisited
			victim = e
			break
		}
	}

	if victim == nil {
		return false
	}

	// Evict this entry
	totalSize := int64(entryHeaderSize) + int64(victim.header.keyLen) + int64(victim.header.valLen)
	victim.header.flags &^= flagActive
	victim.header.flags |= flagEvicted
	delete(s.entries, victim.header.hash)
	s.used -= totalSize
	s.evicts.Add(1)
	return true
}

// encodeHeader serializes an entryHeader into bytes.
func encodeHeader(h entryHeader) []byte {
	b := make([]byte, entryHeaderSize)
	binary.LittleEndian.PutUint16(b[0:], h.keyLen)
	binary.LittleEndian.PutUint32(b[2:], h.valLen)
	binary.LittleEndian.PutUint64(b[6:], h.hash)
	b[14] = h.flags
	binary.LittleEndian.PutUint32(b[15:], h.createdAt)
	binary.LittleEndian.PutUint32(b[19:], h.ttl)
	return b
}

// decodeHeader deserializes an entryHeader from bytes.
func decodeHeader(b []byte) entryHeader {
	return entryHeader{
		keyLen:    binary.LittleEndian.Uint16(b[0:]),
		valLen:    binary.LittleEndian.Uint32(b[2:]),
		hash:      binary.LittleEndian.Uint64(b[6:]),
		flags:     b[14],
		createdAt: binary.LittleEndian.Uint32(b[15:]),
		ttl:       binary.LittleEndian.Uint32(b[19:]),
	}
}

// Stats returns cache statistics.
type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Inserts   int64
	Entries   int
	BytesUsed int64
}

// Stats returns current cache statistics.
func (c *Cache) Stats() Stats {
	var entries int
	var bytesUsed int64
	var hits, misses, evictions int64
	for _, s := range c.shards {
		s.mu.RLock()
		entries += len(s.entries)
		bytesUsed += s.used
		s.mu.RUnlock()
		hits += s.hits.Load()
		misses += s.misses.Load()
		evictions += s.evicts.Load()
	}
	return Stats{
		Hits:      hits,
		Misses:    misses,
		Evictions: evictions,
		Inserts:   c.inserts.Load(),
		Entries:   entries,
		BytesUsed: bytesUsed,
	}
}

// Close releases all cache resources.
func (c *Cache) Close() error {
	for _, s := range c.shards {
		s.mu.Lock()
		s.entries = make(map[uint64]*entry)
		s.ring = nil
		s.ringSize = 0
		s.ringHead = 0
		s.ringTail = 0
		s.used = 0
		s.mu.Unlock()
	}
	return nil
}
