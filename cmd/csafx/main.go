package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"github.com/manifoldco/promptui"
	"github.com/mprpic/csafx/pkg/csaf/cache"
	"github.com/mprpic/csafx/pkg/csaf/download"
	"github.com/mprpic/csafx/pkg/csaf/view"
	"github.com/spf13/cobra"
	"log"
	"os"
	"strings"
)

var rootCmd = &cobra.Command{
	Use:   "csafx",
	Short: "CSAF Explorer",
}

var (
	providerURL  string
	directoryURL string
	clearAll     bool
	interactive  bool
)

var viewCmd = &cobra.Command{
	Use:   "view <path_or_url_to_csaf_file.json>",
	Short: "View a CSAF document in a clean TUI",
	Long: `View a CSAF JSON file in a Terminal User Interface.

The command accepts either a local file path or a URL to a remote CSAF file.

Examples:
  # View a local file
  csafx view /path/to/csaf-document.json

  # View a remote file
  csafx view https://example.com/advisories/document.json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		source := args[0]

		var err error
		// Determine if the input is a URL or file path and call the appropriate function
		if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
			err = view.ViewDocumentFromURL(source)
		} else {
			err = view.ViewDocumentFromPath(source)
		}

		if err != nil {
			log.Fatalf("Error viewing CSAF document: %v", err)
		}
	},
}

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download CSAF data set or update an existing one",
	Long: `Download CSAF data set from provider metadata or directly from a directory URL.

Examples:
  # Download from a specific provider metadata URL
  csafx download --provider https://example.com/.well-known/csaf/provider-metadata.json

  # Download directly from a directory URL (where index.txt is located)
  csafx download --directory https://example.com/csaf/advisories/

  # Use BSI aggregator to select from available CSAF providers
  csafx download`,
	Run: func(cmd *cobra.Command, args []string) {
		if directoryURL != "" {
			// Direct directory URL specified
			err := downloadFromDirectoryURL(directoryURL)
			if err != nil {
				log.Fatalf("Error downloading from directory: %v", err)
			}
			return
		}

		if providerURL != "" {
			// Provider metadata URL specified
			err := downloadFromProviderURL(providerURL)
			if err != nil {
				log.Fatalf("Error downloading from provider: %v", err)
			}
			return
		}

		// No URL specified, use BSI aggregator
		err := downloadFromAggregator()
		if err != nil {
			log.Fatalf("Error downloading from aggregator: %v", err)
		}
	},
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage CSAF cache",
	Long:  "Manage the local CSAF cache with list and clear operations",
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available cached CSAF data sets",
	Long:  "List all available cached CSAF data sets with their sizes",
	Run: func(cmd *cobra.Command, args []string) {
		if err := listCacheDataSets(); err != nil {
			log.Fatalf("Error listing cached CSAF data sets: %v", err)
		}
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear [data-set]",
	Short: "Clear cached data sets",
	Long: `Clear cache data sets. Can clear specific data set, all data sets, or use interactive mode.

Examples:
  # Clear a specific data set
  csafx cache clear example.com_csaf_advisories

  # Clear all cached data sets
  csafx cache clear --all

  # Interactive multi-select of data sets to clear
  csafx cache clear --interactive`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := clearCacheDataSets(cmd, args); err != nil {
			log.Fatalf("Error clearing cache: %v", err)
		}
	},
}

var cacheSyncCmd = &cobra.Command{
	Use:   "sync [data-set]",
	Short: "Sync cached data sets",
	Long: `Sync cached data sets by re-downloading them from their original source.
Data sets older than 3 weeks will be re-downloaded using archives if available.

Examples:
  # Sync a specific data set
  csafx cache sync example.com_csaf_advisories

  # Sync all cached data sets
  csafx cache sync --all

  # Interactive selection of data sets to sync
  csafx cache sync --interactive`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := syncCacheDataSets(cmd, args); err != nil {
			log.Fatalf("Error syncing cache: %v", err)
		}
	},
}

func init() {
	downloadCmd.Flags().StringVarP(&providerURL, "provider", "p", "", "URL to a provider-metadata.json file that points to a CSAF data set")
	downloadCmd.Flags().StringVarP(&directoryURL, "directory", "d", "", "URL to a CSAF directory that contains an index.txt file")

	cacheClearCmd.Flags().BoolVar(&clearAll, "all", false, "Clear all cached CSAF data sets")
	cacheClearCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive multi-select of data sets to clear")

	cacheSyncCmd.Flags().BoolVar(&clearAll, "all", false, "Sync all cached CSAF data sets")
	cacheSyncCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive multi-select of data sets to sync")

	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheSyncCmd)

	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(viewCmd)
	rootCmd.AddCommand(cacheCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// downloadFromDirectoryURL handles CLI interaction for directory URL downloads
func downloadFromDirectoryURL(directoryURL string) error {
	fmt.Printf("Downloading from directory: %s\n", directoryURL)
	fmt.Println("Checking for available archive...")
	targetPath, err := download.FromDirectoryURL(directoryURL)
	if err != nil {
		return err
	}
	fmt.Printf("Successfully downloaded to: %s\n", targetPath)
	return nil
}

// downloadFromProviderURL handles CLI interaction for provider-metadata URL downloads
func downloadFromProviderURL(providerURL string) error {
	fmt.Printf("Fetching provider metadata from: %s\n", providerURL)
	providerMetadata, err := download.FromProviderURL(providerURL)
	if err != nil {
		return err
	}

	fmt.Printf("Provider: %s (%s)\n", providerMetadata.Publisher.Name, providerMetadata.Publisher.Category)

	urlSet := make(map[string]struct{})
	for _, dist := range providerMetadata.Distributions {
		dirURLs, err := dist.GetDirectoryURLs()
		if err != nil {
			fmt.Printf("Warning: failed to get directory URLs for distribution: %v\n", err)
			continue
		}
		for _, dirURL := range dirURLs {
			urlSet[dirURL] = struct{}{}
		}
	}

	var allDirURLs []string
	for url := range urlSet {
		allDirURLs = append(allDirURLs, url)
	}
	if len(allDirURLs) == 1 {
		if err := downloadFromDirectoryURL(allDirURLs[0]); err != nil {
			return fmt.Errorf("failed to download from %s: %w", allDirURLs[0], err)
		}
		return nil
	}

	var items []string
	for _, url := range allDirURLs {
		items = append(items, url)
	}
	items = append(items, "Download all")
	prompt := promptui.Select{
		Label:    "Select data set to download",
		Items:    items,
		HideHelp: true,
		Size:     len(items),
	}

	choiceIndex, choice, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrInterrupt) {
			os.Exit(0)
		}
		return fmt.Errorf("selection cancelled: %w", err)
	}

	if choice == "Download all" {
		var targetPaths []string
		var errors []error

		for _, dirURL := range allDirURLs {
			targetPath, err := download.FromDirectoryURL(dirURL)
			if err != nil {
				errors = append(errors, fmt.Errorf("failed to download %s: %w", dirURL, err))
				continue
			}
			targetPaths = append(targetPaths, targetPath)
		}

		for _, path := range targetPaths {
			fmt.Printf("Downloaded to: %s\n", path)
		}
		for _, err := range errors {
			fmt.Printf("Failed to download: %v\n", err)
		}
		if len(errors) > 0 {
			return fmt.Errorf("completed with %d errors", len(errors))
		}
		return nil
	}

	return downloadFromDirectoryURL(allDirURLs[choiceIndex])
}

// downloadFromAggregator handles CLI interaction for aggregator-based downloads
func downloadFromAggregator() error {
	fmt.Println("Fetching CSAF provider list from BSI aggregator...")

	aggregator, err := download.GetAvailableProviders()
	if err != nil {
		return fmt.Errorf("failed to fetch aggregator data: %w", err)
	}

	if len(aggregator.CSAFProviders) == 0 {
		return fmt.Errorf("no providers found in aggregator")
	}

	var items []string
	for _, provider := range aggregator.CSAFProviders {
		items = append(items, fmt.Sprintf("%s (%s)", provider.Metadata.Publisher.Name, provider.Metadata.Publisher.Namespace))
	}

	prompt := promptui.Select{
		Label:    "Select CSAF provider to download from",
		Items:    items,
		HideHelp: true,
		Size:     len(items),
	}

	choiceIndex, _, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrInterrupt) {
			os.Exit(0)
		}
		return fmt.Errorf("selection cancelled: %w", err)
	}

	selectedProvider := aggregator.CSAFProviders[choiceIndex]
	return downloadFromProviderURL(selectedProvider.Metadata.URL)
}

// listCacheDataSets lists all available cached CSAF data sets with their sizes
func listCacheDataSets() error {
	dataSets, err := cache.ListDataSets()
	if err != nil {
		return err
	}

	if len(dataSets) == 0 {
		fmt.Println("No cached CSAF data sets found")
		return nil
	}

	fmt.Printf("Available cached CSAF data sets:\n\n")
	for _, ds := range dataSets {
		fmt.Printf("%-20s %s\n", ds.Name, cache.FormatSize(ds.Size))
	}

	return nil
}

// clearCacheDataSets handles the cache clear command logic
func clearCacheDataSets(cmd *cobra.Command, args []string) error {
	if clearAll {
		return clearAllDataSets()
	}

	if interactive {
		return interactiveClear()
	}

	if len(args) == 1 {
		dataSetName := args[0]
		return clearOneDataSet(dataSetName)
	}

	return cmd.Help()
}

// clearAllDataSets confirms and clears all cached CSAF data sets
func clearAllDataSets() error {
	dataSets, err := cache.ListDataSets()
	if err != nil {
		return err
	}

	if len(dataSets) == 0 {
		fmt.Println("No cached CSAF data sets to clear")
		return nil
	}

	fmt.Printf("This will clear all %d cached CSAF data sets\n", len(dataSets))
	for _, ds := range dataSets {
		fmt.Printf("  - %s (%s)\n", ds.Name, cache.FormatSize(ds.Size))
	}

	prompt := promptui.Prompt{
		Label:     "Are you sure you want to continue",
		IsConfirm: true,
	}

	_, err = prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			fmt.Println("Operation cancelled")
			return nil
		}
		return err
	}

	if err := cache.ClearAllDataSets(); err != nil {
		return err
	}

	fmt.Println("Successfully cleared all cached CSAF data sets")
	return nil
}

// clearOneDataSet confirms and clears a specific cached CSAF data set
func clearOneDataSet(dataSetName string) error {
	dataSets, err := cache.ListDataSets()
	if err != nil {
		return err
	}

	// Find the data set
	var targetDataSet *cache.DataSetInfo
	for _, ds := range dataSets {
		if ds.Name == dataSetName {
			targetDataSet = &ds
			break
		}
	}

	if targetDataSet == nil {
		fmt.Printf("Data set '%s' not found\n", dataSetName)
		fmt.Println("\nAvailable data sets:")
		return listCacheDataSets()
	}

	fmt.Printf("This will clear data set '%s' (%s)\n", targetDataSet.Name, cache.FormatSize(targetDataSet.Size))

	prompt := promptui.Prompt{
		Label:     "Are you sure you want to continue",
		IsConfirm: true,
	}

	_, err = prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			fmt.Println("Operation cancelled")
			return nil
		}
		return err
	}

	if err := cache.ClearDataSet(dataSetName); err != nil {
		return err
	}

	fmt.Printf("Successfully cleared data set '%s' (%s freed)\n", dataSetName, cache.FormatSize(targetDataSet.Size))
	return nil
}

// interactiveClear provides interactive multi-select for clearing cached CSAF data sets
func interactiveClear() error {
	dataSets, err := cache.ListDataSets()
	if err != nil {
		return err
	}

	if len(dataSets) == 0 {
		fmt.Println("No cached CSAF data sets to clear")
		return nil
	}

	// Create items for selection with size information
	var items []string
	for _, ds := range dataSets {
		items = append(items, fmt.Sprintf("%s (%s)", ds.Name, cache.FormatSize(ds.Size)))
	}

	// Use promptui.SelectWithAdd for multi-select simulation
	// Since promptui doesn't have native multi-select, we'll use a loop
	var selectedIndices []int
	var selectedDataSets []cache.DataSetInfo

	for {

		// Add options to the items
		displayItems := make([]string, len(items))
		copy(displayItems, items)

		// Mark selected items
		for _, idx := range selectedIndices {
			displayItems[idx] = "✓ " + items[idx]
		}

		displayItems = append(displayItems, "--- Done (proceed with clearing) ---")
		displayItems = append(displayItems, "--- Cancel ---")

		prompt := promptui.Select{
			Label:    "Select data sets to clear (choose 'Done' when finished)",
			Items:    displayItems,
			HideHelp: true,
			Size:     len(displayItems),
		}

		choiceIndex, _, err := prompt.Run()
		if err != nil {
			if errors.Is(err, promptui.ErrInterrupt) {
				fmt.Println("Operation cancelled")
				return nil
			}
			return err
		}

		// Handle special options
		if choiceIndex == len(items) {
			// Done - proceed with clearing
			break
		} else if choiceIndex == len(items)+1 {
			// Cancel
			fmt.Println("Operation cancelled")
			return nil
		}

		// Toggle selection
		isSelected := false
		for i, idx := range selectedIndices {
			if idx == choiceIndex {
				// Remove from selection
				selectedIndices = append(selectedIndices[:i], selectedIndices[i+1:]...)
				for j, ds := range selectedDataSets {
					if ds.Name == dataSets[choiceIndex].Name {
						selectedDataSets = append(selectedDataSets[:j], selectedDataSets[j+1:]...)
						break
					}
				}
				isSelected = true
				break
			}
		}

		if !isSelected {
			// Add to selection
			selectedIndices = append(selectedIndices, choiceIndex)
			selectedDataSets = append(selectedDataSets, dataSets[choiceIndex])
		}
	}

	if len(selectedDataSets) == 0 {
		fmt.Println("No data sets selected")
		return nil
	}

	// Final confirmation
	var totalSize int64
	fmt.Printf("\nSelected data sets to clear:\n")
	for _, ds := range selectedDataSets {
		fmt.Printf("  - %s (%s)\n", ds.Name, cache.FormatSize(ds.Size))
		totalSize += ds.Size
	}
	fmt.Printf("Total size: %s\n", cache.FormatSize(totalSize))

	confirmPrompt := promptui.Prompt{
		Label:     "Are you sure you want to clear these data sets",
		IsConfirm: true,
	}

	_, err = confirmPrompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			fmt.Println("Operation cancelled")
			return nil
		}
		return err
	}

	// Clear selected data sets
	var clearErrors []error
	var clearedSize int64
	for _, ds := range selectedDataSets {
		if err := cache.ClearDataSet(ds.Name); err != nil {
			clearErrors = append(clearErrors, fmt.Errorf("failed to clear %s: %w", ds.Name, err))
		} else {
			clearedSize += ds.Size
			fmt.Printf("Cleared data set '%s'\n", ds.Name)
		}
	}

	if len(clearErrors) > 0 {
		fmt.Printf("Some operations failed:\n")
		for _, err := range clearErrors {
			fmt.Printf("  - %v\n", err)
		}
	}

	fmt.Printf("Successfully cleared %d data sets (%s freed)\n",
		len(selectedDataSets)-len(clearErrors), cache.FormatSize(clearedSize))

	if len(clearErrors) > 0 {
		return fmt.Errorf("completed with %d errors", len(clearErrors))
	}

	return nil
}

// syncCacheDataSets handles the cache sync command logic
func syncCacheDataSets(cmd *cobra.Command, args []string) error {
	if clearAll {
		return syncAllDataSets()
	}

	if interactive {
		return interactiveSync()
	}

	if len(args) == 1 {
		dataSetName := args[0]
		return syncDataSet(dataSetName)
	}

	return cmd.Help()
}

// syncDataSet syncs a specific cached CSAF data set
func syncDataSet(dataSetName string) error {
	sourceURL, err := cache.GetDataSetSourceURL(dataSetName)
	if err != nil {
		return err
	}

	fmt.Printf("Data set: %s\n", dataSetName)
	fmt.Printf("Syncing data set from: %s\n", sourceURL)
	targetPath, err := download.FromDirectoryURL(sourceURL)
	if err != nil {
		return fmt.Errorf("failed to sync data set: %w", err)
	}

	fmt.Printf("Successfully synced data set to: %s\n", targetPath)
	return nil
}

// syncAllDataSets syncs all cached CSAF data sets
func syncAllDataSets() error {
	dataSets, err := cache.ListDataSets()
	if err != nil {
		return err
	}

	if len(dataSets) == 0 {
		fmt.Println("No cached CSAF data sets to sync")
		return nil
	}

	var dataSetsToSync []string
	for _, ds := range dataSets {
		_, err := cache.GetDataSetSourceURL(ds.Name)
		if err != nil {
			fmt.Printf("Warning: Could not find data set for %s: %v\n", ds.Name, err)
			continue
		}
		dataSetsToSync = append(dataSetsToSync, ds.Name)
	}

	fmt.Printf("This will sync %d data sets:\n", len(dataSetsToSync))
	for _, dsName := range dataSetsToSync {
		// Find the corresponding DataSetInfo to get size
		var dsInfo *cache.DataSetInfo
		for _, ds := range dataSets {
			if ds.Name == dsName {
				dsInfo = &ds
				break
			}
		}
		if dsInfo != nil {
			fmt.Printf("  - %s (%s)\n", dsInfo.Name, cache.FormatSize(dsInfo.Size))
		} else {
			fmt.Printf("  - %s\n", dsName)
		}
	}

	prompt := promptui.Prompt{
		Label:     "Are you sure you want to continue",
		IsConfirm: true,
	}

	_, err = prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			fmt.Println("Sync cancelled")
			return nil
		}
		return err
	}

	var syncErrors []error
	var successCount int

	for _, dsName := range dataSetsToSync {
		fmt.Printf("\nSyncing %s...\n", dsName)

		sourceURL, err := cache.GetDataSetSourceURL(dsName)
		if err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("failed to get source URL for %s: %w", dsName, err))
			continue
		}

		_, err = download.FromDirectoryURL(sourceURL)
		if err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("failed to sync %s: %w", dsName, err))
		} else {
			successCount++
			fmt.Printf("Successfully synced %s\n", dsName)
		}
	}

	if len(syncErrors) > 0 {
		fmt.Printf("\nSome operations failed:\n")
		for _, err := range syncErrors {
			fmt.Printf("  - %v\n", err)
		}
	}

	fmt.Printf("\nSuccessfully synced %d out of %d data sets\n", successCount, len(dataSetsToSync))

	if len(syncErrors) > 0 {
		return fmt.Errorf("completed with %d errors", len(syncErrors))
	}

	return nil
}

// interactiveSync provides interactive multi-select for syncing cached CSAF data sets
func interactiveSync() error {
	dataSets, err := cache.ListDataSets()
	if err != nil {
		return err
	}

	if len(dataSets) == 0 {
		fmt.Println("No cached CSAF data sets to sync")
		return nil
	}

	// Create items for selection with sync status information
	var items []string
	var sourceURLs []string

	for _, ds := range dataSets {
		sourceURL, err := cache.GetDataSetSourceURL(ds.Name)
		if err != nil {
			items = append(items, fmt.Sprintf("%s (%s) - Error: %v", ds.Name, cache.FormatSize(ds.Size), err))
			sourceURLs = append(sourceURLs, "")
			continue
		}

		// Check if cache is valid to determine sync status
		cachePath := cache.DetermineCachePath()
		dataSetPath := filepath.Join(cachePath, ds.Name)
		isValid, _, err := cache.IsValidCache(dataSetPath)
		status := "up to date"
		if err != nil || !isValid {
			status = "needs sync (>3 weeks old)"
		}

		items = append(items, fmt.Sprintf("%s (%s) - %s", ds.Name, cache.FormatSize(ds.Size), status))
		sourceURLs = append(sourceURLs, sourceURL)
	}

	var selectedIndices []int
	var selectedDataSets []cache.DataSetInfo

	for {

		// Add options to the items
		displayItems := make([]string, len(items))
		copy(displayItems, items)

		// Mark selected items
		for _, idx := range selectedIndices {
			displayItems[idx] = "✓ " + items[idx]
		}

		displayItems = append(displayItems, "--- Done (proceed with syncing) ---")
		displayItems = append(displayItems, "--- Cancel ---")

		prompt := promptui.Select{
			Label:    "Select data sets to sync (choose 'Done' when finished)",
			Items:    displayItems,
			HideHelp: true,
			Size:     len(displayItems),
		}

		choiceIndex, _, err := prompt.Run()
		if err != nil {
			if errors.Is(err, promptui.ErrInterrupt) {
				fmt.Println("Operation cancelled")
				return nil
			}
			return err
		}

		// Handle special options
		if choiceIndex == len(items) {
			// Done - proceed with syncing
			break
		} else if choiceIndex == len(items)+1 {
			// Cancel
			fmt.Println("Operation cancelled")
			return nil
		}

		// Toggle selection
		isSelected := false
		for i, idx := range selectedIndices {
			if idx == choiceIndex {
				// Remove from selection
				selectedIndices = append(selectedIndices[:i], selectedIndices[i+1:]...)
				for j, ds := range selectedDataSets {
					if ds.Name == dataSets[choiceIndex].Name {
						selectedDataSets = append(selectedDataSets[:j], selectedDataSets[j+1:]...)
						break
					}
				}
				isSelected = true
				break
			}
		}

		if !isSelected {
			// Add to selection
			selectedIndices = append(selectedIndices, choiceIndex)
			selectedDataSets = append(selectedDataSets, dataSets[choiceIndex])
		}
	}

	if len(selectedDataSets) == 0 {
		fmt.Println("No data sets selected")
		return nil
	}

	// Final confirmation
	fmt.Printf("\nSelected data sets to sync:\n")
	for _, ds := range selectedDataSets {
		fmt.Printf("  - %s (%s)\n", ds.Name, cache.FormatSize(ds.Size))
	}

	confirmPrompt := promptui.Prompt{
		Label:     "Are you sure you want to sync these data sets",
		IsConfirm: true,
	}

	_, err = confirmPrompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			fmt.Println("Operation cancelled")
			return nil
		}
		return err
	}

	// Sync selected data sets
	var syncErrors []error
	var successCount int

	for i, ds := range selectedDataSets {
		fmt.Printf("\nSyncing %s (%d/%d)...\n", ds.Name, i+1, len(selectedDataSets))

		// Find the corresponding source URL
		var sourceURL string
		for j, originalDS := range dataSets {
			if originalDS.Name == ds.Name {
				sourceURL = sourceURLs[j]
				break
			}
		}

		if sourceURL == "" {
			syncErrors = append(syncErrors, fmt.Errorf("could not find source URL for %s", ds.Name))
			continue
		}

		_, err = download.FromDirectoryURL(sourceURL)
		if err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("failed to sync %s: %w", ds.Name, err))
		} else {
			successCount++
			fmt.Printf("Successfully synced %s\n", ds.Name)
		}
	}

	if len(syncErrors) > 0 {
		fmt.Printf("\nSome operations failed:\n")
		for _, err := range syncErrors {
			fmt.Printf("  - %v\n", err)
		}
	}

	fmt.Printf("\nSuccessfully synced %d out of %d data sets\n", successCount, len(selectedDataSets))

	if len(syncErrors) > 0 {
		return fmt.Errorf("completed with %d errors", len(syncErrors))
	}

	return nil
}
