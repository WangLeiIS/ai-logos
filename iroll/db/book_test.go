package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"logos/book"
)

func TestSyncBooksCreatesAndInserts(t *testing.T) {
	conn := openBookTestDB(t)
	bundle := testBundle("book-b", "Book B")

	if err := SyncBooks(conn, []book.Bundle{bundle}); err != nil {
		t.Fatalf("SyncBooks returned error: %v", err)
	}

	got, err := GetBook(conn, bundle.Manifest.BookID)
	if err != nil {
		t.Fatalf("GetBook returned error: %v", err)
	}
	if got.Title != "Book B" || got.ResourcePath != "Resources/books/book-b" {
		t.Fatalf("GetBook = %#v", got)
	}
	if !equalStrings(got.Authors, []string{"Author One"}) || !equalStrings(got.Tags, []string{"tag-one"}) {
		t.Fatalf("JSON arrays were not preserved: %#v", got)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Fatalf("timestamps must be populated: %#v", got)
	}
}

func TestSyncBooksUpdatesManifestMetadata(t *testing.T) {
	conn := openBookTestDB(t)
	bundle := testBundle("book-a", "Old title")
	if err := SyncBooks(conn, []book.Bundle{bundle}); err != nil {
		t.Fatal(err)
	}
	before, err := GetBook(conn, "book-a")
	if err != nil {
		t.Fatal(err)
	}

	bundle.Manifest.Title = "New title"
	bundle.Manifest.Description = "New description"
	bundle.Manifest.Authors = []string{"New Author"}
	bundle.Manifest.Tags = []string{"new-tag"}
	if err := SyncBooks(conn, []book.Bundle{bundle}); err != nil {
		t.Fatal(err)
	}
	after, err := GetBook(conn, "book-a")
	if err != nil {
		t.Fatal(err)
	}

	if after.Title != "New title" || after.Description != "New description" {
		t.Fatalf("metadata was not updated: %#v", after)
	}
	if before.CreatedAt != after.CreatedAt {
		t.Fatalf("created_at changed: before %q after %q", before.CreatedAt, after.CreatedAt)
	}
}

func TestSyncBooksDeletesMissingBooks(t *testing.T) {
	conn := openBookTestDB(t)
	if err := SyncBooks(conn, []book.Bundle{
		testBundle("book-a", "Book A"),
		testBundle("book-b", "Book B"),
	}); err != nil {
		t.Fatal(err)
	}

	if err := SyncBooks(conn, []book.Bundle{testBundle("book-b", "Book B")}); err != nil {
		t.Fatal(err)
	}

	books, err := ListBooks(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].BookID != "book-b" {
		t.Fatalf("ListBooks = %#v, want only book-b", books)
	}
}

func TestListBooksReturnsStableOrder(t *testing.T) {
	conn := openBookTestDB(t)
	if err := SyncBooks(conn, []book.Bundle{
		testBundle("book-z", "Same"),
		testBundle("book-a", "Same"),
	}); err != nil {
		t.Fatal(err)
	}

	books, err := ListBooks(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 || books[0].BookID != "book-a" || books[1].BookID != "book-z" {
		t.Fatalf("ListBooks order = %#v", books)
	}
}

func TestGetBookReturnsRegisteredBook(t *testing.T) {
	conn := openBookTestDB(t)
	want := testBundle("book-a", "Book A")
	if err := SyncBooks(conn, []book.Bundle{want}); err != nil {
		t.Fatal(err)
	}

	got, err := GetBook(conn, "book-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.BookID != want.Manifest.BookID || got.FormatVersion != want.Manifest.FormatVersion {
		t.Fatalf("GetBook = %#v", got)
	}
}

func TestSyncBooksRollsBackAllDeletionsWhenLaterDeleteFails(t *testing.T) {
	conn := openBookTestDB(t)
	if err := SyncBooks(conn, []book.Bundle{
		testBundle("delete-first", "Delete First"),
		testBundle("reject-delete", "Reject Delete"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		CREATE TRIGGER reject_later_delete
		BEFORE DELETE ON book
		WHEN OLD.book_id = 'reject-delete'
		  AND NOT EXISTS (SELECT 1 FROM book WHERE book_id = 'delete-first')
		BEGIN
			SELECT RAISE(FAIL, 'rejected after prior delete');
		END
	`); err != nil {
		t.Fatal(err)
	}

	err := SyncBooks(conn, nil)
	if err == nil {
		t.Fatal("SyncBooks returned nil error")
	}
	if !strings.Contains(err.Error(), "rejected after prior delete") {
		t.Fatalf("SyncBooks failed before the first delete: %v", err)
	}
	books, err := ListBooks(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 || books[0].BookID != "delete-first" || books[1].BookID != "reject-delete" {
		t.Fatalf("deletions were not rolled back: %#v", books)
	}
}

func openBookTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "ai_roll.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func testBundle(id, title string) book.Bundle {
	return book.Bundle{
		ResourcePath: "Resources/books/" + id,
		Manifest: book.Manifest{
			Format:        book.FormatV1,
			FormatVersion: book.FormatVersion1,
			BookID:        id,
			Title:         title,
			Description:   "Description",
			Authors:       []string{"Author One"},
			Language:      "en",
			Tags:          []string{"tag-one"},
			SearchEngine:  book.SearchEngineV1,
		},
	}
}

func equalStrings(a, b []string) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}
