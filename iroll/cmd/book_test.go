package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"logos/book"
	"logos/db"
	"logos/store"

	"github.com/parquet-go/parquet-go"
)

func TestRunBookListUsesActiveIroll(t *testing.T) {
	cwd, _ := setupBookCommandTest(t)
	got, err := runBookList(cwd, nil)
	if err != nil {
		t.Fatalf("runBookList: %v", err)
	}
	if len(got) != 1 || got[0].BookID != "book-one" {
		t.Fatalf("runBookList() = %#v, want registered book-one", got)
	}
}

func TestRunBookInspectRejectsUnknownBook(t *testing.T) {
	cwd, _ := setupBookCommandTest(t)
	_, err := runBookInspect(cwd, "missing", nil)
	if err == nil || !strings.Contains(err.Error(), `get book "missing"`) {
		t.Fatalf("runBookInspect error = %v, want unknown book error", err)
	}
}

func TestRunBookQueryRejectsInvalidArguments(t *testing.T) {
	cwd, _ := setupBookCommandTest(t)
	tests := []book.Query{
		{Tags: []string{"tag"}, Limit: 10, PerBookLimit: 5},
		{Books: []string{"book-one"}, Limit: 10, PerBookLimit: 5},
		{Books: []string{"book-one"}, Tags: []string{"tag"}, Limit: 0, PerBookLimit: 5},
		{Books: []string{"book-one"}, Tags: []string{"tag"}, Limit: 10, PerBookLimit: 0},
	}
	for _, query := range tests {
		if _, err := runBookQuery(context.Background(), cwd, query); err == nil {
			t.Fatalf("runBookQuery accepted invalid query %#v", query)
		}
	}
}

func TestRunBookQueryValidatesArgumentsBeforeResolvingActiveIroll(t *testing.T) {
	tests := []struct {
		query book.Query
		want  string
	}{
		{query: book.Query{Tags: []string{"tag"}, Limit: 10, PerBookLimit: 5}, want: "at least one book"},
		{query: book.Query{Books: []string{"book-one"}, Limit: 10, PerBookLimit: 5}, want: "at least one non-empty tag"},
		{query: book.Query{Books: []string{"book-one"}, Tags: []string{"tag"}, Limit: 0, PerBookLimit: 5}, want: "query limit must be positive"},
		{query: book.Query{Books: []string{"book-one"}, Tags: []string{"tag"}, Limit: 10, PerBookLimit: 0}, want: "per-book limit must be positive"},
	}
	for _, test := range tests {
		_, err := runBookQuery(context.Background(), t.TempDir(), test.query)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("runBookQuery error = %v, want %q", err, test.want)
		}
	}
}

func TestRunBookQueryReturnsScoringDetails(t *testing.T) {
	cwd, rollRoot := setupBookCommandTest(t)
	writeCommandBookBundle(t, rollRoot)

	got, err := runBookQuery(context.Background(), cwd, book.Query{
		Books: []string{"book-one"}, Tags: []string{" LASER ", "laser"}, Limit: 10, PerBookLimit: 5,
	})
	if err != nil {
		t.Fatalf("runBookQuery: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one result", got.Results)
	}
	result := got.Results[0]
	if result.Content != "Original content" || result.Score != 30 ||
		result.TitleCoverage != 1 || result.ContentCoverage != 1 || result.AvgIDF != 2 {
		t.Fatalf("result = %#v, want original content and scoring details", result)
	}
	if len(got.QueryTags) != 1 || got.QueryTags[0] != "laser" {
		t.Fatalf("query tags = %#v, want normalized deduplicated tag", got.QueryTags)
	}
}

func setupBookCommandTest(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	cwd := filepath.Join(t.TempDir(), "workspace")
	rollName := "test-roll"
	rollRoot, err := store.IrollPath(rollName, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rollRoot, 0755); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(filepath.Join(rollRoot, "roll-inner.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.SyncBooks(conn, []book.Bundle{{
		ResourcePath: "Resources/books/book-one",
		Manifest: book.Manifest{
			BookID: "book-one", Title: "Book One", FormatVersion: 1,
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexPage(rollName, "latest", "page-one", cwd, "", ""); err != nil {
		t.Fatal(err)
	}
	return cwd, rollRoot
}

func writeCommandBookBundle(t *testing.T, rollRoot string) {
	t.Helper()
	dir := filepath.Join(rollRoot, "Resources", "books", "book-one")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := book.Manifest{
		Format: book.FormatV1, FormatVersion: book.FormatVersion1, BookID: "book-one",
		Title: "Book One", ChunkCount: 1, SearchEngine: book.SearchEngineV1,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := parquet.WriteFile(filepath.Join(dir, "chunks.parquet"), []book.ChunkRow{{
		ChunkID: "chunk-one", BookID: "book-one", Summary: "Laser", Content: "Original content", SeqNum: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := parquet.WriteFile(filepath.Join(dir, "inverted_index.parquet"), []book.IndexRow{{
		ID: "idx-title", Keyword: "laser", ChunkID: "chunk-one", FieldType: "title",
	}, {
		ID: "idx-content", Keyword: "laser", ChunkID: "chunk-one", FieldType: "content",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := parquet.WriteFile(filepath.Join(dir, "idf_stats.parquet"), []book.IDFRow{{
		Keyword: "laser", IDF: 2, DF: 1,
	}}); err != nil {
		t.Fatal(err)
	}
}
