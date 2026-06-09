package book

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestQueryBundleScoresCoverageTimesAverageIDF(t *testing.T) {
	dir := writeScoringBundle(t, t.TempDir(), "score-book", 0)
	results, err := queryBundle(context.Background(), dir, "score-book", []string{"laser", "strength"}, 10)
	if err != nil {
		t.Fatalf("queryBundle returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("queryBundle returned %d results, want 1", len(results))
	}
	got := results[0]
	wantScore := (0.5*10 + 1.0*5 + 0.5*1) * ((2.0 + 4.0) / 2.0)
	if math.Abs(got.Score-wantScore) > 1e-9 {
		t.Fatalf("score = %v, want %v", got.Score, wantScore)
	}
	if got.TitleCoverage != 0.5 || got.ContentCoverage != 1 || got.QuoteCoverage != 0.5 || got.AvgIDF != 3 {
		t.Fatalf("scoring details = %#v", got)
	}
}

func TestQueryBundleCountsOneTagAcrossFieldsWithoutFrequency(t *testing.T) {
	manifest := validManifest("fields-book")
	chunks := []ChunkRow{{ChunkID: "chunk-1", Content: "content", Position: 0}}
	index := []IndexRow{{Keyword: "laser", ChunkID: "chunk-1", Fields: []string{"title", "title", "content", "quote"}}}
	idf := []IDFRow{{Keyword: "laser", IDF: 2, DocumentFrequency: 1}}
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)

	results, err := queryBundle(context.Background(), dir, "fields-book", []string{"laser"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Score != 32 || results[0].AvgIDF != 2 {
		t.Fatalf("queryBundle results = %#v, want one result with score 32", results)
	}
}

func TestQueryBundleRequiresExactNormalizedMatchAndOmitsNoMatch(t *testing.T) {
	dir := writeScoringBundle(t, t.TempDir(), "exact-book", 0)
	results, err := queryBundle(context.Background(), dir, "exact-book", []string{" LASER ", "laser"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || len(results[0].MatchedKeywords) != 1 || results[0].MatchedKeywords[0] != "laser" {
		t.Fatalf("normalized query results = %#v", results)
	}
	results, err = queryBundle(context.Background(), dir, "exact-book", []string{"las"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("substring query returned %#v, want no results", results)
	}
}

func TestQueryBooksAppliesLimitsAndStableTieBreaks(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "Resources", "books")
	writeTieBundle(t, booksDir, "b-book", []int64{2, 1})
	writeTieBundle(t, booksDir, "a-book", []int64{3, 0})
	registered := []Book{
		{BookID: "a-book", ResourcePath: "Resources/books/a-book"},
		{BookID: "b-book", ResourcePath: "Resources/books/b-book"},
	}

	response, err := QueryBooks(context.Background(), root, registered, Query{
		Books: []string{"b-book", "a-book", "a-book"}, Tags: []string{" TAG ", "tag"}, Limit: 3, PerBookLimit: 2,
	})
	if err != nil {
		t.Fatalf("QueryBooks returned error: %v", err)
	}
	if len(response.Results) != 3 {
		t.Fatalf("got %d results, want 3", len(response.Results))
	}
	got := []string{
		response.Results[0].BookID + ":" + response.Results[0].ChunkID,
		response.Results[1].BookID + ":" + response.Results[1].ChunkID,
		response.Results[2].BookID + ":" + response.Results[2].ChunkID,
	}
	want := []string{"a-book:chunk-0", "a-book:chunk-3", "b-book:chunk-1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %#v, want %#v", got, want)
		}
	}
	if len(response.Books) != 2 || len(response.QueryTags) != 1 {
		t.Fatalf("deduplicated response metadata = %#v", response)
	}
}

func TestQueryBooksAppliesPerBookLimit(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "Resources", "books")
	writeTieBundle(t, booksDir, "a-book", []int64{0, 1})
	response, err := QueryBooks(context.Background(), root, []Book{{BookID: "a-book", ResourcePath: "Resources/books/a-book"}}, Query{
		Books: []string{"a-book"}, Tags: []string{"tag"}, Limit: 10, PerBookLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].ChunkID != "chunk-0" {
		t.Fatalf("results = %#v, want only chunk-0", response.Results)
	}
}

func TestQueryBooksFailsEntirelyWhenOneBookIsInvalid(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "Resources", "books")
	writeTieBundle(t, booksDir, "good-book", []int64{0})
	badDir := writeTieBundle(t, booksDir, "bad-book", []int64{0})
	if err := os.Remove(filepath.Join(badDir, idfFile)); err != nil {
		t.Fatal(err)
	}
	registered := []Book{
		{BookID: "good-book", ResourcePath: "Resources/books/good-book"},
		{BookID: "bad-book", ResourcePath: "Resources/books/bad-book"},
	}
	if _, err := QueryBooks(context.Background(), root, registered, Query{
		Books: []string{"good-book", "bad-book"}, Tags: []string{"tag"}, Limit: 10, PerBookLimit: 5,
	}); err == nil || !strings.Contains(err.Error(), "bad-book") {
		t.Fatalf("QueryBooks error = %v, want bad-book error", err)
	}
}

func TestQueryBooksRejectsInvalidInputAndUnknownBook(t *testing.T) {
	root := t.TempDir()
	tests := []Query{
		{Tags: []string{"tag"}, Limit: 1, PerBookLimit: 1},
		{Books: []string{"missing"}, Limit: 1, PerBookLimit: 1},
		{Books: []string{"missing"}, Tags: []string{"tag"}, Limit: 0, PerBookLimit: 1},
		{Books: []string{"missing"}, Tags: []string{"tag"}, Limit: 1, PerBookLimit: 0},
	}
	for _, query := range tests {
		if _, err := QueryBooks(context.Background(), root, nil, query); err == nil {
			t.Fatalf("QueryBooks accepted invalid query %#v", query)
		}
	}
}

func TestQueryBooksRejectsRegisteredBundleEscapingRollRootViaSymlink(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "Resources", "books")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, chunks, index, idf := validBundleRows()
	outside := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	symlinkOrSkip(t, outside, filepath.Join(booksDir, manifest.BookID))

	_, err := QueryBooks(context.Background(), root, []Book{{
		BookID: manifest.BookID, ResourcePath: "Resources/books/" + manifest.BookID,
	}}, Query{Books: []string{manifest.BookID}, Tags: []string{"laser"}, Limit: 1, PerBookLimit: 1})
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("QueryBooks error = %v, want roll root escape error", err)
	}
}

