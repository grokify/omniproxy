package backend

import (
	"crypto/tls"
	"sync"
	"time"
)

// MemoryCertCache is an in-memory certificate cache.
// Suitable for laptop and team modes where all traffic goes through one process.
type MemoryCertCache struct {
	mu       sync.RWMutex
	certs    map[string]*certEntry
	ttl      time.Duration
	metrics  Metrics
	stopChan chan struct{}
}

type certEntry struct {
	cert      *tls.Certificate
	expiresAt time.Time
}

// MemoryCertCacheConfig configures a MemoryCertCache.
type MemoryCertCacheConfig struct {
	// TTL is how long certificates are cached (default: 1 hour).
	TTL time.Duration

	// CleanupInterval is how often expired entries are removed (default: 5 minutes).
	CleanupInterval time.Duration

	// Metrics for observability (optional).
	Metrics Metrics
}

// NewMemoryCertCache creates a new in-memory certificate cache.
func NewMemoryCertCache(cfg *MemoryCertCacheConfig) *MemoryCertCache {
	if cfg == nil {
		cfg = &MemoryCertCacheConfig{}
	}

	ttl := cfg.TTL
	if ttl == 0 {
		ttl = time.Hour
	}

	cleanupInterval := cfg.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = 5 * time.Minute
	}

	metrics := cfg.Metrics
	if metrics == nil {
		metrics = NoopMetrics{}
	}

	cache := &MemoryCertCache{
		certs:    make(map[string]*certEntry),
		ttl:      ttl,
		metrics:  metrics,
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanupLoop(cleanupInterval)

	return cache
}

// Get retrieves a certificate for the given hostname.
func (c *MemoryCertCache) Get(host string) (*tls.Certificate, bool) {
	c.mu.RLock()
	entry, ok := c.certs[host]
	c.mu.RUnlock()

	if !ok {
		c.metrics.IncCacheMiss()
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.expiresAt) {
		c.Delete(host)
		c.metrics.IncCacheMiss()
		return nil, false
	}

	c.metrics.IncCacheHit()
	return entry.cert, true
}

// Set stores a certificate for the given hostname.
func (c *MemoryCertCache) Set(host string, cert *tls.Certificate) {
	c.mu.Lock()
	c.certs[host] = &certEntry{
		cert:      cert,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Delete removes a certificate from the cache.
func (c *MemoryCertCache) Delete(host string) {
	c.mu.Lock()
	delete(c.certs, host)
	c.mu.Unlock()
}

// Clear removes all certificates from the cache.
func (c *MemoryCertCache) Clear() {
	c.mu.Lock()
	c.certs = make(map[string]*certEntry)
	c.mu.Unlock()
}

// Close stops the cleanup goroutine and releases resources.
func (c *MemoryCertCache) Close() error {
	close(c.stopChan)
	return nil
}

// Size returns the number of certificates in the cache.
func (c *MemoryCertCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.certs)
}

// cleanupLoop periodically removes expired entries.
func (c *MemoryCertCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopChan:
			return
		}
	}
}

// cleanup removes expired entries.
func (c *MemoryCertCache) cleanup() {
	now := time.Now()
	expired := make([]string, 0)

	c.mu.RLock()
	for host, entry := range c.certs {
		if now.After(entry.expiresAt) {
			expired = append(expired, host)
		}
	}
	c.mu.RUnlock()

	if len(expired) > 0 {
		c.mu.Lock()
		for _, host := range expired {
			delete(c.certs, host)
		}
		c.mu.Unlock()
	}
}

// LRUCertCache is a memory cache with LRU eviction.
// Useful when memory is constrained.
type LRUCertCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*lruEntry
	head     *lruEntry
	tail     *lruEntry
	metrics  Metrics
}

type lruEntry struct {
	host string
	cert *tls.Certificate
	prev *lruEntry
	next *lruEntry
}

// LRUCertCacheConfig configures an LRUCertCache.
type LRUCertCacheConfig struct {
	// Capacity is the maximum number of certificates to cache.
	Capacity int

	// Metrics for observability (optional).
	Metrics Metrics
}

// NewLRUCertCache creates a new LRU certificate cache.
func NewLRUCertCache(cfg *LRUCertCacheConfig) *LRUCertCache {
	if cfg == nil {
		cfg = &LRUCertCacheConfig{}
	}

	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = 1000
	}

	metrics := cfg.Metrics
	if metrics == nil {
		metrics = NoopMetrics{}
	}

	return &LRUCertCache{
		capacity: capacity,
		items:    make(map[string]*lruEntry),
		metrics:  metrics,
	}
}

// Get retrieves a certificate for the given hostname.
func (c *LRUCertCache) Get(host string) (*tls.Certificate, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[host]
	if !ok {
		c.metrics.IncCacheMiss()
		return nil, false
	}

	// Move to front (most recently used)
	c.moveToFront(entry)

	c.metrics.IncCacheHit()
	return entry.cert, true
}

// Set stores a certificate for the given hostname.
func (c *LRUCertCache) Set(host string, cert *tls.Certificate) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	if entry, ok := c.items[host]; ok {
		entry.cert = cert
		c.moveToFront(entry)
		return
	}

	// Create new entry
	entry := &lruEntry{host: host, cert: cert}
	c.items[host] = entry
	c.addToFront(entry)

	// Evict if over capacity
	if len(c.items) > c.capacity {
		c.evictLRU()
	}
}

// Delete removes a certificate from the cache.
func (c *LRUCertCache) Delete(host string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.items[host]; ok {
		c.remove(entry)
		delete(c.items, host)
	}
}

// Clear removes all certificates from the cache.
func (c *LRUCertCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*lruEntry)
	c.head = nil
	c.tail = nil
}

// Close releases resources.
func (c *LRUCertCache) Close() error {
	c.Clear()
	return nil
}

// Size returns the number of certificates in the cache.
func (c *LRUCertCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *LRUCertCache) addToFront(entry *lruEntry) {
	entry.prev = nil
	entry.next = c.head

	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry

	if c.tail == nil {
		c.tail = entry
	}
}

func (c *LRUCertCache) remove(entry *lruEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		c.head = entry.next
	}

	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		c.tail = entry.prev
	}
}

func (c *LRUCertCache) moveToFront(entry *lruEntry) {
	if entry == c.head {
		return
	}
	c.remove(entry)
	c.addToFront(entry)
}

func (c *LRUCertCache) evictLRU() {
	if c.tail == nil {
		return
	}
	delete(c.items, c.tail.host)
	c.remove(c.tail)
}
