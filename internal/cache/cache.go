// Package cache provides a simple file-based caching mechanism for application data.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Cache provides a simple file-based caching mechanism.
type Cache struct {
	cacheDir      string
	cacheDuration time.Duration
}

// New creates a new Cache instance.
func New(cacheDir string, cacheDuration time.Duration) (*Cache, error) {
	if cacheDir == "" {
		return nil, errors.New("cache directory cannot be empty")
	}
	if cacheDuration <= 0 {
		return nil, errors.New("cache duration must be positive")
	}

	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create cache directory %s: %w", cacheDir, err)
	}

	return &Cache{
			cacheDir:      cacheDir,
			cacheDuration: cacheDuration,
		},
		nil
}

// Get retrieves data from the cache. Returns (data, true, nil) if found and valid,
// (nil, false, nil) if not found or expired, or (nil, false, err) on error.
func (c *Cache) Get(key string, v interface{}) (bool, error) {
	filePath := filepath.Join(c.cacheDir, key)
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // Not found
		}
		return false, fmt.Errorf("failed to get file info for %s: %w", filePath, err)
	}

	if time.Since(fileInfo.ModTime()) > c.cacheDuration {
		_ = os.Remove(filePath) // Remove expired file, ignore error
		return false, nil       // Expired
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to read cache file %s: %w", filePath, err)
	}

	if err := json.Unmarshal(data, v); err != nil {
		return false, fmt.Errorf("failed to unmarshal cache data from %s: %w", filePath, err)
	}

	return true, nil
}

// Set stores data in the cache.
func (c *Cache) Set(key string, v interface{}) error {
	filePath := filepath.Join(c.cacheDir, key)

	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data for key %s: %w", key, err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write cache file %s: %w", filePath, err)
	}

	return nil
}

// Clear removes a specific entry from the cache.
func (c *Cache) Clear(key string) error {
	filePath := filepath.Join(c.cacheDir, key)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already gone
		}
		return fmt.Errorf("failed to remove cache file %s: %w", filePath, err)
	}
	return nil
}

// ClearAll removes all entries from the cache.
func (c *Cache) ClearAll() error {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory %s: %w", c.cacheDir, err)
	}

	for _, entry := range entries {
		if err := os.Remove(filepath.Join(c.cacheDir, entry.Name())); err != nil {
			// Log error but continue to try and remove other files
			fmt.Fprintf(os.Stderr, "Warning: failed to remove cache file %s: %v\n", entry.Name(), err)
		}
	}
	return nil
}
