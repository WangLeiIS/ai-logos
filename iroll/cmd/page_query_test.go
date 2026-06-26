package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"logos/db"
	"logos/store"
)

// setupPageQueryTest builds a roll + one cwd page (with a seeded memory row) and
// returns the cwd. The cwd outer path is registered via IndexPage so GetActive returns it.
func setupPageQueryTest(t *testing.T) string {
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

	// roll-inner.db with schema (keeps the roll well-formed; not opened by page query).
	innerConn, err := db.Open(filepath.Join(rollRoot, "roll-inner.db"))
	if err != nil {
		t.Fatal(err)
	}
	innerSchema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_inner.sql"))
	if err != nil {
		innerConn.Close()
		t.Fatal(err)
	}
	if _, err := innerConn.Exec(string(innerSchema)); err != nil {
		innerConn.Close()
		t.Fatal(err)
	}
	if err := innerConn.Close(); err != nil {
		t.Fatal(err)
	}

	// cwd outer db: create dir, open, apply schema, seed a memory row.
	outerPath, err := store.CwdOuterDbPath(cwd, rollName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(outerPath), 0755); err != nil {
		t.Fatal(err)
	}
	outerConn, err := db.Open(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	outerSchema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_outer.sql"))
	if err != nil {
		outerConn.Close()
		t.Fatal(err)
	}
	if _, err := outerConn.Exec(string(outerSchema)); err != nil {
		outerConn.Close()
		t.Fatal(err)
	}
	if _, err := outerConn.Exec(`INSERT INTO memory (page_id, name, question, content, importance, sleep_count, created_at, updated_at)
		VALUES ('page-one','m1','q?','c',0.5,0,datetime('now'),datetime('now'))`); err != nil {
		outerConn.Close()
		t.Fatal(err)
	}
	if err := outerConn.Close(); err != nil {
		t.Fatal(err)
	}

	// Register the page with its REAL outer path so resolveActiveOuter can find it.
	if err := store.IndexPage(rollName, "latest", "page-one", cwd, outerPath, ""); err != nil {
		t.Fatal(err)
	}
	return cwd
}

func TestPageQuerySelectMemory(t *testing.T) {
	cwd := setupPageQueryTest(t)

	origCwd := pageQueryTarget.cwd
	origSQL := pageQuerySQL
	pageQueryTarget.cwd = cwd
	pageQuerySQL = "SELECT name FROM memory WHERE page_id='page-one'"
	defer func() {
		pageQueryTarget.cwd = origCwd
		pageQuerySQL = origSQL
	}()

	outerPath, pageID := resolveActiveOuter("", "", cwd)
	if pageID != "page-one" {
		t.Fatalf("pageID = %q, want page-one", pageID)
	}
	conn, err := db.Open(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, pageQuerySQL, false)
	if err != nil {
		t.Fatalf("ExecuteAll: %v", err)
	}
	if len(results) != 1 || results[0].Type != "rows" || results[0].Count != 1 {
		t.Fatalf("results = %#v", results)
	}
}

func TestPageQueryCannotMutateInner(t *testing.T) {
	cwd := setupPageQueryTest(t)

	pageQueryTarget.cwd = cwd
	defer func() { pageQueryTarget.cwd = "" }()

	outerPath, _ := resolveActiveOuter("", "", cwd)
	conn, err := db.Open(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// inner is not attached to the standalone outer connection, so inner.* is unreachable.
	_, err = db.ExecuteAll(conn, "SELECT count(*) FROM inner.dna", false)
	if err == nil {
		t.Fatal("expected error: inner.* must not be reachable from page query (standalone outer)")
	}
}