func TestQueryBooksAllowsRegisteredBundleLinkWithinRollRoot(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "Resources", "books")
	targetsDir := filepath.Join(root, "Resources", "book-targets")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, chunks, index, idf := validBundleRows()
	target := writeBundleFixture(t, targetsDir, manifest, chunks, index, idf)
	symlinkOrSkip(t, target, filepath.Join(booksDir, manifest.BookID))

	response, err := QueryBooks(context.Background(), root, []Book{{
		BookID: manifest.BookID, ResourcePath: "Resources/books/" + manifest.BookID,
	}}, Query{Books: []string{manifest.BookID}, Tags: []string{"laser"}, Limit: 1, PerBookLimit: 1})
	if err != nil {
		t.Fatalf("QueryBooks rejected internal bundle link: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("QueryBooks returned %#v, want one result", response.Results)
	}
}

func TestQueryBooksStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := QueryBooks(ctx, t.TempDir(), nil, Query{
		Books: []string{"book"}, Tags: []string{"tag"}, Limit: 1, PerBookLimit: 1,
	}); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("QueryBooks error = %v, want context canceled", err)
	}
}

func TestQueryBooksSearchesAtMostMaxConcurrentBooks(t *testing.T) {
	var active, maximum int
	var mu sync.Mutex
	release := make(chan struct{})
	searcher := func(ctx context.Context, _, _ string, _ []string, _ int) ([]Result, error) {
		mu.Lock()
		active++
		if active > maximum {
			maximum = active
		}
		mu.Unlock()
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		mu.Lock()
		active--
		mu.Unlock()
		return nil, nil
	}
	books := []string{"a", "b", "c", "d", "e", "f"}
	dirs := make(map[string]string, len(books))
	for _, id := range books {
		dirs[id] = id
	}
	done := make(chan error, 1)
	go func() {
		_, err := queryValidatedBooks(context.Background(), dirs, books, []string{"tag"}, 1, 10, searcher)
		done <- err
	}()
	for {
		mu.Lock()
		currentMaximum := maximum
		mu.Unlock()
		if currentMaximum == maxConcurrentBookQueries {
			break
		}
		runtime.Gosched()
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if maximum > maxConcurrentBookQueries {
		t.Fatalf("maximum concurrency = %d, want <= %d", maximum, maxConcurrentBookQueries)
	}
}

func TestQueryValidatedBooksCancelsRemainingWorkOnFirstError(t *testing.T) {
	var started int
	var mu sync.Mutex
	var once sync.Once
	bothStarted := make(chan struct{})
	stopped := make(chan struct{}, 1)
	searcher := func(ctx context.Context, _, bookID string, _ []string, _ int) ([]Result, error) {
		mu.Lock()
		started++
		if started == 2 {
			once.Do(func() { close(bothStarted) })
		}
		mu.Unlock()
		if bookID == "bad" {
			<-bothStarted
			return nil, errors.New("bad book")
		}
		<-ctx.Done()
		stopped <- struct{}{}
		return nil, ctx.Err()
	}
	_, err := queryValidatedBooks(context.Background(), map[string]string{"bad": "bad", "slow": "slow"},
		[]string{"bad", "slow"}, []string{"tag"}, 1, 2, searcher)
	if err == nil || !strings.Contains(err.Error(), "bad book") {
		t.Fatalf("queryValidatedBooks error = %v, want bad book", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("remaining search was not canceled")
	}
}

func writeScoringBundle(t *testing.T, root, id string, position int64) string {
	t.Helper()
	manifest := validManifest(id)
	chunks := []ChunkRow{{ChunkID: "chunk-1", Title: "Title", Content: "Original", Position: position}}
	index := []IndexRow{
		{Keyword: "laser", ChunkID: "chunk-1", Fields: []string{"title", "content"}},
		{Keyword: "strength", ChunkID: "chunk-1", Fields: []string{"content", "quote"}},
	}
	idf := []IDFRow{
		{Keyword: "laser", IDF: 2, DocumentFrequency: 1},
		{Keyword: "strength", IDF: 4, DocumentFrequency: 1},
	}
	return writeBundleFixture(t, root, manifest, chunks, index, idf)
}

func writeTieBundle(t *testing.T, root, id string, positions []int64) string {
	t.Helper()
	manifest := validManifest(id)
	manifest.ChunkCount = int64(len(positions))
	chunks := make([]ChunkRow, 0, len(positions))
	index := make([]IndexRow, 0, len(positions))
	for _, position := range positions {
		chunkID := "chunk-" + string(rune('0'+position))
		chunks = append(chunks, ChunkRow{ChunkID: chunkID, Content: "content", Position: position})
		index = append(index, IndexRow{Keyword: "tag", ChunkID: chunkID, Fields: []string{"content"}})
	}
	idf := []IDFRow{{Keyword: "tag", IDF: 1, DocumentFrequency: int64(len(chunks))}}
	return writeBundleFixture(t, root, manifest, chunks, index, idf)
}
