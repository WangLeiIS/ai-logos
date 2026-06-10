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
		if chunk.BookID != manifest.BookID {
			return nil, fmt.Errorf("chunk %q book_id %q does not match manifest book_id %q", chunk.ChunkID, chunk.BookID, manifest.BookID)
		}
		if chunk.SeqNum < 0 {
			return nil, fmt.Errorf("chunk %q seq_num must be non-negative", chunk.ChunkID)
		}
		if chunk.StartLine < 0 || chunk.EndLine < chunk.StartLine {
			return nil, fmt.Errorf("chunk %q has invalid line range", chunk.ChunkID)
		}
		if chunk.CharCount < 0 {
			return nil, fmt.Errorf("chunk %q char_count must be non-negative", chunk.ChunkID)
		}
	}
	if manifest.ChunkCount != int64(len(chunks)) {
		return nil, fmt.Errorf("manifest chunk_count %d does not match chunks.parquet row count %d", manifest.ChunkCount, len(chunks))
	}

	indexKeywords := make(map[string]struct{})
	indexChunkIDs := make(map[string]map[string]struct{})
	indexEntries := make(map[string]struct{}, len(index))
	indexIDs := make(map[string]struct{}, len(index))
	for _, row := range index {
		if strings.TrimSpace(row.ID) == "" {
			return nil, fmt.Errorf("index id must not be empty")
		}
		if _, exists := indexIDs[row.ID]; exists {
			return nil, fmt.Errorf("duplicate index id %q", row.ID)
		}
		indexIDs[row.ID] = struct{}{}
		if strings.TrimSpace(row.Keyword) == "" {
			return nil, fmt.Errorf("index keyword must not be empty")
		}
		if _, exists := chunkIDs[row.ChunkID]; !exists {
			return nil, fmt.Errorf("index keyword %q references unknown chunk %q", row.Keyword, row.ChunkID)
		}
		entry := NormalizeKeyword(row.Keyword) + "\x00" + row.ChunkID + "\x00" + row.FieldType
		if _, exists := indexEntries[entry]; exists {
			return nil, fmt.Errorf("duplicate index entry for keyword %q, chunk %q, and field %q", row.Keyword, row.ChunkID, row.FieldType)
		}
		indexEntries[entry] = struct{}{}
		if row.FieldType != "title" && row.FieldType != "content" && row.FieldType != "quote" {
			return nil, fmt.Errorf("index keyword %q has invalid field_type %q", row.Keyword, row.FieldType)
		}
		keyword := NormalizeKeyword(row.Keyword)
		indexKeywords[keyword] = struct{}{}
		if indexChunkIDs[keyword] == nil {
			indexChunkIDs[keyword] = make(map[string]struct{})
		}
		indexChunkIDs[keyword][row.ChunkID] = struct{}{}
	}

	idfKeywords := make(map[string]struct{}, len(idf))
	for _, row := range idf {
		if strings.TrimSpace(row.Keyword) == "" {
			return nil, fmt.Errorf("IDF keyword must not be empty")
		}
		keyword := NormalizeKeyword(row.Keyword)
		if _, exists := idfKeywords[keyword]; exists {
			return nil, fmt.Errorf("duplicate IDF keyword %q", row.Keyword)
		}
		idfKeywords[keyword] = struct{}{}
		if math.IsNaN(row.IDF) || math.IsInf(row.IDF, 0) || row.IDF < 0 {
			return nil, fmt.Errorf("keyword %q has invalid idf %v", row.Keyword, row.IDF)
		}
		if row.DF < 0 || int64(row.DF) > manifest.ChunkCount {
			return nil, fmt.Errorf("keyword %q has invalid df %d", row.Keyword, row.DF)
		}
		if chunks := indexChunkIDs[keyword]; int64(row.DF) != int64(len(chunks)) {
			return nil, fmt.Errorf("keyword %q df %d does not match unique indexed chunks %d", row.Keyword, row.DF, len(chunks))
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
