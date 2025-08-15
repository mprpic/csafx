package download

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/mholt/archiver/v3"

	"github.com/mprpic/csafx/pkg/csaf/cache"
)

func GetCSAFArchive(url, destination string) error {
	fmt.Printf("Downloading CSAF archive from %s\n", url)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to archive file: %w", err)
	}
	defer resp.Body.Close()

	out, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	fmt.Printf("Saved CSAF archive to %s\n", destination)
	return nil
}

func ExtractCSAFArchive(archivePath, destination string) error {
	fmt.Printf("Extracting CSAF archive to %s\n", destination)

	zstReader, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer zstReader.Close()

	zr, err := zstd.NewReader(zstReader)
	if err != nil {
		return fmt.Errorf("failed to create zstd reader: %w", err)
	}
	defer zr.Close()

	tarPath := filepath.Join(destination, "csaf_advisories.tar")
	tarFile, err := os.Create(tarPath)
	if err != nil {
		return fmt.Errorf("failed to create tar file: %w", err)
	}
	defer tarFile.Close()

	// Stream decompressed data directly to file instead of loading into memory
	_, err = io.Copy(tarFile, zr)
	if err != nil {
		return fmt.Errorf("failed to decompress archive: %w", err)
	}

	// Close the tar file before extraction
	tarFile.Close()

	archive := archiver.NewTar()
	err = archive.Unarchive(tarPath, destination)
	if err != nil {
		return fmt.Errorf("failed to extract tar archive: %w", err)
	}

	// Clean up the intermediate tar file
	os.Remove(tarPath)

	fmt.Printf("CSAF archive extracted to %s\n", destination)
	return nil
}

// fetchResource performs an HTTP GET request and returns the response body as bytes
func fetchResource(url string, allowNotFound bool) ([]byte, bool, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, false, fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && allowNotFound {
		return nil, true, nil // Return notFound=true for 404 when allowed
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("failed to fetch %s: HTTP %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read response from %s: %w", url, err)
	}

	return data, false, nil
}

func ParseChangesCSV(data []byte) (map[string]time.Time, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse changes.csv: %w", err)
	}

	changes := make(map[string]time.Time)
	for _, record := range records {
		// Skip empty lines
		if len(record) == 0 || (len(record) == 1 && strings.TrimSpace(record[0]) == "") {
			continue
		}

		if len(record) < 2 {
			continue // Skip malformed records
		}

		filePath := record[0]
		timestampStr := record[1]

		timestamp, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp %s: %w", timestampStr, err)
		}
		changes[filePath] = timestamp
	}

	return changes, nil
}

// IndividualFiles downloads a list of files from the base URL to the target directory
func IndividualFiles(baseURL string, filePaths []string, targetDir string) error {
	if len(filePaths) == 0 {
		return nil
	}

	fmt.Printf("Downloading %d individual files...\n", len(filePaths))

	for i, filePath := range filePaths {
		if strings.TrimSpace(filePath) == "" {
			continue // Skip empty file paths
		}

		// Construct full URL for the file
		fileURL := strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(filePath, "/")

		// Determine local file path
		localPath := filepath.Join(targetDir, filePath)

		// Create directory structure if needed
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", localPath, err)
		}

		// Download the file
		resp, err := http.Get(fileURL)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", fileURL, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download %s: HTTP %d", fileURL, resp.StatusCode)
		}

		// Create the local file
		file, err := os.Create(localPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", localPath, err)
		}
		defer file.Close()

		// Copy content
		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return fmt.Errorf("failed to save file %s: %w", localPath, err)
		}

		if (i+1)%10 == 0 || i == len(filePaths)-1 {
			fmt.Printf("Downloaded %d/%d files\n", i+1, len(filePaths))
		}
	}

	return nil
}

// PerformIncrementalUpdate downloads only changed files since the last sync
func PerformIncrementalUpdate(directoryURL, targetDir string, lastSync time.Time) error {
	fmt.Printf("Performing incremental update (changes since %s)\n", lastSync.Format(time.RFC3339))

	// Download changes.csv
	changesURL := strings.TrimSuffix(directoryURL, "/") + "/changes.csv"
	changesData, _, err := fetchResource(changesURL, false)
	if err != nil {
		return fmt.Errorf("failed to get changes.csv: %w", err)
	}

	// Parse changes.csv
	allChanges, err := ParseChangesCSV(changesData)
	if err != nil {
		return fmt.Errorf("failed to parse changes.csv: %w", err)
	}

	// Filter to only files modified after lastSync
	var changedFiles []string
	for filePath, timestamp := range allChanges {
		if timestamp.After(lastSync) {
			changedFiles = append(changedFiles, filePath)
		}
	}

	if len(changedFiles) == 0 {
		fmt.Println("No files have changed since last sync")
		return nil
	}

	fmt.Printf("Found %d files to update\n", len(changedFiles))

	// Download the changed files
	return IndividualFiles(directoryURL, changedFiles, targetDir)
}

