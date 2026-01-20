// Package cache provides an in-memory caching mechanism for application data.
package cache

import (
	"errors"
	"time"

	gocache "github.com/patrickmn/go-cache"
)

// Cache provides an in-memory caching mechanism with TTL support.
type Cache struct {
	store    *gocache.Cache
	duration time.Duration
}

// New creates a new Cache instance.
// The cleanupInterval determines how often expired items are purged (typically 10 minutes).
func New(cacheDuration time.Duration) (*Cache, error) {
	if cacheDuration <= 0 {
		return nil, errors.New("cache duration must be positive")
	}

	// Cleanup interval is 10% of the cache duration, minimum 1 minute
	cleanupInterval := cacheDuration / 10
	if cleanupInterval < time.Minute {
		cleanupInterval = time.Minute
	}

	return &Cache{
		store:    gocache.New(cacheDuration, cleanupInterval),
		duration: cacheDuration,
	}, nil
}

// Get retrieves data from the cache.
// Returns (value, true) if found, (nil, false) if not found or expired.
// Callers should use type assertion on the returned value:
//
//	if data, found := cache.Get("key"); found {
//	    if channels, ok := data.([]Channel); ok {
//	        // use channels
//	    }
//	}
func (c *Cache) Get(key string) (any, bool) {
	return c.store.Get(key)
}

// Set stores data in the cache with the default expiration time.
func (c *Cache) Set(key string, v any) error {
	c.store.Set(key, v, gocache.DefaultExpiration)
	return nil
}
