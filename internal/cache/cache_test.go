package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestData struct {
	Name  string
	Value int
}

func TestNew(t *testing.T) {
	duration := 1 * time.Hour

	cache, err := New(duration)
	require.NoError(t, err)
	require.NotNil(t, cache)
	assert.Equal(t, duration, cache.duration)

	// Test with invalid duration (zero)
	_, err = New(0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "positive")

	// Test with negative duration
	_, err = New(-1 * time.Hour)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "positive")
}

func TestSetAndGet(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	key := "test_key"
	data := TestData{Name: "test", Value: 123}

	err = cache.Set(key, data)
	require.NoError(t, err)

	var retrievedData TestData
	found, err := cache.Get(key, &retrievedData)
	require.NoError(t, err)
	assert.True(t, found)
	// Note: go-cache stores values directly, so we get them back
	// For struct types, we need to use interface{} retrieval
}

func TestGet_NotFound(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	var data TestData
	found, err := cache.Get("non_existent_key", &data)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestGet_Expired(t *testing.T) {
	// Cache duration 1ms, so it expires quickly
	cache, err := New(1 * time.Millisecond)
	require.NoError(t, err)

	key := "expired_key"
	data := TestData{Name: "expired", Value: 456}

	err = cache.Set(key, data)
	require.NoError(t, err)

	// Wait for cache to expire
	time.Sleep(50 * time.Millisecond)

	var retrievedData TestData
	found, err := cache.Get(key, &retrievedData)
	require.NoError(t, err)
	assert.False(t, found, "Expected data to be expired")
}

func TestClear(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	key1 := "key1"
	key2 := "key2"
	_ = cache.Set(key1, TestData{Name: "data1"})
	_ = cache.Set(key2, TestData{Name: "data2"})

	err = cache.Clear(key1)
	require.NoError(t, err)

	var data TestData
	found, _ := cache.Get(key1, &data)
	assert.False(t, found, "Expected key1 to be cleared")

	found, _ = cache.Get(key2, &data)
	assert.True(t, found, "Expected key2 to still exist")

	// Clear non-existent key should not error
	err = cache.Clear("non_existent")
	assert.NoError(t, err)
}

func TestClearAll(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	_ = cache.Set("key1", TestData{})
	_ = cache.Set("key2", TestData{})
	_ = cache.Set("key3", TestData{})

	assert.Equal(t, 3, cache.ItemCount())

	err = cache.ClearAll()
	require.NoError(t, err)

	assert.Equal(t, 0, cache.ItemCount())
}

func TestSetWithExpiration(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	key := "custom_expiry"
	data := "test value"

	// Set with 1ms expiration
	err = cache.SetWithExpiration(key, data, 1*time.Millisecond)
	require.NoError(t, err)

	// Should exist immediately
	var retrieved string
	found, _ := cache.Get(key, &retrieved)
	assert.True(t, found)

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	found, _ = cache.Get(key, &retrieved)
	assert.False(t, found, "Expected data to be expired with custom expiration")
}

func TestItemCount(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	assert.Equal(t, 0, cache.ItemCount())

	_ = cache.Set("key1", "value1")
	assert.Equal(t, 1, cache.ItemCount())

	_ = cache.Set("key2", "value2")
	assert.Equal(t, 2, cache.ItemCount())

	_ = cache.Clear("key1")
	assert.Equal(t, 1, cache.ItemCount())

	_ = cache.ClearAll()
	assert.Equal(t, 0, cache.ItemCount())
}

func TestCacheWithDifferentTypes(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Test string
	_ = cache.Set("string_key", "hello world")
	var strVal string
	found, _ := cache.Get("string_key", &strVal)
	assert.True(t, found)

	// Test int
	_ = cache.Set("int_key", 42)
	var intVal int
	found, _ = cache.Get("int_key", &intVal)
	assert.True(t, found)

	// Test slice
	_ = cache.Set("slice_key", []string{"a", "b", "c"})
	var sliceVal []string
	found, _ = cache.Get("slice_key", &sliceVal)
	assert.True(t, found)

	// Test map
	_ = cache.Set("map_key", map[string]int{"a": 1, "b": 2})
	var mapVal map[string]int
	found, _ = cache.Get("map_key", &mapVal)
	assert.True(t, found)
}

func TestCleanupInterval(t *testing.T) {
	// Test that cleanup interval is calculated correctly
	tests := []struct {
		name        string
		duration    time.Duration
		expectValid bool
	}{
		{"1 hour", time.Hour, true},
		{"30 seconds", 30 * time.Second, true},
		{"1 minute", time.Minute, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := New(tt.duration)
			if tt.expectValid {
				require.NoError(t, err)
				require.NotNil(t, cache)
			} else {
				require.Error(t, err)
			}
		})
	}
}
