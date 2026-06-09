package book

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
)

func TestReadTypedParquetFiles(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)

	gotChunks, err := ReadChunks(dir)
	if err != nil || len(gotChunks) != 1 || gotChunks[0].ChunkID != "chunk-1" {
		t.Fatalf("ReadChunks() = %#v, %v", gotChunks, err)
	}
	gotIndex, err := ReadIndex(dir)
	if err != nil || len(gotIndex) != 1 || gotIndex[0].Keyword != "laser" {
		t.Fatalf("ReadIndex() = %#v, %v", gotIndex, err)
	}
	gotIDF, err := ReadIDF(dir)
	if err != nil || len(gotIDF) != 1 || gotIDF[0].IDF != 2 {
		t.Fatalf("ReadIDF() = %#v, %v", gotIDF, err)
	}
}

func TestFastValidateRejectsMissingParquet(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	if err := os.Remove(filepath.Join(dir, "idf_stats.parquet")); err != nil {
		t.Fatal(err)
	}
	if _, err := FastValidate(dir); err == nil || !strings.Contains(err.Error(), "idf_stats.parquet") {
		t.Fatalf("FastValidate error = %v, want missing idf_stats.parquet", err)
	}
}

func TestFastValidateRejectsWrongSchema(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	type wrongRow struct {
		Other string `parquet:"other"`
	}
	if err := parquet.WriteFile(filepath.Join(dir, "chunks.parquet"), []wrongRow{{Other: "wrong"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := FastValidate(dir); err == nil {
		t.Fatal("FastValidate accepted incompatible chunks schema")
	}
}

func TestFastValidateRejectsParquetSymlinkEscapingBundle(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	outside := filepath.Join(t.TempDir(), chunksFile)
	if err := parquet.WriteFile(outside, chunks); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, chunksFile)); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(dir, chunksFile))

	if _, err := FastValidate(dir); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("FastValidate error = %v, want symlink escape error", err)
	}
}

func TestFastValidateAllowsBundleLinkWhenTargetContainsBundleFiles(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	container := t.TempDir()
	target := writeBundleFixture(t, container, manifest, chunks, index, idf)
	link := filepath.Join(container, "links", manifest.BookID)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, target, link)

	if _, err := FastValidate(link); err != nil {
		t.Fatalf("FastValidate rejected internal bundle link: %v", err)
	}
}

func TestFastValidateAllowsParquetSymlinkWithinBundle(t *testing.T) {
	manifest, chunks, index, idf := validBundleRows()
	dir := writeBundleFixture(t, t.TempDir(), manifest, chunks, index, idf)
	actual := filepath.Join(dir, "actual-chunks.parquet")
	if err := os.Rename(filepath.Join(dir, chunksFile), actual); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, actual, filepath.Join(dir, chunksFile))

	if _, err := FastValidate(dir); err != nil {
		t.Fatalf("FastValidate rejected internal parquet symlink: %v", err)
	}
}
