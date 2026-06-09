package book

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestAcceptsValidV1(t *testing.T) {
	dir := writeManifestFixture(t, validManifest("valid-book"), "valid-book")
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest returned error: %v", err)
	}
	if got.BookID != "valid-book" {
		t.Fatalf("BookID = %q, want valid-book", got.BookID)
	}
}

func TestLoadManifestRejectsMismatchedDirectoryID(t *testing.T) {
	dir := writeManifestFixture(t, validManifest("other-book"), "valid-book")
	if _, err := LoadManifest(dir); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("LoadManifest error = %v, want directory mismatch", err)
	}
}

func TestLoadManifestRejectsUnsupportedFormat(t *testing.T) {
	manifest := validManifest("valid-book")
	manifest.Format = "other"
	dir := writeManifestFixture(t, manifest, "valid-book")
	if _, err := LoadManifest(dir); err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("LoadManifest error = %v, want format error", err)
	}
}

func TestLoadManifestRejectsUnsafeBookID(t *testing.T) {
	manifest := validManifest("valid-book")
	manifest.BookID = "../bad"
	dir := writeManifestFixture(t, manifest, "valid-book")
	if _, err := LoadManifest(dir); err == nil || !strings.Contains(err.Error(), "book_id") {
		t.Fatalf("LoadManifest error = %v, want unsafe book_id error", err)
	}
}

func TestLoadManifestRejectsEmptyTitle(t *testing.T) {
	manifest := validManifest("valid-book")
	manifest.Title = " "
	dir := writeManifestFixture(t, manifest, "valid-book")
	if _, err := LoadManifest(dir); err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("LoadManifest error = %v, want title error", err)
	}
}

func TestLoadManifestRejectsManifestSymlinkEscapingBundle(t *testing.T) {
	dir := writeManifestFixture(t, validManifest("valid-book"), "valid-book")
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "manifest.json")
	data, err := json.Marshal(validManifest("valid-book"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(dir, "manifest.json"))

	if _, err := LoadManifest(dir); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("LoadManifest error = %v, want symlink escape error", err)
	}
}

func TestLoadManifestAllowsManifestSymlinkWithinBundle(t *testing.T) {
	dir := writeManifestFixture(t, validManifest("valid-book"), "valid-book")
	actual := filepath.Join(dir, "actual-manifest.json")
	if err := os.Rename(filepath.Join(dir, "manifest.json"), actual); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, actual, filepath.Join(dir, "manifest.json"))

	if _, err := LoadManifest(dir); err != nil {
		t.Fatalf("LoadManifest rejected internal manifest symlink: %v", err)
	}
}

func validManifest(bookID string) Manifest {
	return Manifest{
		Format:        "logos-book",
		FormatVersion: 1,
		BookID:        bookID,
		Title:         "Book title",
		ChunkCount:    1,
		SearchEngine:  "keyword-coverage-idf-v1",
	}
}

func writeManifestFixture(t *testing.T, manifest Manifest, dirName string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
