package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SyncMetadata tracks when the cache was last synchronized
type SyncMetadata struct {
	LastSync time.Time `json:"last_sync"`
}

// LoadSyncMetadata reads the sync metadata file from the cache directory
func LoadSyncMetadata(cacheDir string) (*SyncMetadata, error) {
	metadataPath := filepath.Join(cacheDir, "metadata.json")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sync metadata file: %w", err)
	}

	var metadata SyncMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse sync metadata file: %w", err)
	}

	return &metadata, nil
}

// SaveSyncMetadata writes the sync metadata file to the cache directory
func SaveSyncMetadata(cacheDir string, metadata *SyncMetadata) error {
	metadataPath := filepath.Join(cacheDir, "metadata.json")

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sync metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write sync metadata file: %w", err)
	}

	return nil
}

// IsValidCache checks if cached data exists and is less than 3 weeks old
func IsValidCache(cacheDir string) (bool, *SyncMetadata, error) {
	metadata, err := LoadSyncMetadata(cacheDir)
	if err != nil {
		return false, nil, err
	}

	if metadata == nil {
		return false, nil, nil
	}

	// Check if data is less than 3 weeks old
	threeWeeksAgo := time.Now().Add(-3 * 7 * 24 * time.Hour)
	if metadata.LastSync.Before(threeWeeksAgo) {
		return false, metadata, nil
	}

	return true, metadata, nil
}

// DetermineCachePath determines the appropriate cache directory path
// Priority: CSAFX_CACHE_DIR env var > XDG_CACHE_HOME/csafx > ~/.cache/csafx (Unix) or %LOCALAPPDATA%/csafx (Windows)
func DetermineCachePath() string {
	cachePath := os.Getenv("CSAFX_CACHE_DIR")
	if cachePath != "" {
		return cachePath
	}

	xdgCachePath := os.Getenv("XDG_CACHE_HOME")
	if xdgCachePath != "" {
		return filepath.Join(xdgCachePath, "csafx")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to local directory if home directory cannot be determined
		return ".cache"
	}

	if os.PathSeparator == '\\' {
		// Windows
		return filepath.Join(homeDir, "AppData", "Local", "csafx")
	} else {
		// Unix-like systems
		return filepath.Join(homeDir, ".cache", "csafx")
	}
}

// EnsureCachePath creates the cache directory if it doesn't exist and returns the path
func EnsureCachePath() (string, error) {
	cachePath := DetermineCachePath()
	if err := os.MkdirAll(cachePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory %s: %w", cachePath, err)
	}
	return cachePath, nil
}
