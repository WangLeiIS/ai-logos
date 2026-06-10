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
		{"wrong chunk book id", func(_ *Manifest, c *[]ChunkRow, _ *[]IndexRow, _ *[]IDFRow) { (*c)[0].BookID = "other-book" }, "book_id"},
		{"negative sequence", func(_ *Manifest, c *[]ChunkRow, _ *[]IndexRow, _ *[]IDFRow) { (*c)[0].SeqNum = -1 }, "seq_num"},
		{"unknown index chunk", func(_ *Manifest, _ *[]ChunkRow, i *[]IndexRow, _ *[]IDFRow) { (*i)[0].ChunkID = "missing" }, "unknown chunk"},
		{"duplicate index id", func(_ *Manifest, _ *[]ChunkRow, i *[]IndexRow, _ *[]IDFRow) { *i = append(*i, (*i)[0]) }, "duplicate index id"},
		{"empty index id", func(_ *Manifest, _ *[]ChunkRow, i *[]IndexRow, _ *[]IDFRow) { (*i)[0].ID = "" }, "index id"},
		{"invalid index field", func(_ *Manifest, _ *[]ChunkRow, i *[]IndexRow, _ *[]IDFRow) { (*i)[0].FieldType = "body" }, "field_type"},
		{"missing idf", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { *d = nil }, "missing IDF"},
		{"duplicate idf", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { *d = append(*d, (*d)[0]) }, "duplicate IDF"},
		{"invalid idf", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { (*d)[0].IDF = math.Inf(1) }, "idf"},
		{"negative document frequency", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { (*d)[0].DF = -1 }, "df"},
		{"high document frequency", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) { (*d)[0].DF = 2 }, "df"},
		{"mismatched document frequency", func(m *Manifest, c *[]ChunkRow, i *[]IndexRow, d *[]IDFRow) {
			*c = append(*c, ChunkRow{ChunkID: "chunk-2", BookID: m.BookID, Content: "content", SeqNum: 2})
			m.ChunkCount = 2
			(*d)[0].DF = 0
		}, "df"},
		{"extra idf with nonzero document frequency", func(_ *Manifest, _ *[]ChunkRow, _ *[]IndexRow, d *[]IDFRow) {
			*d = append(*d, IDFRow{Keyword: "extra", IDF: 1, DF: 1})
		}, "df"},
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
		chunks[0].BookID = id
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
