package book

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"logos/safepath"
)

const (
	FormatV1       = "logos-book"
	FormatVersion1 = 1
	SearchEngineV1 = "keyword-coverage-idf-v1"
)

func LoadManifest(bundleDir string) (*Manifest, error) {
	file, err := openValidatedFile(bundleDir, "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer file.Close()
	var manifest Manifest
	if err := json.NewDecoder(file).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := ValidateManifest(&manifest, filepath.Base(filepath.Clean(bundleDir))); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func ValidateManifest(manifest *Manifest, directoryName string) error {
	if manifest == nil {
		return fmt.Errorf("manifest is required")
	}
	if manifest.Format != FormatV1 {
		return fmt.Errorf("unsupported manifest format %q", manifest.Format)
	}
	if manifest.FormatVersion != FormatVersion1 {
		return fmt.Errorf("unsupported manifest format_version %d", manifest.FormatVersion)
	}
	if err := safepath.ValidateName(manifest.BookID); err != nil {
		return fmt.Errorf("invalid manifest book_id: %w", err)
	}
	if manifest.BookID != directoryName {
		return fmt.Errorf("manifest book_id %q does not match bundle directory %q", manifest.BookID, directoryName)
	}
	if strings.TrimSpace(manifest.Title) == "" {
		return fmt.Errorf("manifest title must not be empty")
	}
	if manifest.ChunkCount < 0 {
		return fmt.Errorf("manifest chunk_count must be non-negative")
	}
	if manifest.SearchEngine != SearchEngineV1 {
		return fmt.Errorf("unsupported manifest search_engine %q", manifest.SearchEngine)
	}
	return nil
}
