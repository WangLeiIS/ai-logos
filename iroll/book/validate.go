package book

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"logos/safepath"
)

func ValidateBundle(bundleDir string) (*Bundle, error) {
	manifest, err := LoadManifest(bundleDir)
	if err != nil {
		return nil, err
	}
	chunks, err := ReadChunks(bundleDir)
	if err != nil {
		return nil, err
	}
	index, err := ReadIndex(bundleDir)
	if err != nil {
		return nil, err
	}
	idf, err := ReadIDF(bundleDir)
	if err != nil {
		return nil, err
	}

	chunkIDs := make(map[string]struct{}, len(chunks))
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.ChunkID) == "" {
			return nil, fmt.Errorf("chunk_id must not be empty")
		}
		if _, exists := chunkIDs[chunk.ChunkID]; exists {
			return nil, fmt.Errorf("duplicate chunk_id %q", chunk.ChunkID)
		}
		chunkIDs[chunk.ChunkID] = struct{}{}
		if strings.TrimSpace(chunk.Content) == "" {
			return nil, fmt.Errorf("chunk %q content must not be empty", chunk.ChunkID)
		}
		if chunk.Position < 0 {
			return nil, fmt.Errorf("chunk %q position must be non-negative", chunk.ChunkID)
		}
		if chunk.SourcePath != "" {
			if err := optionalExistingPathWithin(bundleDir, chunk.SourcePath); err != nil {
				return nil, fmt.Errorf("chunk %q source_path: %w", chunk.ChunkID, err)
			}
		}
	}
	if manifest.ChunkCount != int64(len(chunks)) {
		return nil, fmt.Errorf("manifest chunk_count %d does not match chunks.parquet row count %d", manifest.ChunkCount, len(chunks))
	}

	indexKeywords := make(map[string]struct{})
	indexChunkIDs := make(map[string]map[string]struct{})
	indexPairs := make(map[string]struct{}, len(index))
	for _, row := range index {
		if row.Keyword == "" {
			return nil, fmt.Errorf("index keyword must not be empty")
		}
		if row.Keyword != NormalizeKeyword(row.Keyword) {
			return nil, fmt.Errorf("index keyword %q is not normalized", row.Keyword)
		}
		if _, exists := chunkIDs[row.ChunkID]; !exists {
			return nil, fmt.Errorf("index keyword %q references unknown chunk %q", row.Keyword, row.ChunkID)
		}
		pair := row.Keyword + "\x00" + row.ChunkID
		if _, exists := indexPairs[pair]; exists {
			return nil, fmt.Errorf("duplicate index pair for keyword %q and chunk %q", row.Keyword, row.ChunkID)
		}
		indexPairs[pair] = struct{}{}
		if len(row.Fields) == 0 {
			return nil, fmt.Errorf("index keyword %q fields must not be empty", row.Keyword)
		}
		for _, field := range row.Fields {
			if field != "title" && field != "content" && field != "quote" {
				return nil, fmt.Errorf("index keyword %q has invalid field %q", row.Keyword, field)
			}
		}
		indexKeywords[row.Keyword] = struct{}{}
		if indexChunkIDs[row.Keyword] == nil {
			indexChunkIDs[row.Keyword] = make(map[string]struct{})
		}
		indexChunkIDs[row.Keyword][row.ChunkID] = struct{}{}
	}

	idfKeywords := make(map[string]struct{}, len(idf))
	for _, row := range idf {
		if row.Keyword == "" {
			return nil, fmt.Errorf("IDF keyword must not be empty")
		}
		if row.Keyword != NormalizeKeyword(row.Keyword) {
			return nil, fmt.Errorf("IDF keyword %q is not normalized", row.Keyword)
		}
		if _, exists := idfKeywords[row.Keyword]; exists {
			return nil, fmt.Errorf("duplicate IDF keyword %q", row.Keyword)
		}
		idfKeywords[row.Keyword] = struct{}{}
		if math.IsNaN(row.IDF) || math.IsInf(row.IDF, 0) || row.IDF < 0 {
			return nil, fmt.Errorf("keyword %q has invalid idf %v", row.Keyword, row.IDF)
		}
		if row.DocumentFrequency < 0 || row.DocumentFrequency > manifest.ChunkCount {
			return nil, fmt.Errorf("keyword %q has invalid document_frequency %d", row.Keyword, row.DocumentFrequency)
		}
		if chunks := indexChunkIDs[row.Keyword]; row.DocumentFrequency != int64(len(chunks)) {
			return nil, fmt.Errorf("keyword %q document_frequency %d does not match unique indexed chunks %d", row.Keyword, row.DocumentFrequency, len(chunks))
		}
	}
	for keyword := range indexKeywords {
		if _, exists := idfKeywords[keyword]; !exists {
			return nil, fmt.Errorf("missing IDF row for index keyword %q", keyword)
		}
	}
	return &Bundle{
		Dir:          bundleDir,
		ResourcePath: "Resources/books/" + manifest.BookID,
		Manifest:     *manifest,
	}, nil
}

func Discover(rollRoot string) ([]Bundle, error) {
	booksDir, err := safepath.Join(rollRoot, "Resources/books")
	if err != nil {
		return nil, fmt.Errorf("resolve books directory: %w", err)
	}
	if _, err := os.Lstat(booksDir); os.IsNotExist(err) {
		return []Bundle{}, nil
	}
	booksDir, err = existingPathWithin(rollRoot, "Resources/books")
	if err != nil {
		return nil, fmt.Errorf("resolve books directory: %w", err)
	}
	entries, err := os.ReadDir(booksDir)
	if err != nil {
		return nil, fmt.Errorf("read books directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	bundles := make([]Bundle, 0, len(entries))
	for _, entry := range entries {
		relativePath := "Resources/books/" + entry.Name()
		dir, err := existingPathWithin(rollRoot, relativePath)
		if err != nil {
			return nil, fmt.Errorf("validate book %q: %w", entry.Name(), err)
		}
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("stat book %q: %w", entry.Name(), err)
		}
		if !info.IsDir() {
			continue
		}
		bundle, err := ValidateBundle(dir)
		if err != nil {
			return nil, fmt.Errorf("validate book %q: %w", entry.Name(), err)
		}
		bundles = append(bundles, *bundle)
	}
	return bundles, nil
}
