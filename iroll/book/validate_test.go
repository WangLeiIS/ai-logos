package book

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateBundleAcceptsValidBundle(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	bundle, err := ValidateBundle(dir)
	if err != nil {
		t.Fatalf("ValidateBundle returned error: %v", err)
	}
	if bundle.Manifest.BookID != manifest.BookID || bundle.Dir != dir {
		t.Fatalf("ValidateBundle() = %#v", bundle)
	}
	if bundle.ResourcePath != "Resources/books/valid-book" {
		t.Fatalf("ResourcePath = %q, want Resources/books/valid-book", bundle.ResourcePath)
	}
}

func TestValidateBundleRejectsInvalidRowsAndRelationships(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest, *[]ChunkRow, *[]IndexRow, *[]IDFRow)
		want   string
	}{
		{"chunk count mismatch", func(m *Manifest, _ *[]ChunkRow, _ *[]IndexRow, _ *[]IDFRow) { m.ChunkCount = 2 }, "chunk_count"},
		{"duplicate chunk id", func(_ *Manifest, c *[]ChunkRow, _ *[]IndexRow, _ *[]IDFRow) { *c = append(*c, (*c)[0]) }, "duplicate chunk_id"},
		{"empty content", func(_ *Manifest, c *[]ChunkRow, _ *[]IndexRow, _ *[]IDFRow) { (*c)[0].Content = "" }, "content"},
		{"negative position", func(_ *Manifest, c *[]ChunkRow, _ *[]IndexRow, _ *[]IDFRow) { (*c)[0].Position = -1 }, "position"},
		{"unsafe source path", func(_ *Manifest, c *[]ChunkRow, _ *[]IndexRow, _ *[]IDFRow) { (*c)[0].SourcePath = "../source.md" }, "source_path"},
		{"unknown index chunk", func(_ *Manifest, _ *[]ChunkRow, i *[]IndexRow, _ *[]IDFRow) { (*i)[0].ChunkID = "missing" }, "unknown chunk"},
		{"duplicate index pair", func(_ *Manifest, _ *[]ChunkRow, i *[]IndexRow, _ *[]IDFRow) { *i = append(*i, (*i)[0]) }, "duplicate index"},
		{"empty index fields", func(_ *Manifest, _ *[]ChunkRow, i *[]IndexRow, _ *[]IDFRow) { (*i)[0].Fields = nil }, "fields"},
		{"invalid index field", func(_ *Manifest, _ *[]ChunkRow, i *[]IndexRow, _ *[]IDFRow) { (*i)[0].Fields = []string{"body"} }, "field"},
		{"non-normalized keyword", func(_ *Manifest, _ *[]ChunkRow, i *[]IndexRow, _ *[]IDFRow) { (*i)[0].Keyword = " Laser " }, "normalized"},
		{"missing idf", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { *d = nil }, "missing IDF"},
		{"duplicate idf", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { *d = append(*d, (*d)[0]) }, "duplicate IDF"},
		{"invalid idf", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { (*d)[0].IDF = math.Inf(1) }, "idf"},
		{"negative document frequency", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { (*d)[0].DocumentFrequency = -1 }, "document_frequency"},
		{"high document frequency", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { (*d)[0].DocumentFrequency = 2 }, "document_frequency"},
		{"mismatched document frequency", func(m *Manifest, c *[]ChunkRow, i *[]IndexRow, d *[]IDFRow) {
			*c = append(*c, ChunkRow{ChunkID: "chunk-2", Content: "content", Position: 1})
			m.ChunkCount = 2
			(*d)[0].DocumentFrequency = 0
		}, "document_frequency"},
		{"extra idf with nonzero document frequency", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) {
			*d = append(*d, IDFRow{Keyword: "extra", IDF: 1, DocumentFrequency: 1})
		}, "document_frequency"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest, chunks, index, idf := validBundleRows()
			test.mutate(&manifest, &chunks, &index, &idf)
			dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
			if _, err := ValidateBundle(dir); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateBundle error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDiscoverFindsSortedDirectChildBundles(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "Resources", "books")
	for _, id := range []string{"z-book", "a-book"} {
		manifest, chunks, index, idf := validBundleRows()
		manifest.BookID = id
		writeBundleFixture(t, booksDir, manifest, chunks, index, idf)
	}
	if err := os.WriteFile(filepath.Join(booksDir, "ignored.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got) != 2 || got[0].Manifest.BookID != "a-book" || got[1].Manifest.BookID != "z-book" {
		t.Fatalf("Discover() = %#v", got)
	}
}

func TestDiscoverRejectsInvalidChildBundle(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Resources", "books", "bad-book")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), "bad-book") {
		t.Fatalf("Discover error = %v, want bad-book error", err)
	}
}

func TestDiscoverReturnsEmptyWhenBooksDirectoryIsMissing(t *testing.T) {
	got, err := Discover(t.TempDir())
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Discover() = %#v, want empty", got)
	}
}

func TestValidateBundleRejectsSourcePathSymlinkEscapingBundle(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	chunks[0].SourcePath = "source.md"
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	outside := filepath.Join(t.TempDir(), "source.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(dir, "source.md"))

	if _, err := ValidateBundle(dir); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("ValidateBundle error = %v, want source_path escape error", err)
	}
}

func TestValidateBundleRejectsSourcePathJunctionEscapingBundle(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	chunks[0].SourcePath = "docs/source.md"
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "source.md"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(dir, "docs"))

	if _, err := ValidateBundle(dir); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("ValidateBundle error = %v, want source_path junction escape error", err)
	}
}

func TestValidateBundleAllowsSourcePathJunctionWithinBundle(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	chunks[0].SourcePath = "docs/source.md"
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	target := filepath.Join(dir, "actual-docs")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "source.md"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, target, filepath.Join(dir, "docs"))

	if _, err := ValidateBundle(dir); err != nil {
		t.Fatalf("ValidateBundle rejected internal source_path junction: %v", err)
	}
}

func TestValidateBundleRejectsMissingSourceBelowEscapingJunction(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	chunks[0].SourcePath = "docs/missing.md"
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	outside := t.TempDir()
	symlinkOrSkip(t, outside, filepath.Join(dir, "docs"))

	if _, err := ValidateBundle(dir); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("ValidateBundle error = %v, want missing source parent escape error", err)
	}
}

func TestValidateBundleAllowsBundleLink(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	container := t.TempDir()
	target := writeBundleFixture(t, container, manifest, chunks, index, idf)
	link := filepath.Join(container, "links", manifest.BookID)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, target, link)

	if _, err := ValidateBundle(link); err != nil {
		t.Fatalf("ValidateBundle rejected bundle link: %v", err)
	}
}

func TestDiscoverRejectsSymlinkBundle(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "Resources", "books")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, chunks, index, idf := validBundleRows()
	outside := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	symlinkOrSkip(t, outside, filepath.Join(booksDir, manifest.BookID))

	if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("Discover error = %v, want bundle escape error", err)
	}
}

func TestDiscoverAllowsBundleLinkWithinRollRoot(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "Resources", "books")
	targetsDir := filepath.Join(root, "Resources", "book-targets")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, chunks, index, idf := validBundleRows()
	target := writeBundleFixture(t, targetsDir, manifest, chunks, index, idf)
	symlinkOrSkip(t, target, filepath.Join(booksDir, manifest.BookID))

	bundles, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover rejected internal bundle link: %v", err)
	}
	if len(bundles) != 1 || bundles[0].Manifest.BookID != manifest.BookID {
		t.Fatalf("Discover() = %#v, want linked bundle", bundles)
	}
}
