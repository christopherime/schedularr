package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type TestData struct {
	Name  string
	Value int
}

func TestNew(t *testing.T) {
	tempDir := t.TempDir()
	duration := 1 * time.Hour

	cache, err := New(tempDir, duration)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if cache.cacheDir != tempDir {
		t.Errorf("Expected cacheDir %s, got %s", tempDir, cache.cacheDir)
	}
	if cache.cacheDuration != duration {
		t.Errorf("Expected cacheDuration %v, got %v", duration, cache.cacheDuration)
	}

	// Test with invalid cacheDir
	_, err = New("", duration)
	if err == nil {
		t.Error("Expected error for empty cacheDir, got nil")
	}

	// Test with invalid duration
	_, err = New(tempDir, 0)
	if err == nil {
		t.Error("Expected error for non-positive duration, got nil")
	}
}

func TestSetAndGet(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := New(tempDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "test_key"
	data := TestData{Name: "test", Value: 123}

	err = cache.Set(key, data)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	var retrievedData TestData
	found, err := cache.Get(key, &retrievedData)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if !found {
		t.Fatal("Expected data to be found, but it wasn't")
	}
	if retrievedData.Name != data.Name || retrievedData.Value != data.Value {
		t.Errorf("Retrieved data mismatch. Expected %v, got %v", data, retrievedData)
	}

	// Test non-existent key
	var nonExistentData TestData
	found, err = cache.Get("non_existent_key", &nonExistentData)
	if err != nil {
		t.Fatalf("Get() for non-existent key failed: %v", err)
	}
	if found {
		t.Error("Expected data not to be found for non-existent key, but it was")
	}
}

func TestGet_Expired(t *testing.T) {
	tempDir := t.TempDir()
	// Cache duration 1ns, so it expires immediately
	cache, err := New(tempDir, 1*time.Nanosecond)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "expired_key"
	data := TestData{Name: "expired", Value: 456}

	err = cache.Set(key, data)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Wait for cache to expire
	time.Sleep(10 * time.Millisecond) // Give it a bit more than 1ns

	var retrievedData TestData
	found, err := cache.Get(key, &retrievedData)
	if err != nil {
		t.Fatalf("Get() for expired key failed: %v", err)
	}
	if found {
		t.Error("Expected data to be expired, but it was found")
	}

	// Ensure file is removed after expiration
	if _, err := os.Stat(filepath.Join(tempDir, key)); !os.IsNotExist(err) {
		t.Errorf("Expected expired file to be removed, but it still exists or other error: %v", err)
	}
}

func TestClear(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := New(tempDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key1 := "key1"
	key2 := "key2"
	_ = cache.Set(key1, TestData{Name: "data1"})
	_ = cache.Set(key2, TestData{Name: "data2"})

	err = cache.Clear(key1)
	if err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}

	var data TestData
	found, _ := cache.Get(key1, &data)
	if found {
		t.Error("Expected key1 to be cleared, but it was found")
	}

	found, _ = cache.Get(key2, &data)
	if !found {
		t.Error("Expected key2 to still exist, but it was not found")
	}

	// Clear non-existent key
	err = cache.Clear("non_existent")
	if err != nil {
		t.Errorf("Clear() on non-existent key failed: %v", err)
	}
}

func TestClearAll(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := New(tempDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_ = cache.Set("key1", TestData{})
	_ = cache.Set("key2", TestData{})
	_ = cache.Set("key3", TestData{})

	err = cache.ClearAll()
	if err != nil {
		t.Fatalf("ClearAll() failed: %v", err)
	}

	entries, _ := os.ReadDir(tempDir)
	if len(entries) != 0 {
		t.Errorf("Expected cache directory to be empty, got %d entries", len(entries))
	}
}

func TestGet_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := New(tempDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	key := "invalid_json"
	filePath := filepath.Join(tempDir, key)
	_ = os.WriteFile(filePath, []byte("{invalid json"), 0644)

	var data TestData
	_, err = cache.Get(key, &data)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	// Ensure the error is related to unmarshaling
	if err != nil && !strings.Contains(err.Error(), "unmarshal cache data") {
		t.Errorf("Expected unmarshal error, got %v", err)
	}
}
