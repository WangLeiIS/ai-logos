package builder

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"logos/book"
	"logos/db"

	"github.com/parquet-go/parquet-go"
)

func TestProcessCopyRejectsSourceTraversal(t *testing.T) {
	parent := t.TempDir()
	layerDir := filepath.Join(parent, "layer")
	buildDir := filepath.Join(parent, "build")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	err := processCopy(buildDir, layerDir, filepath.Join("..", "secret.txt"), filepath.Join("Resources", "secret.txt"))
	if err == nil {
		t.Fatal("processCopy returned nil error for source traversal")
	}
}

func TestProcessCopyRejectsDestinationTraversal(t *testing.T) {
	parent := t.TempDir()
	layerDir := filepath.Join(parent, "layer")
	buildDir := filepath.Join(parent, "build")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layerDir, "safe.txt"), []byte("safe"), 0644); err != nil {
		t.Fatal(err)
	}

	err := processCopy(buildDir, layerDir, "safe.txt", filepath.Join("..", "escaped.txt"))
	if err == nil {
		t.Fatal("processCopy returned nil error for destination traversal")
	}
}

func TestBuildRejectsUnsafeTagName(t *testing.T) {
	lf := &Layerfile{Dir: t.TempDir()}

	if _, err := Build(lf, "../escaped"); err == nil {
		t.Fatal("Build returned nil error for unsafe tag name")
	}
}

func TestProcessFromRejectsUnsafeBaseName(t *testing.T) {
	if _, err := processFrom(t.TempDir(), "../base"); err == nil {
		t.Fatal("processFrom returned nil error for unsafe base name")
	}
}

func TestProcessMigrateRejectsTraversal(t *testing.T) {
	parent := t.TempDir()
	layerDir := filepath.Join(parent, "layer")
	buildDir := filepath.Join(parent, "build")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "migration.sql"), []byte("SELECT 1;"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := processMigrate(buildDir, layerDir, filepath.Join("..", "migration.sql")); err == nil {
		t.Fatal("processMigrate returned nil error for traversal")
	}
}

func TestBuildRegistersValidBooks(t *testing.T) {
	home := isolatedBuildHome(t)
	layerDir := t.TempDir()
	writeBuilderBook(t, filepath.Join(layerDir, "books"), "valid-book", true)
	lf := &Layerfile{
		Dir: layerDir,
		Instructions: []Instruction{{
			Type: InstCopy,
			Args: []string{"books", "Resources/books"},
		}},
	}

	result, err := Build(lf, "valid-roll")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if result.Path != filepath.Join(home, ".iroll", "valid-roll") {
		t.Fatalf("result path = %q", result.Path)
	}
	conn, err := sql.Open("sqlite3", filepath.Join(result.Path, "ai_roll.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	got, err := db.GetBook(conn, "valid-book")
	if err != nil {
		t.Fatalf("GetBook returned error: %v", err)
	}
	if got.Title != "Valid Book" {
		t.Fatalf("registered book = %#v", got)
	}
}

func TestBuildFailsForInvalidBookBundle(t *testing.T) {
	home := isolatedBuildHome(t)
	layerDir := t.TempDir()
	writeBuilderBook(t, filepath.Join(layerDir, "books"), "invalid-book", false)
	lf := &Layerfile{
		Dir: layerDir,
		Instructions: []Instruction{{
			Type: InstCopy,
			Args: []string{"books", "Resources/books"},
		}},
	}

	if _, err := Build(lf, "invalid-roll"); err == nil {
		t.Fatal("Build returned nil error for invalid bundle")
	}
	if _, err := os.Stat(filepath.Join(home, ".iroll", "invalid-roll")); !os.IsNotExist(err) {
		t.Fatalf("invalid build entered store: %v", err)
	}
}

func TestBuildRemovesInheritedBookRegistrationWhenResourceIsRemoved(t *testing.T) {
	home := isolatedBuildHome(t)
	baseDir := filepath.Join(home, ".iroll", "base-roll")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite3", filepath.Join(baseDir, "ai_roll.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SyncBooks(conn, []book.Bundle{{
		ResourcePath: "Resources/books/missing-book",
		Manifest: book.Manifest{
			Format: book.FormatV1, FormatVersion: book.FormatVersion1,
			BookID: "missing-book", Title: "Missing", SearchEngine: book.SearchEngineV1,
		},
	}}); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	lf := &Layerfile{Dir: t.TempDir(), Instructions: []Instruction{{
		Type: InstFrom,
		Args: []string{"base-roll"},
	}}}

	result, err := Build(lf, "child-roll")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	child, err := sql.Open("sqlite3", filepath.Join(result.Path, "ai_roll.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	books, err := db.ListBooks(child)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 0 {
		t.Fatalf("inherited stale books remain: %#v", books)
	}
}

func TestBuildCheckpointsWALBeforeCopyingToStore(t *testing.T) {
	isolatedBuildHome(t)
	layerDir := t.TempDir()
	writeBuilderBook(t, filepath.Join(layerDir, "books"), "wal-book", true)
	if err := os.WriteFile(filepath.Join(layerDir, "wal.sql"), []byte(`
		PRAGMA journal_mode=WAL;
		CREATE TABLE wal_marker (value TEXT NOT NULL);
		INSERT INTO wal_marker (value) VALUES ('persisted');
	`), 0644); err != nil {
		t.Fatal(err)
	}
	lf := &Layerfile{
		Dir: layerDir,
		Instructions: []Instruction{
			{Type: InstMigrate, Args: []string{"wal.sql"}},
			{Type: InstCopy, Args: []string{"books", "Resources/books"}},
		},
	}

	result, err := Build(lf, "wal-roll")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "ai_roll.db-wal")); !os.IsNotExist(err) {
		t.Fatalf("stored roll contains WAL sidecar: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "ai_roll.db-shm")); !os.IsNotExist(err) {
		t.Fatalf("stored roll contains SHM sidecar: %v", err)
	}

	conn, err := sql.Open("sqlite3", filepath.Join(result.Path, "ai_roll.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var marker string
	if err := conn.QueryRow("SELECT value FROM wal_marker").Scan(&marker); err != nil {
		t.Fatalf("read WAL marker: %v", err)
	}
	if marker != "persisted" {
		t.Fatalf("marker = %q", marker)
	}
	if _, err := db.GetBook(conn, "wal-book"); err != nil {
		t.Fatalf("book registration missing from copied database: %v", err)
	}
	history, err := db.QueryHistory(conn)
	if err != nil {
		t.Fatalf("history missing from copied database: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history = %#v, want one entry", history)
	}
}

func isolatedBuildHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func writeBuilderBook(t *testing.T, booksDir, id string, valid bool) {
	t.Helper()
	dir := filepath.Join(booksDir, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := book.Manifest{
		Format: book.FormatV1, FormatVersion: book.FormatVersion1,
		BookID: id, Title: "Valid Book", ChunkCount: 1, SearchEngine: book.SearchEngineV1,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if !valid {
		return
	}
	if err := parquet.WriteFile(filepath.Join(dir, "chunks.parquet"), []book.ChunkRow{{
		ChunkID: "chunk-1", Content: "Original content", Position: 0,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := parquet.WriteFile(filepath.Join(dir, "inverted_index.parquet"), []book.IndexRow{{
		Keyword: "laser", ChunkID: "chunk-1", Fields: []string{"content"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := parquet.WriteFile(filepath.Join(dir, "idf_stats.parquet"), []book.IDFRow{{
		Keyword: "laser", IDF: 1, DocumentFrequency: 1,
	}}); err != nil {
		t.Fatal(err)
	}
}
