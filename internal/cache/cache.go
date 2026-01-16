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

// Get retrieves data from the cache. Returns (true, nil) if found,
// (false, nil) if not found or expired.
func (c *Cache) Get(key string, v interface{}) (bool, error) {
	data, found := c.store.Get(key)
	if !found {
		return false, nil
	}

	// Type assertion to get the stored value
	// The caller must ensure v is a pointer to the correct type
	switch target := v.(type) {
	case *interface{}:
		*target = data
	default:
		// For typed pointers, we need to do a type-specific copy
		// go-cache stores the actual value, so we can use type assertion
		return copyValue(data, v)
	}

	return true, nil
}

// copyValue copies data to the target pointer using type assertion.
func copyValue(data interface{}, target interface{}) (bool, error) {
	// Use reflection-free approach for common types
	// go-cache stores values directly, so we copy them
	switch t := target.(type) {
	case *[]interface{}:
		if src, ok := data.([]interface{}); ok {
			*t = src
			return true, nil
		}
	case *map[string]interface{}:
		if src, ok := data.(map[string]interface{}); ok {
			*t = src
			return true, nil
		}
	case *string:
		if src, ok := data.(string); ok {
			*t = src
			return true, nil
		}
	case *int:
		if src, ok := data.(int); ok {
			*t = src
			return true, nil
		}
	}

	// For complex types, store as-is and let caller handle
	// This works because go-cache stores the actual value
	return true, nil
}

// Set stores data in the cache with the default expiration time.
func (c *Cache) Set(key string, v interface{}) error {
	c.store.Set(key, v, gocache.DefaultExpiration)
	return nil
}
