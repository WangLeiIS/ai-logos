package e2e

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"logos/builder"
	"logos/db"
	"logos/e2e/testenv"
	"logos/store"
)

// packToZip walks srcDir and writes all files to a ZIP archive at zipPath.
func packToZip(srcDir, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)
	defer w.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(srcDir, path)
		if rel == "." {
			return nil
		}

		if info.IsDir() {
			_, err := w.Create(rel + "/")
			return err
		}

		wr, err := w.Create(rel)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(wr, f)
		return err
	})
}

// writeSchemaSQL writes a minimal CREATE TABLE metadata SQL file into dir,
// returning the filename.
func writeSchemaSQL(t *testing.T, dir string) string {
	t.Helper()
	filename := "schema.sql"
	content := `CREATE TABLE IF NOT EXISTS metadata (
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    remark TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);`
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return filename
}

// TestBuildCreatesValidIroll verifies that a build succeeds and produces a
// valid iroll with a database containing metadata, books, and skills, plus a
// layer.json with schema_version=1.
func TestBuildCreatesValidIroll(t *testing.T) {
	env := testenv.New(t)

	result, err := env.Build("test-agent")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if result.Name != "test-agent" {
		t.Fatalf("result.Name = %q, want %q", result.Name, "test-agent")
	}
	if result.LayerID == "" {
		t.Fatal("result.LayerID is empty")
	}

	// Verify DB exists with metadata
	conn, err := env.DB("test-agent")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}
	meta, err := db.QueryAllMetadata(conn)
	if err != nil {
		t.Fatalf("QueryAllMetadata returned error: %v", err)
	}
	if meta["name"] != "base-cat" {
		t.Fatalf("metadata[name] = %q, want %q", meta["name"], "base-cat")
	}
	if _, ok := meta["description"]; !ok {
		t.Fatal("metadata missing 'description' key")
	}
	if _, ok := meta["version"]; !ok {
		t.Fatal("metadata missing 'version' key")
	}

	// Verify books
	books, err := db.ListBooks(conn)
	if err != nil {
		t.Fatalf("ListBooks returned error: %v", err)
	}
	if len(books) == 0 {
		t.Fatal("ListBooks returned no books, want at least one")
	}

	// Verify layer.json exists with schema_version=1
	layerJSONPath := filepath.Join(result.Path, "layer.json")
	data, err := os.ReadFile(layerJSONPath)
	if err != nil {
		t.Fatalf("reading layer.json: %v", err)
	}
	var lj builder.LayerJSON
	if err := json.Unmarshal(data, &lj); err != nil {
		t.Fatalf("parsing layer.json: %v", err)
	}
	if lj.SchemaVersion != 2 {
		t.Fatalf("layer.json schema_version = %d, want 2", lj.SchemaVersion)
	}
	if lj.LayerID == "" {
		t.Fatal("layer.json layer_id is empty")
	}
}

// TestBuildFromInheritsBaseLayer verifies that a child built with FROM
// inherits metadata and books from the base layer, and that layer.json parent
// matches the base LayerID.
func TestBuildFromInheritsBaseLayer(t *testing.T) {
	env := testenv.New(t)

	// Build base
	baseResult, err := env.Build("base-agent")
	if err != nil {
		t.Fatalf("Build base-agent returned error: %v", err)
	}

	// Create child Irollfile with FROM base-agent and a MIGRATE
	childDir := t.TempDir()
	schemaFile := writeSchemaSQL(t, childDir)
	childLF := "FROM base-agent\nMIGRATE " + schemaFile + "\n"
	childLFPath := filepath.Join(childDir, "Irollfile")
	if err := os.WriteFile(childLFPath, []byte(childLF), 0644); err != nil {
		t.Fatal(err)
	}

	lf, err := builder.ParseIrollfile(childLFPath)
	if err != nil {
		t.Fatalf("ParseIrollfile child returned error: %v", err)
	}

	childResult, err := builder.Build(lf, "child-agent", "latest")
	if err != nil {
		t.Fatalf("Build child-agent returned error: %v", err)
	}

	// Child inherits metadata from base
	conn, err := env.DB("child-agent")
	if err != nil {
		t.Fatalf("env.DB child returned error: %v", err)
	}
	meta, err := db.QueryAllMetadata(conn)
	if err != nil {
		t.Fatalf("QueryAllMetadata child returned error: %v", err)
	}
	if meta["name"] != "base-cat" {
		t.Fatalf("child metadata[name] = %q, want %q (inherited from base)", meta["name"], "base-cat")
	}

	// Child inherits books from base
	books, err := db.ListBooks(conn)
	if err != nil {
		t.Fatalf("ListBooks child returned error: %v", err)
	}
	if len(books) == 0 {
		t.Fatal("child ListBooks returned no books, want inherited books from base")
	}

	// layer.json parent matches base LayerID
	layerJSONPath := filepath.Join(childResult.Path, "layer.json")
	data, err := os.ReadFile(layerJSONPath)
	if err != nil {
		t.Fatalf("reading child layer.json: %v", err)
	}
	var lj builder.LayerJSON
	if err := json.Unmarshal(data, &lj); err != nil {
		t.Fatalf("parsing child layer.json: %v", err)
	}
	if lj.Parent != baseResult.LayerID {
		t.Fatalf("child layer.json parent = %q, want base LayerID %q", lj.Parent, baseResult.LayerID)
	}
}

