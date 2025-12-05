package ingest

import (
	"sync"
	"testing"
	"time"
)

func TestGeoCache_BasicOperations(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 100,
		TTL:      time.Hour,
	})

	// Test Get on empty cache
	result, ok := cache.Get("192.168.1.1")
	if ok {
		t.Error("expected cache miss on empty cache")
	}
	if result.Country != "" || result.Region != "" || result.City != "" {
		t.Error("expected empty result on cache miss")
	}

	// Test Set and Get
	cache.Set("192.168.1.1", GeoResult{
		Country: "US",
		Region:  "California",
		City:    "San Francisco",
	})

	result, ok = cache.Get("192.168.1.1")
	if !ok {
		t.Error("expected cache hit")
	}
	if result.Country != "US" {
		t.Errorf("expected country 'US', got '%s'", result.Country)
	}
	if result.Region != "California" {
		t.Errorf("expected region 'California', got '%s'", result.Region)
	}
	if result.City != "San Francisco" {
		t.Errorf("expected city 'San Francisco', got '%s'", result.City)
	}
}

func TestGeoCache_UpdateExisting(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 100,
		TTL:      time.Hour,
	})

	// Set initial value
	cache.Set("10.0.0.1", GeoResult{
		Country: "DE",
		Region:  "Bavaria",
		City:    "Munich",
	})

	// Update with new value
	cache.Set("10.0.0.1", GeoResult{
		Country: "DE",
		Region:  "Berlin",
		City:    "Berlin",
	})

	result, ok := cache.Get("10.0.0.1")
	if !ok {
		t.Error("expected cache hit")
	}
	if result.Region != "Berlin" {
		t.Errorf("expected updated region 'Berlin', got '%s'", result.Region)
	}
	if result.City != "Berlin" {
		t.Errorf("expected updated city 'Berlin', got '%s'", result.City)
	}
}

func TestGeoCache_TTLExpiration(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 100,
		TTL:      50 * time.Millisecond,
	})

	cache.Set("1.2.3.4", GeoResult{
		Country: "JP",
		Region:  "Tokyo",
		City:    "Tokyo",
	})

	// Should be in cache immediately
	_, ok := cache.Get("1.2.3.4")
	if !ok {
		t.Error("expected cache hit before TTL expiry")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	_, ok = cache.Get("1.2.3.4")
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestGeoCache_LRUEviction(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 3,
		TTL:      time.Hour,
	})

	// Fill the cache
	cache.Set("ip1", GeoResult{Country: "A"})
	cache.Set("ip2", GeoResult{Country: "B"})
	cache.Set("ip3", GeoResult{Country: "C"})

	// Access ip1 to make it most recently used
	cache.Get("ip1")

	// Add a new entry, should evict ip2 (least recently used)
	cache.Set("ip4", GeoResult{Country: "D"})

	// ip1 and ip3 and ip4 should be in cache
	if _, ok := cache.Get("ip1"); !ok {
		t.Error("ip1 should still be in cache")
	}
	if _, ok := cache.Get("ip3"); !ok {
		t.Error("ip3 should still be in cache")
	}
	if _, ok := cache.Get("ip4"); !ok {
		t.Error("ip4 should be in cache")
	}

	// ip2 should be evicted
	if _, ok := cache.Get("ip2"); ok {
		t.Error("ip2 should have been evicted")
	}
}

func TestGeoCache_Stats(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 100,
		TTL:      time.Hour,
	})

	// Initial stats
	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("expected size 0, got %d", stats.Size)
	}
	if stats.Capacity != 100 {
		t.Errorf("expected capacity 100, got %d", stats.Capacity)
	}
	if stats.Hits != 0 {
		t.Errorf("expected 0 hits, got %d", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("expected 0 misses, got %d", stats.Misses)
	}

	// Add some entries and generate hits/misses
	cache.Set("ip1", GeoResult{Country: "US"})
	cache.Set("ip2", GeoResult{Country: "UK"})

	// 2 hits
	cache.Get("ip1")
	cache.Get("ip2")

	// 2 misses
	cache.Get("ip3")
	cache.Get("ip4")

	stats = cache.Stats()
	if stats.Size != 2 {
		t.Errorf("expected size 2, got %d", stats.Size)
	}
	if stats.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 2 {
		t.Errorf("expected 2 misses, got %d", stats.Misses)
	}
	expectedHitRate := 0.5
	if stats.HitRate != expectedHitRate {
		t.Errorf("expected hit rate %v, got %v", expectedHitRate, stats.HitRate)
	}
}

func TestGeoCache_StatsEvictions(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 2,
		TTL:      time.Hour,
	})

	// Fill and overflow cache
	cache.Set("ip1", GeoResult{Country: "A"})
	cache.Set("ip2", GeoResult{Country: "B"})
	cache.Set("ip3", GeoResult{Country: "C"}) // Evicts ip1

	stats := cache.Stats()
	if stats.Evicts != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.Evicts)
	}

	cache.Set("ip4", GeoResult{Country: "D"}) // Evicts ip2

	stats = cache.Stats()
	if stats.Evicts != 2 {
		t.Errorf("expected 2 evictions, got %d", stats.Evicts)
	}
}

