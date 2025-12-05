package ingest

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

// GeoResult holds the result of a geo lookup.
type GeoResult struct {
	Country string
	Region  string
	City    string
}

// geoCacheEntry stores a cached geo lookup result with its expiration time.
type geoCacheEntry struct {
	ip        string
	result    GeoResult
	expiresAt time.Time
}

// GeoCache is a thread-safe LRU cache for GeoIP lookups with TTL expiration.
type GeoCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration

	// LRU list - front is most recently used, back is least recently used
	lru   *list.List
	items map[string]*list.Element

	// Metrics counters (atomic for lock-free reads)
	hits   atomic.Uint64
	misses atomic.Uint64
	evicts atomic.Uint64
}

// GeoCacheConfig holds configuration for the GeoCache.
type GeoCacheConfig struct {
	// Capacity is the maximum number of entries in the cache.
	// Default: 10000
	Capacity int

	// TTL is how long entries remain valid.
	// Default: 1 hour
	TTL time.Duration
}

// DefaultGeoCacheConfig returns the default cache configuration.
func DefaultGeoCacheConfig() GeoCacheConfig {
	return GeoCacheConfig{
		Capacity: 10000,
		TTL:      time.Hour,
	}
}

// NewGeoCache creates a new GeoIP LRU cache with the given configuration.
func NewGeoCache(cfg GeoCacheConfig) *GeoCache {
	if cfg.Capacity <= 0 {
		cfg.Capacity = 10000
	}
	if cfg.TTL <= 0 {
		cfg.TTL = time.Hour
	}

	return &GeoCache{
		capacity: cfg.Capacity,
		ttl:      cfg.TTL,
		lru:      list.New(),
		items:    make(map[string]*list.Element, cfg.Capacity),
	}
}

// Get retrieves a cached geo result for the given IP.
// Returns the result and true if found and not expired, otherwise returns empty result and false.
func (c *GeoCache) Get(ip string) (GeoResult, bool) {
	if ip == "" {
		return GeoResult{}, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[ip]
	if !ok {
		c.misses.Add(1)
		return GeoResult{}, false
	}

	entry := elem.Value.(*geoCacheEntry)

	// Check if entry has expired
	if time.Now().After(entry.expiresAt) {
		// Remove expired entry
		c.lru.Remove(elem)
		delete(c.items, ip)
		c.misses.Add(1)
		return GeoResult{}, false
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(elem)
	c.hits.Add(1)
	return entry.result, true
}

// Set stores a geo result for the given IP in the cache.
// Empty IP strings are ignored.
func (c *GeoCache) Set(ip string, result GeoResult) {
	if ip == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if entry already exists
	if elem, ok := c.items[ip]; ok {
		// Update existing entry
		entry := elem.Value.(*geoCacheEntry)
		entry.result = result
		entry.expiresAt = time.Now().Add(c.ttl)
		c.lru.MoveToFront(elem)
		return
	}

	// Evict oldest entries if at capacity
	for c.lru.Len() >= c.capacity {
		oldest := c.lru.Back()
		if oldest != nil {
			oldEntry := oldest.Value.(*geoCacheEntry)
			delete(c.items, oldEntry.ip)
			c.lru.Remove(oldest)
			c.evicts.Add(1)
		}
	}

	// Add new entry
	entry := &geoCacheEntry{
		ip:        ip,
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
	elem := c.lru.PushFront(entry)
	c.items[ip] = elem
}

// GeoCacheStats contains cache statistics.
type GeoCacheStats struct {
	Size     int     // Current number of entries
	Capacity int     // Maximum capacity
	Hits     uint64  // Total cache hits
	Misses   uint64  // Total cache misses
	Evicts   uint64  // Total evictions due to capacity
	HitRate  float64 // Hit rate (0.0 to 1.0)
}

// Stats returns current cache statistics.
func (c *GeoCache) Stats() GeoCacheStats {
	c.mu.Lock()
	size := c.lru.Len()
	c.mu.Unlock()

	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return GeoCacheStats{
		Size:     size,
		Capacity: c.capacity,
		Hits:     hits,
		Misses:   misses,
		Evicts:   c.evicts.Load(),
		HitRate:  hitRate,
	}
}

// Clear removes all entries from the cache.
func (c *GeoCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lru.Init()
	c.items = make(map[string]*list.Element, c.capacity)
}
