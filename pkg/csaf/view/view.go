// Package view provides functionality for viewing CSAF documents in a Terminal User Interface.
package view

import "strings"

// ViewDocument reads a CSAF document from the given path or URL and displays it in a TUI.
// The pathOrURL can be either a local file path or a HTTP/HTTPS URL.
// Returns an error if the document cannot be read, parsed, or displayed.
func ViewDocument(pathOrURL string) error {
	var doc Document
	var err error

	// Determine if the input is a URL or file path
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		doc, err = ReadFromURL(pathOrURL)
	} else {
		doc, err = ReadFromPath(pathOrURL)
	}

	if err != nil {
		return err
	}

	// Run the TUI
	return RunTUI(doc)
}

// ViewDocumentFromURL reads a CSAF document from a URL and displays it in a TUI.
func ViewDocumentFromURL(url string) error {
	doc, err := ReadFromURL(url)
	if err != nil {
		return err
	}
	return RunTUI(doc)
}

// ViewDocumentFromPath reads a CSAF document from a local file path and displays it in a TUI.
func ViewDocumentFromPath(path string) error {
	doc, err := ReadFromPath(path)
	if err != nil {
		return err
	}
	return RunTUI(doc)
}