// urlToDirectoryName converts a URL to a directory name that is used as a
// identifier for a given cached CSAF data set
func urlToDirectoryName(rawURL string) string {
	u, _ := url.Parse(rawURL)

	// Combine host and path, replace unsafe characters
	dirName := u.Host + u.Path

	re := regexp.MustCompile(`[^a-zA-Z0-9._-]`)
	sanitized := re.ReplaceAllString(dirName, "_")

	// Remove multiple consecutive underscores
	re = regexp.MustCompile(`_+`)
	sanitized = re.ReplaceAllString(sanitized, "_")

	// Trim leading/trailing underscores
	sanitized = strings.Trim(sanitized, "_")

	return sanitized
}

// FromDirectoryURL downloads a CSAF data set from a specific directory URL to
// the cache with support for incremental updates using changes.csv
func FromDirectoryURL(directoryURL string) (string, error) {
	cachePath, err := cache.EnsureCachePath()
	if err != nil {
		return "", fmt.Errorf("failed to ensure cache path: %w", err)
	}

	dirName := urlToDirectoryName(directoryURL)
	targetPath := filepath.Join(cachePath, dirName)

	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	// Check if valid cache exists for incremental update
	isValid, metadata, err := cache.IsValidCache(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to check cache validity: %w", err)
	}

	if isValid && metadata != nil {
		// Perform incremental update
		fmt.Printf("Valid cache found (last sync: %s), performing incremental update\n",
			metadata.LastSync.Format(time.RFC3339))

		err := PerformIncrementalUpdate(directoryURL, targetPath, metadata.LastSync)
		if err != nil {
			return "", fmt.Errorf("incremental update failed: %w", err)
		}

		// Update metadata with current sync time
		newMetadata := &cache.SyncMetadata{
			LastSync:  time.Now(),
			SourceURL: directoryURL,
		}
		if err := cache.SaveSyncMetadata(targetPath, newMetadata); err != nil {
			return "", fmt.Errorf("failed to save sync metadata: %w", err)
		}

		fmt.Println("Incremental update completed successfully")
		return targetPath, nil
	}

	// No valid cache, perform full download
	if metadata != nil {
		fmt.Printf("Cache is stale (last sync: %s), performing full download\n",
			metadata.LastSync.Format(time.RFC3339))
	} else {
		fmt.Println("No cache found, performing full download")
	}

	// Clear cache directory for fresh download
	if err := os.RemoveAll(targetPath); err != nil {
		return "", fmt.Errorf("failed to clear cache directory: %w", err)
	}

	// Recreate the target directory
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return "", fmt.Errorf("failed to recreate target directory: %w", err)
	}

	// Check if an archive is available
	archiveLatestURL := strings.TrimSuffix(directoryURL, "/") + "/archive_latest.txt"
	data, notFound, err := fetchResource(archiveLatestURL, true)
	var archiveURL string
	if notFound {
		fmt.Printf("Warning: no archive found at %s\n", archiveLatestURL)
	} else if err != nil {
		return "", fmt.Errorf("failed to fetch %s: %w", archiveLatestURL, err)
	} else if !notFound {
		archiveURL = strings.TrimSuffix(directoryURL, "/") + "/" + strings.TrimSpace(string(data))
	}

	if archiveURL != "" {
		archivePath := filepath.Join(targetPath, "archive.tar.zst")

		err := GetCSAFArchive(archiveURL, archivePath)
		if err != nil {
			return "", fmt.Errorf("failed to download archive: %w", err)
		}

		err = ExtractCSAFArchive(archivePath, targetPath)
		if err != nil {
			return "", fmt.Errorf("failed to extract archive: %w", err)
		}

		os.Remove(archivePath)
	} else {
		// No archive available, download individual files from index.txt
		fmt.Println("No archive available, downloading individual files from index.txt")
		indexURL := strings.TrimSuffix(directoryURL, "/") + "/index.txt"
		data, _, err := fetchResource(indexURL, false)
		if err != nil {
			return "", fmt.Errorf("failed to fetch index.txt: %w", err)
		}

		lines := strings.Split(string(data), "\n")
		var files []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				files = append(files, line)
			}
		}

		if len(files) == 0 {
			return "", fmt.Errorf("no files found in index.txt")
		}

		err = IndividualFiles(directoryURL, files, targetPath)
		if err != nil {
			return "", fmt.Errorf("failed to download individual files: %w", err)
		}
	}

	// Save metadata for successful full download
	newMetadata := &cache.SyncMetadata{
		LastSync:  time.Now(),
		SourceURL: directoryURL,
	}
	if err := cache.SaveSyncMetadata(targetPath, newMetadata); err != nil {
		return "", fmt.Errorf("failed to save sync metadata: %w", err)
	}

	fmt.Println("Full download completed successfully")
	return targetPath, nil
}

