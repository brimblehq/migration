package cache

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CacheEntry struct {
	Data      interface{} `json:"data"`
	ExpiresAt time.Time   `json:"expires_at"`
}

type DiskCache struct {
	mu       sync.RWMutex
	cacheDir string
}

func NewDiskCache(appName string) (*DiskCache, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, fmt.Sprintf(".%s", appName), "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &DiskCache{
		cacheDir: cacheDir,
	}, nil
}

func (c *DiskCache) getCachePath(key string) string {
	return filepath.Join(c.cacheDir, fmt.Sprintf("cache_%x.json", md5.Sum([]byte(key))))
}

func (c *DiskCache) Get(key string, value interface{}) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cachePath := c.getCachePath(key)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		os.Remove(cachePath)
		return false
	}

	if time.Now().After(entry.ExpiresAt) {
		os.Remove(cachePath)
		return false
	}

	dataBytes, err := json.Marshal(entry.Data)
	if err != nil {
		return false
	}

	if err := json.Unmarshal(dataBytes, value); err != nil {
		return false
	}

	return true
}

func (c *DiskCache) Set(key string, value interface{}, expiration time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := CacheEntry{
		Data:      value,
		ExpiresAt: time.Now().Add(expiration),
	}

	data, err := json.MarshalIndent(entry, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	cachePath := c.getCachePath(key)
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}