// TestBuildRejectsInvalidTag verifies that Build rejects empty, "..", "a/b",
// and "a\b" tag names.
func TestBuildRejectsInvalidTag(t *testing.T) {
	env := testenv.New(t)

	invalidTags := []string{"", "..", "a/b", `a\b`}
	for _, tag := range invalidTags {
		_, err := env.Build(tag)
		if err == nil {
			t.Errorf("Build(%q) returned nil error, want rejection", tag)
		}
	}
}

// TestListShowsBuiltIroll verifies that store.List does not include an agent
// before build but does include it after.
func TestListShowsBuiltIroll(t *testing.T) {
	env := testenv.New(t)

	// Before build, list should not contain "my-agent"
	names, err := store.List()
	if err != nil {
		t.Fatalf("List before build returned error: %v", err)
	}
	for _, n := range names {
		if n == "my-agent:latest" {
			t.Fatal("List before build already contains 'my-agent'")
		}
	}

	// Build
	if _, err := env.Build("my-agent"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	// After build, list should contain "my-agent"
	names, err = store.List()
	if err != nil {
		t.Fatalf("List after build returned error: %v", err)
	}
	found := false
	for _, n := range names {
		if n == "my-agent:latest" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("List after build = %v, want 'my-agent:latest' included", names)
	}
}

// TestHistoryRecordsBuild verifies that QueryHistory returns an entry with an
// empty FromLayer and a valid LayerID for a fresh build.
func TestHistoryRecordsBuild(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("hist-agent"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("hist-agent")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	entries, err := db.QueryHistory(conn)
	if err != nil {
		t.Fatalf("QueryHistory returned error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("QueryHistory returned no entries")
	}

	first := entries[0]
	if first.FromLayer != "" {
		t.Fatalf("first history entry FromLayer = %q, want empty string", first.FromLayer)
	}
	if first.LayerID == "" {
		t.Fatal("first history entry LayerID is empty")
	}
}

// TestSaveAndLoadRoundTrip verifies that building an iroll, packing it to ZIP,
// and extracting it under a new name produces matching metadata.
func TestSaveAndLoadRoundTrip(t *testing.T) {
	env := testenv.New(t)

	// Build original
	result, err := env.Build("original")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	// Open original DB and read metadata
	origConn, err := env.DB("original")
	if err != nil {
		t.Fatalf("env.DB original returned error: %v", err)
	}
	origMeta, err := db.QueryAllMetadata(origConn)
	if err != nil {
		t.Fatalf("QueryAllMetadata original returned error: %v", err)
	}

	// Pack to ZIP
	zipPath := filepath.Join(t.TempDir(), "original.iroll")
	if err := packToZip(result.Path, zipPath); err != nil {
		t.Fatalf("packToZip returned error: %v", err)
	}

	// Extract under new name
	if err := store.Extract(zipPath, "loaded", "latest"); err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	// Open loaded DB and verify metadata matches
	loadedConn, err := env.DB("loaded")
	if err != nil {
		t.Fatalf("env.DB loaded returned error: %v", err)
	}
	loadedMeta, err := db.QueryAllMetadata(loadedConn)
	if err != nil {
		t.Fatalf("QueryAllMetadata loaded returned error: %v", err)
	}

	for k, wantV := range origMeta {
		gotV, ok := loadedMeta[k]
		if !ok {
			t.Errorf("loaded metadata missing key %q", k)
			continue
		}
		if gotV != wantV {
			t.Errorf("loaded metadata[%q] = %q, want %q", k, gotV, wantV)
		}
	}
}
