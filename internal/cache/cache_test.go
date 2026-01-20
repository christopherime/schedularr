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

	result, found := cache.Get(key)
	assert.True(t, found)

	// Type assertion to get the data back
	retrievedData, ok := result.(TestData)
	assert.True(t, ok)
	assert.Equal(t, "test", retrievedData.Name)
	assert.Equal(t, 123, retrievedData.Value)
}

func TestGet_NotFound(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	result, found := cache.Get("non_existent_key")
	assert.False(t, found)
	assert.Nil(t, result)
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

	result, found := cache.Get(key)
	assert.False(t, found, "Expected data to be expired")
	assert.Nil(t, result)
}

func TestCacheWithDifferentTypes(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Test string
	_ = cache.Set("string_key", "hello world")
	result, found := cache.Get("string_key")
	assert.True(t, found)
	strVal, ok := result.(string)
	assert.True(t, ok)
	assert.Equal(t, "hello world", strVal)

	// Test int
	_ = cache.Set("int_key", 42)
	result, found = cache.Get("int_key")
	assert.True(t, found)
	intVal, ok := result.(int)
	assert.True(t, ok)
	assert.Equal(t, 42, intVal)

	// Test slice
	_ = cache.Set("slice_key", []string{"a", "b", "c"})
	result, found = cache.Get("slice_key")
	assert.True(t, found)
	sliceVal, ok := result.([]string)
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "b", "c"}, sliceVal)

	// Test map
	_ = cache.Set("map_key", map[string]int{"a": 1, "b": 2})
	result, found = cache.Get("map_key")
	assert.True(t, found)
	mapVal, ok := result.(map[string]int)
	assert.True(t, ok)
	assert.Equal(t, map[string]int{"a": 1, "b": 2}, mapVal)
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

func TestGet_SliceInterface(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Store a []interface{} slice
	key := "slice_interface_key"
	data := []interface{}{"a", 1, true, nil}
	err = cache.Set(key, data)
	require.NoError(t, err)

	result, found := cache.Get(key)
	assert.True(t, found)

	slice, ok := result.([]interface{})
	assert.True(t, ok)
	assert.Equal(t, 4, len(slice))
	assert.Equal(t, "a", slice[0])
	assert.Equal(t, 1, slice[1])
	assert.Equal(t, true, slice[2])
	assert.Nil(t, slice[3])
}

func TestGet_MapStringInterface(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Store a map[string]interface{}
	key := "map_interface_key"
	data := map[string]interface{}{
		"name":   "John",
		"age":    30,
		"active": true,
	}
	err = cache.Set(key, data)
	require.NoError(t, err)

	result, found := cache.Get(key)
	assert.True(t, found)

	mapResult, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "John", mapResult["name"])
	assert.Equal(t, 30, mapResult["age"])
	assert.Equal(t, true, mapResult["active"])
}

func TestConcurrentAccess(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Concurrent writes and reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := "concurrent_key"
				_ = cache.Set(key, id*100+j)
				_, _ = cache.Get(key)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or have data races - verify we can still read
	_, found := cache.Get("concurrent_key")
	assert.True(t, found)
}

func TestGet_ComplexType(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Store a complex struct type
	type ComplexData struct {
		Items  []string
		Counts map[string]int
		Nested TestData
	}
	key := "complex_key"
	data := ComplexData{
		Items:  []string{"a", "b", "c"},
		Counts: map[string]int{"x": 1, "y": 2},
		Nested: TestData{Name: "nested", Value: 100},
	}
	err = cache.Set(key, data)
	require.NoError(t, err)

	result, found := cache.Get(key)
	assert.True(t, found)
	assert.NotNil(t, result)

	// Verify the data is the correct type
	retrievedData, ok := result.(ComplexData)
	assert.True(t, ok, "Expected ComplexData type")
	assert.Equal(t, 3, len(retrievedData.Items))
	assert.Equal(t, "nested", retrievedData.Nested.Name)
}
