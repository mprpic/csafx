package view

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Document represents a CSAF document with the minimal fields needed for viewing
type Document struct {
	Document DocumentFields `json:"document"`
}

// DocumentFields contains the main document metadata
type DocumentFields struct {
	Category string       `json:"category"`
	Title    string       `json:"title"`
	Tracking TrackingInfo `json:"tracking"`
}

// TrackingInfo contains document tracking information
type TrackingInfo struct {
	ID string `json:"id"`
}

// ReadFromURL reads a CSAF document from a remote URL
func ReadFromURL(url string) (Document, error) {
	resp, err := http.Get(url)
	if err != nil {
		return Document{}, fmt.Errorf("failed to fetch URL %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Document{}, fmt.Errorf("HTTP request failed with status %d for URL %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Document{}, fmt.Errorf("failed to read response body from %s: %w", url, err)
	}

	return parseDocument(data, url)
}

// ReadFromPath reads a CSAF document from a local file path
func ReadFromPath(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	return parseDocument(data, path)
}

// parseDocument parses JSON data into a CSAF Document and validates required fields
func parseDocument(data []byte, source string) (Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("failed to parse JSON from %s: %w", source, err)
	}

	// Basic validation - ensure required fields are present
	if doc.Document.Category == "" {
		return Document{}, fmt.Errorf("invalid CSAF document: missing document.category field in %s", source)
	}
	if doc.Document.Tracking.ID == "" {
		return Document{}, fmt.Errorf("invalid CSAF document: missing document.tracking.id field in %s", source)
	}
	if doc.Document.Title == "" {
		return Document{}, fmt.Errorf("invalid CSAF document: missing document.title field in %s", source)
	}

	return doc, nil
}