func TestGeoCache_Clear(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 100,
		TTL:      time.Hour,
	})

	cache.Set("ip1", GeoResult{Country: "US"})
	cache.Set("ip2", GeoResult{Country: "UK"})
	cache.Set("ip3", GeoResult{Country: "DE"})

	// Verify entries exist
	if stats := cache.Stats(); stats.Size != 3 {
		t.Errorf("expected size 3 before clear, got %d", stats.Size)
	}

	cache.Clear()

	// Verify cache is empty
	if stats := cache.Stats(); stats.Size != 0 {
		t.Errorf("expected size 0 after clear, got %d", stats.Size)
	}

	// Verify entries are gone
	if _, ok := cache.Get("ip1"); ok {
		t.Error("ip1 should not be in cache after clear")
	}
}

func TestGeoCache_DefaultConfig(t *testing.T) {
	cfg := DefaultGeoCacheConfig()

	if cfg.Capacity != 10000 {
		t.Errorf("expected default capacity 10000, got %d", cfg.Capacity)
	}
	if cfg.TTL != time.Hour {
		t.Errorf("expected default TTL 1 hour, got %v", cfg.TTL)
	}
}

func TestGeoCache_InvalidConfig(t *testing.T) {
	// Zero capacity should use default
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 0,
		TTL:      time.Hour,
	})
	if stats := cache.Stats(); stats.Capacity != 10000 {
		t.Errorf("expected default capacity 10000 for zero config, got %d", stats.Capacity)
	}

	// Negative capacity should use default
	cache = NewGeoCache(GeoCacheConfig{
		Capacity: -1,
		TTL:      time.Hour,
	})
	if stats := cache.Stats(); stats.Capacity != 10000 {
		t.Errorf("expected default capacity 10000 for negative config, got %d", stats.Capacity)
	}

	// Zero TTL should use default
	cache = NewGeoCache(GeoCacheConfig{
		Capacity: 100,
		TTL:      0,
	})
	// Can't directly check TTL, but cache should work
	cache.Set("ip1", GeoResult{Country: "US"})
	if _, ok := cache.Get("ip1"); !ok {
		t.Error("cache should work with default TTL")
	}
}

func TestGeoCache_Concurrent(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 1000,
		TTL:      time.Hour,
	})

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				ip := "ip" + string(rune('A'+id%26)) + string(rune('0'+j%10))
				cache.Set(ip, GeoResult{
					Country: "C" + string(rune('A'+id%26)),
					Region:  "R" + string(rune('0'+j%10)),
					City:    "City",
				})
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				ip := "ip" + string(rune('A'+id%26)) + string(rune('0'+j%10))
				cache.Get(ip)
			}
		}(i)
	}

	wg.Wait()

	// Should not panic and stats should be consistent
	stats := cache.Stats()
	if stats.Size < 0 || stats.Size > 1000 {
		t.Errorf("unexpected cache size after concurrent operations: %d", stats.Size)
	}
}

func TestGeoCache_HitRateCalculation(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 100,
		TTL:      time.Hour,
	})

	// Empty cache - hit rate should be 0
	stats := cache.Stats()
	if stats.HitRate != 0 {
		t.Errorf("expected hit rate 0 with no operations, got %v", stats.HitRate)
	}

	cache.Set("ip1", GeoResult{Country: "US"})

	// 4 hits, 1 miss = 80% hit rate
	cache.Get("ip1")
	cache.Get("ip1")
	cache.Get("ip1")
	cache.Get("ip1")
	cache.Get("ip2") // miss

	stats = cache.Stats()
	expectedRate := 0.8
	if stats.HitRate != expectedRate {
		t.Errorf("expected hit rate %v, got %v", expectedRate, stats.HitRate)
	}
}

func TestGeoCache_EmptyResult(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 100,
		TTL:      time.Hour,
	})

	// Store empty result (valid for unknown IPs)
	cache.Set("0.0.0.0", GeoResult{
		Country: "",
		Region:  "",
		City:    "",
	})

	result, ok := cache.Get("0.0.0.0")
	if !ok {
		t.Error("expected cache hit for empty result")
	}
	if result.Country != "" || result.Region != "" || result.City != "" {
		t.Error("expected empty result to be preserved")
	}
}

func TestGeoCache_EmptyIP(t *testing.T) {
	cache := NewGeoCache(GeoCacheConfig{
		Capacity: 100,
		TTL:      time.Hour,
	})

	// Empty IP should not be stored
	cache.Set("", GeoResult{Country: "US"})

	if stats := cache.Stats(); stats.Size != 0 {
		t.Errorf("expected size 0 after setting empty IP, got %d", stats.Size)
	}

	// Empty IP should return false without counting as a miss
	result, ok := cache.Get("")
	if ok {
		t.Error("expected cache miss for empty IP")
	}
	if result.Country != "" {
		t.Error("expected empty result for empty IP")
	}
}