// ProviderMetadata file as defined in the CSAF specification:
// https://docs.oasis-open.org/csaf/csaf/v2.0/os/schemas/provider_json_schema.json
type ProviderMetadata struct {
	CanonicalURL            string         `json:"canonical_url"`
	Distributions           []Distribution `json:"distributions"`
	LastUpdated             string         `json:"last_updated"`
	Publisher               Publisher      `json:"publisher"`
	Role                    string         `json:"role"`
	MetadataVersion         string         `json:"metadata_version"`
	ListOnCSAFAggregators   bool           `json:"list_on_CSAF_aggregators"`
	MirrorOnCSAFAggregators bool           `json:"mirror_on_CSAF_aggregators"`
	PublicOpenPGPKeys       []OpenPGPKey   `json:"public_openpgp_keys"`
}

type Distribution struct {
	DirectoryURL string `json:"directory_url"`
	Rolie        *Rolie `json:"rolie,omitempty"`
}

type Rolie struct {
	Feeds []RolieFeed `json:"feeds"`
}

type RolieFeed struct {
	URL string `json:"url"`
}

// GetDirectoryURLs returns all directory URLs for this distribution.
// If DirectoryURL is set, it returns that in a slice. Otherwise, it derives directory URLs
// from all Rolie feed URLs by replacing the filename with the directory path.
func (d *Distribution) GetDirectoryURLs() ([]string, error) {
	if d.DirectoryURL != "" {
		return []string{d.DirectoryURL}, nil
	}

	if d.Rolie != nil && len(d.Rolie.Feeds) > 0 {
		uniqueURLs := make(map[string]struct{})
		for _, feed := range d.Rolie.Feeds {
			if feed.URL == "" {
				continue
			}

			// Extract the directory part by removing the filename
			lastSlash := strings.LastIndex(feed.URL, "/")
			if lastSlash == -1 {
				continue
			}

			dirURL := feed.URL[:lastSlash+1]
			uniqueURLs[dirURL] = struct{}{}
		}

		if len(uniqueURLs) == 0 {
			return nil, fmt.Errorf("no valid rolie feed URLs found")
		}

		var dirURLs []string
		for url := range uniqueURLs {
			dirURLs = append(dirURLs, url)
		}

		return dirURLs, nil
	}

	return nil, fmt.Errorf("no directory URL or rolie feeds found in distribution")
}

type Publisher struct {
	Category         string `json:"category"`
	Name             string `json:"name"`
	ContactDetails   string `json:"contact_details"`
	Namespace        string `json:"namespace"`
	IssuingAuthority string `json:"issuing_authority,omitempty"`
}

type OpenPGPKey struct {
	Fingerprint string `json:"fingerprint"`
	URL         string `json:"url"`
}

// FromProviderURL downloads CSAF files from a provider metadata URL
func FromProviderURL(providerURL string) (*ProviderMetadata, error) {
	data, _, err := fetchResource(providerURL, false)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch provider providerMetadata: %w", err)
	}

	var providerMetadata ProviderMetadata
	if err := json.Unmarshal(data, &providerMetadata); err != nil {
		return nil, fmt.Errorf("failed to parse provider providerMetadata: %w", err)
	}

	if len(providerMetadata.Distributions) == 0 {
		return nil, fmt.Errorf("no distributions found in provider providerMetadata")
	}

	return &providerMetadata, nil
}

// Aggregator file as defined in the CSAF specification:
// https://docs.oasis-open.org/csaf/csaf/v2.0/os/schemas/aggregator_json_schema.json
type Aggregator struct {
	Aggregator        AggregatorInfo `json:"aggregator"`
	AggregatorVersion string         `json:"aggregator_version"`
	CanonicalURL      string         `json:"canonical_url"`
	CSAFProviders     []CSAFProvider `json:"csaf_providers"`
}

type AggregatorInfo struct {
	Category         string `json:"category"`
	ContactDetails   string `json:"contact_details"`
	IssuingAuthority string `json:"issuing_authority"`
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
}

type CSAFProvider struct {
	Metadata ProviderInfo `json:"metadata"`
}

type ProviderInfo struct {
	LastUpdated string    `json:"last_updated"`
	Publisher   Publisher `json:"publisher"`
	Role        string    `json:"role"`
	URL         string    `json:"url"`
}

// GetAvailableProviders fetches the list of available CSAF providers from the BSI aggregator
func GetAvailableProviders() (*Aggregator, error) {
	aggregatorURL := "https://wid.cert-bund.de/.well-known/csaf-aggregator/aggregator.json"
	data, _, err := fetchResource(aggregatorURL, false)
	if err != nil {
		return nil, err
	}

	var aggregator Aggregator
	if err := json.Unmarshal(data, &aggregator); err != nil {
		return nil, fmt.Errorf("failed to parse aggregator data: %w", err)
	}

	return &aggregator, nil
}
