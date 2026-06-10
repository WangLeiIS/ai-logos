package book

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"logos/safepath"
)

const maxConcurrentBookQueries = 4

type scoreState struct {
	title   map[string]struct{}
	content map[string]struct{}
	quote   map[string]struct{}
	matched map[string]struct{}
}

type bundleSearcher func(context.Context, string, string, []string, int) ([]Result, error)

func queryBundle(ctx context.Context, bundleDir, bookID string, tags []string, limit int) ([]Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("query limit must be positive")
	}
	normalized, err := NormalizeTags(tags)
	if err != nil {
		return nil, err
	}
	manifest, err := FastValidate(bundleDir)
	if err != nil {
		return nil, err
	}
	if manifest.BookID != bookID {
		return nil, fmt.Errorf("bundle book_id %q does not match requested book %q", manifest.BookID, bookID)
	}
	tagSet := make(map[string]struct{}, len(normalized))
	for _, tag := range normalized {
		tagSet[tag] = struct{}{}
	}
	idfByKeyword := make(map[string]float64, len(normalized))
	if err := scanParquetRows[IDFRow](bundleDir, idfFile, func(row IDFRow) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		keyword := NormalizeKeyword(row.Keyword)
		if _, matched := tagSet[keyword]; matched {
			idfByKeyword[keyword] = row.IDF
		}
		return nil
	}); err != nil {
		return nil, err
	}
	states := make(map[string]*scoreState)
	if err := scanParquetRows[IndexRow](bundleDir, indexFile, func(row IndexRow) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		keyword := NormalizeKeyword(row.Keyword)
		if _, matched := tagSet[keyword]; !matched {
			return nil
		}
		if _, exists := idfByKeyword[keyword]; !exists {
			return fmt.Errorf("missing IDF row for matched keyword %q", row.Keyword)
		}
		state := states[row.ChunkID]
		if state == nil {
			state = &scoreState{
				title: make(map[string]struct{}), content: make(map[string]struct{}),
				quote: make(map[string]struct{}), matched: make(map[string]struct{}),
			}
			states[row.ChunkID] = state
		}
		state.matched[keyword] = struct{}{}
		switch row.FieldType {
		case "title":
			state.title[keyword] = struct{}{}
		case "content":
			state.content[keyword] = struct{}{}
		case "quote":
			state.quote[keyword] = struct{}{}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	chunkByID := make(map[string]ChunkRow, len(states))
	if err := scanParquetRows[ChunkRow](bundleDir, chunksFile, func(chunk ChunkRow) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, candidate := states[chunk.ChunkID]; candidate {
			chunkByID[chunk.ChunkID] = chunk
		}
		return nil
	}); err != nil {
		return nil, err
	}
	for chunkID := range states {
		if _, exists := chunkByID[chunkID]; !exists {
			return nil, fmt.Errorf("matched index references unknown chunk %q", chunkID)
		}
	}

	results := make([]Result, 0, len(states))
	denominator := float64(len(normalized))
	for chunkID, state := range states {
		chunk := chunkByID[chunkID]
		matched := make([]string, 0, len(state.matched))
		idfSum := 0.0
		for _, tag := range normalized {
			if _, ok := state.matched[tag]; ok {
				matched = append(matched, tag)
				idfSum += idfByKeyword[tag]
			}
		}
		titleCoverage := float64(len(state.title)) / denominator
		contentCoverage := float64(len(state.content)) / denominator
		quoteCoverage := float64(len(state.quote)) / denominator
		avgIDF := idfSum / float64(len(state.matched))
		score := (titleCoverage*10 + contentCoverage*5 + quoteCoverage) * avgIDF
		results = append(results, Result{
			BookID: bookID, ChunkID: chunk.ChunkID, Title: chunk.Summary, Content: chunk.Content,
			SourcePath: chunk.SourceFile, Position: int64(chunk.SeqNum), Score: score,
			TitleCoverage: titleCoverage, ContentCoverage: contentCoverage,
			QuoteCoverage: quoteCoverage, AvgIDF: avgIDF, MatchedKeywords: matched,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Position != results[j].Position {
			return results[i].Position < results[j].Position
		}
		return results[i].ChunkID < results[j].ChunkID
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func QueryBooks(ctx context.Context, rollRoot string, registered []Book, query Query) (*QueryResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	books, err := normalizeBooks(query.Books)
	if err != nil {
		return nil, err
	}
	tags, err := NormalizeTags(query.Tags)
	if err != nil {
		return nil, err
	}
	if query.Limit <= 0 {
		return nil, fmt.Errorf("query limit must be positive")
	}
	if query.PerBookLimit <= 0 {
		return nil, fmt.Errorf("per-book limit must be positive")
	}

	registeredByID := make(map[string]Book, len(registered))
	for _, book := range registered {
		registeredByID[book.BookID] = book
	}
	dirs := make(map[string]string, len(books))
	for _, bookID := range books {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		book, exists := registeredByID[bookID]
		if !exists {
			return nil, fmt.Errorf("book %q is not registered", bookID)
		}
		dir, err := existingPathWithin(rollRoot, book.ResourcePath)
		if err != nil {
			return nil, fmt.Errorf("resolve book %q: %w", bookID, err)
		}
		manifest, err := FastValidate(dir)
		if err != nil {
			return nil, fmt.Errorf("validate book %q: %w", bookID, err)
		}
		if manifest.BookID != bookID {
			return nil, fmt.Errorf("registered book %q points to bundle %q", bookID, manifest.BookID)
		}
		dirs[bookID] = dir
	}
	results, err := queryValidatedBooks(ctx, dirs, books, tags, query.PerBookLimit, query.Limit, queryBundle)
	if err != nil {
		return nil, err
	}
	return &QueryResponse{QueryTags: tags, Books: books, Results: results}, nil
}

func queryValidatedBooks(ctx context.Context, dirs map[string]string, books, tags []string, perBookLimit, limit int, search bundleSearcher) ([]Result, error) {
	type outcome struct {
		results []Result
		err     error
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan string)
	outcomes := make(chan outcome, len(books))
	var wg sync.WaitGroup
	var firstErr error
	var firstErrOnce sync.Once
	workers := min(len(books), maxConcurrentBookQueries)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case bookID, ok := <-jobs:
					if !ok {
						return
					}
					results, err := search(ctx, dirs[bookID], bookID, tags, perBookLimit)
					if err != nil {
						err = fmt.Errorf("query book %q: %w", bookID, err)
						firstErrOnce.Do(func() { firstErr = err })
						cancel()
					}
					outcomes <- outcome{results: results, err: err}
					if err != nil {
						return
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, bookID := range books {
			select {
			case jobs <- bookID:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(outcomes)
	}()

	results := make([]Result, 0)
	for outcome := range outcomes {
		if outcome.err == nil {
			results = append(results, outcome.results...)
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].BookID != results[j].BookID {
			return results[i].BookID < results[j].BookID
		}
		if results[i].Position != results[j].Position {
			return results[i].Position < results[j].Position
		}
		return results[i].ChunkID < results[j].ChunkID
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func normalizeBooks(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := safepath.ValidateName(value); err != nil {
			return nil, fmt.Errorf("invalid book id %q: %w", value, err)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one book is required")
	}
	return result, nil
}
