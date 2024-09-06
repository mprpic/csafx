package main

import (
	"errors"
	"fmt"
	"github.com/mprpic/csafx/pkg/csaf/download"
	"log"
	"os"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "csafx",
	Short: "CSAF Explorer",
}

var (
	providerURL  string
	directoryURL string
)

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

func init() {
	downloadCmd.Flags().StringVarP(&providerURL, "provider", "p", "", "URL to a provider-metadata.json file that points to a CSAF data set")
	downloadCmd.Flags().StringVarP(&directoryURL, "directory", "d", "", "URL to a CSAF directory that contains an index.txt file")
	rootCmd.AddCommand(downloadCmd)
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

	var items []string
	var allDirURLs []string
	for _, dist := range providerMetadata.Distributions {
		dirURLs, err := dist.GetDirectoryURLs()
		if err != nil {
			fmt.Printf("Warning: failed to get directory URLs for distribution: %v\n", err)
			continue
		}
		for _, dirURL := range dirURLs {
			items = append(items, dirURL)
			allDirURLs = append(allDirURLs, dirURL)
		}
	}

	if len(allDirURLs) == 1 {
		if err := downloadFromDirectoryURL(allDirURLs[0]); err != nil {
			return fmt.Errorf("failed to download from %s: %w", allDirURLs[0], err)
		}
		return nil
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

	if choiceIndex >= len(allDirURLs) {
		return fmt.Errorf("invalid choice index: %d", choiceIndex)
	}

	selectedDirURL := allDirURLs[choiceIndex]
	return downloadFromDirectoryURL(selectedDirURL)
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
