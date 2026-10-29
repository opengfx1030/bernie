package api

// The cache of the answers that read the host. The page reads the same
// endpoints every few seconds, from more than one tab and from more than
// one browser, and the host does not change in between. One read per TTL
// serves every reader: three open pages cost one read of the host, not three.
//
// The mutex also makes the read single-flight: two browsers that ask at the
// same moment share one read instead of racing two.

import (
	"context"
	"sync"
	"time"
)

// The time to live of the answers of the host. They are read again after
// it: a cap or a cable that changed shows up in a few seconds.
const (
	// hostTTL is for the documents that read the hardware of the machine.
	hostTTL = 5 * time.Second
	// checkTTL is for the documents that run the checks: they cost more and
	// they change less often than the hardware does.
	checkTTL = 15 * time.Second
	// runsTTL is for the list of the runs: the page reads it every five
	// seconds and the list changes when the reader acts.
	runsTTL = 2 * time.Second
)

// cacheEntry is one cached answer and the time it was read.
type cacheEntry struct {
	val any
	at  time.Time
}

// hostCache holds the last answer of one endpoint for a short time. The key
// is the query of the request: two different queries are two different
// documents.
type hostCache struct {
	mu    sync.Mutex
	items map[string]cacheEntry
	ttl   time.Duration
}

// newHostCache returns a cache that keeps an answer for the given time.
func newHostCache(ttl time.Duration) *hostCache {
	return &hostCache{items: map[string]cacheEntry{}, ttl: ttl}
}

// get answers the document of the key. A fresh entry is returned as it is; an
// old one is read again, once, and every reader waits for that read.
func (c *hostCache) get(ctx context.Context, key string, load func(context.Context) (any, error)) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok && time.Since(e.at) < c.ttl {
		return e.val, nil
	}
	val, err := load(ctx)
	if err != nil {
		return nil, err
	}
	c.items[key] = cacheEntry{val: val, at: time.Now()}
	return val, nil
}

// drop forgets the entry of the key. A write that changes the host asks for
// it: the next reader sees the change at once.
func (c *hostCache) drop(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}
