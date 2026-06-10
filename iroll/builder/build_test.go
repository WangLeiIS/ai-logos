package builder

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestBuildBaseAgentContainsLoopSchema(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	layerfile, err := ParseLayerfile(filepath.Join("..", "..", "examples", "base-agent", "Layerfile"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Build(layerfile, "loop-schema-test")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite3", filepath.Join(result.Path, "ai_roll.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	var foreignKeysEnabled int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
		t.Fatal(err)
	}
	if foreignKeysEnabled != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeysEnabled)
	}

	assertTableColumns(t, conn, "loop", []string{
		"id", "name", "describe", "content", "weight", "archived_at", "created_at", "updated_at",
	})
	assertTableColumns(t, conn, "loop_runs", []string{
		"id", "loop_id", "page_id", "parent_run_id",
		"seed_name", "seed_describe", "seed_content", "seed_weight",
		"status", "plan", "progress", "result", "reflection", "abort_reason",
		"started_at", "ended_at", "reflected_at", "updated_at",
	})
	assertIndexExists(t, conn, "idx_loop_runs_page_status")
	assertIndexExists(t, conn, "idx_loop_runs_parent_status")
	assertIndexExists(t, conn, "idx_loop_runs_loop_started")
	assertIndexColumns(t, conn, "idx_loop_runs_loop_started", []string{"loop_id ASC", "id DESC"})
	assertIndexExists(t, conn, "idx_loop_runs_loop_ended")
	assertIndexColumns(t, conn, "idx_loop_runs_loop_ended", []string{"loop_id ASC", "ended_at DESC", "id DESC"})
	assertIndexSQLContains(t, conn, "idx_loop_runs_loop_ended", "ended_at IS NOT NULL")
	assertIndexExists(t, conn, "idx_loop_runs_one_active_main")

	assertExecFails(t, conn, `
		INSERT INTO loop (name, describe, content, weight, created_at, updated_at)
		VALUES ('invalid-weight', 'invalid weight', 'invalid weight', 1.1, datetime('now'), datetime('now'))
	`)

	mainRunID := insertLoopRun(t, conn, 1, "page-one", nil, "active")
	insertLoopRun(t, conn, 1, "page-one", nil, "completed")
	insertLoopRun(t, conn, 1, "page-one", mainRunID, "active")
	assertQueryUsesIndex(t, conn, `
		SELECT id
		FROM loop_runs
		WHERE loop_id = 1
			AND status IN ('completed', 'aborted')
			AND ended_at IS NOT NULL
		ORDER BY ended_at DESC, id DESC
		LIMIT 1
	`, "idx_loop_runs_loop_ended")

	assertLoopRunInsertFails(t, conn, 1, "page-one", nil, "active")
	assertLoopRunInsertFails(t, conn, 1, "page-two", nil, "pending")
	assertLoopRunInsertFails(t, conn, 9999, "page-two", nil, "active")
	assertLoopRunInsertFails(t, conn, 1, "page-two", int64(9999), "active")
}

func assertTableColumns(t *testing.T, conn *sql.DB, table string, want []string) {
	t.Helper()
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s columns = %#v, want %#v", table, got, want)
	}
}

func assertIndexExists(t *testing.T, conn *sql.DB, name string) {
	t.Helper()
	var count int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("index %q does not exist", name)
	}
}

func assertIndexColumns(t *testing.T, conn *sql.DB, name string, want []string) {
	t.Helper()
	rows, err := conn.Query(`PRAGMA index_xinfo(` + name + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var seqno, cid, desc, collKey int
		var columnName, collation sql.NullString
		if err := rows.Scan(&seqno, &cid, &columnName, &desc, &collation, &collKey); err != nil {
			t.Fatal(err)
		}
		if collKey == 1 && cid >= 0 {
			direction := "ASC"
			if desc != 0 {
				direction = "DESC"
			}
			got = append(got, columnName.String+" "+direction)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("index %q columns = %#v, want %#v", name, got, want)
	}
}

func assertIndexSQLContains(t *testing.T, conn *sql.DB, name, want string) {
	t.Helper()
	var sqlText string
	if err := conn.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, name,
	).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, want) {
		t.Fatalf("index %q SQL = %q, want %q", name, sqlText, want)
	}
}

func assertQueryUsesIndex(t *testing.T, conn *sql.DB, query, index string) {
	t.Helper()
	rows, err := conn.Query("EXPLAIN QUERY PLAN " + query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(details, "\n")
	if !strings.Contains(plan, index) || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("query plan = %q, want index %q without temp sort", plan, index)
	}
}

func insertLoopRun(t *testing.T, conn *sql.DB, loopID int64, pageID string, parentRunID any, status string) int64 {
	t.Helper()
	result, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, parent_run_id,
			seed_name, seed_describe, seed_content, seed_weight,
			status, started_at, updated_at
		) VALUES (?, ?, ?, 'seed', 'seed', 'seed', 0.5, ?, datetime('now'), datetime('now'))
	`, loopID, pageID, parentRunID, status)
	if err != nil {
		t.Fatalf("insert loop run: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertLoopRunInsertFails(t *testing.T, conn *sql.DB, loopID int64, pageID string, parentRunID any, status string) {
	t.Helper()
	_, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, parent_run_id,
			seed_name, seed_describe, seed_content, seed_weight,
			status, started_at, updated_at
		) VALUES (?, ?, ?, 'seed', 'seed', 'seed', 0.5, ?, datetime('now'), datetime('now'))
	`, loopID, pageID, parentRunID, status)
	if err == nil {
		t.Fatalf("invalid loop run insert succeeded: loop_id=%d page_id=%q parent_run_id=%v status=%q",
			loopID, pageID, parentRunID, status)
	}
}

func assertExecFails(t *testing.T, conn *sql.DB, query string) {
	t.Helper()
	if _, err := conn.Exec(query); err == nil {
		t.Fatal("invalid SQL execution succeeded")
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
		ChunkID: "chunk-1", BookID: id, Content: "Original content", SeqNum: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := parquet.WriteFile(filepath.Join(dir, "inverted_index.parquet"), []book.IndexRow{{
		ID: "idx-1", Keyword: "laser", ChunkID: "chunk-1", FieldType: "content",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := parquet.WriteFile(filepath.Join(dir, "idf_stats.parquet"), []book.IDFRow{{
		Keyword: "laser", IDF: 1, DF: 1,
	}}); err != nil {
		t.Fatal(err)
	}
}
