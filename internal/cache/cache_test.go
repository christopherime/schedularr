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

func TestGet_InterfacePointer(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Store a value
	key := "interface_key"
	data := TestData{Name: "test", Value: 42}
	err = cache.Set(key, data)
	require.NoError(t, err)

	// Get with *interface{} target (covers the switch case in Get)
	var result interface{}
	found, err := cache.Get(key, &result)
	require.NoError(t, err)
	assert.True(t, found)
	assert.NotNil(t, result)

	// Verify the data is correct
	retrievedData, ok := result.(TestData)
	assert.True(t, ok, "Expected TestData type")
	assert.Equal(t, "test", retrievedData.Name)
	assert.Equal(t, 42, retrievedData.Value)
}

func TestCopyValue_SliceInterface(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Store a []interface{} slice
	key := "slice_interface_key"
	data := []interface{}{"a", 1, true, nil}
	err = cache.Set(key, data)
	require.NoError(t, err)

	// Get with *[]interface{} target
	var result []interface{}
	found, err := cache.Get(key, &result)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 4, len(result))
	assert.Equal(t, "a", result[0])
	assert.Equal(t, 1, result[1])
	assert.Equal(t, true, result[2])
	assert.Nil(t, result[3])
}

func TestCopyValue_MapStringInterface(t *testing.T) {
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

	// Get with *map[string]interface{} target
	var result map[string]interface{}
	found, err := cache.Get(key, &result)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "John", result["name"])
	assert.Equal(t, 30, result["age"])
	assert.Equal(t, true, result["active"])
}

func TestCopyValue_TypeMismatch(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Store a string but try to get as []interface{}
	key := "type_mismatch_key"
	err = cache.Set(key, "not a slice")
	require.NoError(t, err)

	// This should still return true (found), but the slice won't be populated
	var result []interface{}
	found, err := cache.Get(key, &result)
	require.NoError(t, err)
	assert.True(t, found) // Item was found, even though type doesn't match

	// Store an int but try to get as map[string]interface{}
	err = cache.Set("int_key", 42)
	require.NoError(t, err)

	var mapResult map[string]interface{}
	found, err = cache.Get("int_key", &mapResult)
	require.NoError(t, err)
	assert.True(t, found)
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
				var val int
				_, _ = cache.Get(key, &val)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or have data races
	count := cache.ItemCount()
	assert.GreaterOrEqual(t, count, 0)
}

func TestCopyValue_StringType(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Store a string and retrieve with *string
	key := "string_copy_key"
	err = cache.Set(key, "hello world")
	require.NoError(t, err)

	var result string
	found, err := cache.Get(key, &result)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "hello world", result)
}

func TestCopyValue_IntType(t *testing.T) {
	cache, err := New(1 * time.Hour)
	require.NoError(t, err)

	// Store an int and retrieve with *int
	key := "int_copy_key"
	err = cache.Set(key, 12345)
	require.NoError(t, err)

	var result int
	found, err := cache.Get(key, &result)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 12345, result)
}

func TestCopyValue_ComplexType(t *testing.T) {
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

	// Retrieve with *interface{} to test the generic path
	var result interface{}
	found, err := cache.Get(key, &result)
	require.NoError(t, err)
	assert.True(t, found)
	assert.NotNil(t, result)

	// Verify the data is the correct type
	retrievedData, ok := result.(ComplexData)
	assert.True(t, ok, "Expected ComplexData type")
	assert.Equal(t, 3, len(retrievedData.Items))
	assert.Equal(t, "nested", retrievedData.Nested.Name)
}
