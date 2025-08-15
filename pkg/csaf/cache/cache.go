package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SyncMetadata tracks when the cache was last synchronized and its source
type SyncMetadata struct {
	LastSync  time.Time `json:"last_sync"`
	SourceURL string    `json:"source_url"`
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

// DataSetInfo represents information about a cached CSAF data set
type DataSetInfo struct {
	Name string
	Path string
	Size int64
}

// ListDataSets returns all available cached CSAF data sets with their sizes
func ListDataSets() ([]DataSetInfo, error) {
	cachePath := DetermineCachePath()

	// Check if cache directory exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return []DataSetInfo{}, nil
	}

	entries, err := os.ReadDir(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	var dataSets []DataSetInfo
	for _, entry := range entries {
		if entry.IsDir() {
			dataSetPath := filepath.Join(cachePath, entry.Name())
			size, err := calculateDirSize(dataSetPath)
			if err != nil {
				// If we can't calculate size, still include the data set with 0 size
				size = 0
			}

			dataSets = append(dataSets, DataSetInfo{
				Name: entry.Name(),
				Path: dataSetPath,
				Size: size,
			})
		}
	}

	return dataSets, nil
}

// ClearDataSet removes a specific cached CSAF data set
func ClearDataSet(dataSetName string) error {
	cachePath := DetermineCachePath()
	dataSetPath := filepath.Join(cachePath, dataSetName)

	// Check if data set exists
	if _, err := os.Stat(dataSetPath); os.IsNotExist(err) {
		return fmt.Errorf("data set '%s' does not exist", dataSetName)
	}

	// Remove the data set directory
	if err := os.RemoveAll(dataSetPath); err != nil {
		return fmt.Errorf("failed to clear data set '%s': %w", dataSetName, err)
	}

	return nil
}

// ClearAllDataSets removes all cached CSAF data sets
func ClearAllDataSets() error {
	dataSets, err := ListDataSets()
	if err != nil {
		return fmt.Errorf("failed to list data sets: %w", err)
	}

	var errors []error
	for _, ds := range dataSets {
		if err := ClearDataSet(ds.Name); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to clear some data sets: %v", errors)
	}

	return nil
}

// calculateDirSize calculates the total size of a directory
func calculateDirSize(dirPath string) (int64, error) {
	var size int64

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// FormatSize formats a size in bytes to a human-readable string
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetDataSetSourceURL finds the source URL for a named data set
func GetDataSetSourceURL(dataSetName string) (string, error) {
	cachePath := DetermineCachePath()
	dataSetPath := filepath.Join(cachePath, dataSetName)

	// Check if data set exists
	if _, err := os.Stat(dataSetPath); os.IsNotExist(err) {
		return "", fmt.Errorf("data set '%s' does not exist", dataSetName)
	}

	// Load metadata to get source URL and last sync time
	metadata, err := LoadSyncMetadata(dataSetPath)
	if err != nil {
		return "", fmt.Errorf("failed to load metadata for '%s': %w", dataSetName, err)
	}

	if metadata == nil {
		return "", fmt.Errorf("no metadata found for data set '%s'", dataSetName)
	}

	return metadata.SourceURL, nil
}
