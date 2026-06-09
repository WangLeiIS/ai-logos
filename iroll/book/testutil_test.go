package book

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/parquet-go/parquet-go"
)

func writeBundleFixture(t *testing.T, root string, manifest Manifest, chunks []ChunkRow, index []IndexRow, idf []IDFRow) string {
	t.Helper()
	dir := filepath.Join(root, manifest.BookID)
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
	if err := parquet.WriteFile(filepath.Join(dir, "chunks.parquet"), chunks); err != nil {
		t.Fatal(err)
	}
	if err := parquet.WriteFile(filepath.Join(dir, "inverted_index.parquet"), index); err != nil {
		t.Fatal(err)
	}
	if err := parquet.WriteFile(filepath.Join(dir, "idf_stats.parquet"), idf); err != nil {
		t.Fatal(err)
	}
	return dir
}

func validBundleRows() (Manifest, []ChunkRow, []IndexRow, []IDFRow) {
	manifest := validManifest("valid-book")
	chunks := []ChunkRow{{
		ChunkID: "chunk-1", Title: "Title", Content: "Original content", SourcePath: "source.md", Position: 0,
	}}
	index := []IndexRow{{Keyword: "laser", ChunkID: "chunk-1", Fields: []string{"title", "content"}}}
	idf := []IDFRow{{Keyword: "laser", IDF: 2, DocumentFrequency: 1}}
	return manifest, chunks, index, idf
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err == nil {
		return
	} else if runtime.GOOS != "windows" {
		t.Skipf("symlinks unavailable: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Skipf("file symlinks unavailable: %v", err)
	}
	output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Skipf("directory links unavailable: %v: %s", err, output)
	}
}
