package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rolldb "logos/db"
)

func TestDeletePageCleansRollBeforeIndexAndClearsActiveMapping(t *testing.T) {
	conn, pageID, mainID, childID, cwd := setupDeletePageStoreTest(t)
	defer conn.Close()

	if err := DeletePage(pageID); err != nil {
		t.Fatal(err)
	}

	if _, err := rolldb.GetPageByPageID(conn, pageID); err == nil {
		t.Fatal("deleted page still exists in roll")
	}
	for _, runID := range []int64{mainID, childID} {
		run, err := rolldb.GetLoopRun(conn, runID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != "aborted" || run.AbortReason == nil || *run.AbortReason != "page_deleted" ||
			run.EndedAt == nil || run.UpdatedAt != *run.EndedAt {
			t.Fatalf("run after page deletion = %#v", run)
		}
	}
	assertPageIndexCount(t, pageID, 0, 0)
	if _, _, _, _, err := GetActive(cwd); err == nil {
		t.Fatal("active page mapping still exists")
	}
}

func TestDeletePageLeavesIndexIntactWhenRollCleanupFails(t *testing.T) {
	conn, pageID, mainID, childID, cwd := setupDeletePageStoreTest(t)
	defer conn.Close()
	if _, err := conn.Exec(`
		CREATE TRIGGER reject_page_delete
		BEFORE DELETE ON pages
		BEGIN
			SELECT RAISE(ABORT, 'page delete rejected');
		END
	`); err != nil {
		t.Fatal(err)
	}

	if err := DeletePage(pageID); err == nil || !strings.Contains(err.Error(), "page delete rejected") {
		t.Fatalf("DeletePage error = %v", err)
	}

	assertPageIndexCount(t, pageID, 1, 1)
	name, _, activePageID, _, err := GetActive(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if name != "test-roll" || activePageID != pageID {
		t.Fatalf("active mapping = %q %q", name, activePageID)
	}
	for _, runID := range []int64{mainID, childID} {
		run, err := rolldb.GetLoopRun(conn, runID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != "active" || run.EndedAt != nil || run.AbortReason != nil {
			t.Fatalf("run changed despite failed roll cleanup: %#v", run)
		}
	}
}

func TestDeletePageRollsBackSystemIndexDeletesTogether(t *testing.T) {
	conn, pageID, _, _, _ := setupDeletePageStoreTest(t)
	conn.Close()
	sdb, err := OpenSystem()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sdb.Exec(`
		CREATE TRIGGER reject_active_page_delete
		BEFORE DELETE ON active_page
		BEGIN
			SELECT RAISE(ABORT, 'active delete rejected');
		END
	`); err != nil {
		t.Fatal(err)
	}
	sdb.Close()

	if err := DeletePage(pageID); err == nil || !strings.Contains(err.Error(), "active delete rejected") {
		t.Fatalf("DeletePage error = %v", err)
	}
	assertPageIndexCount(t, pageID, 1, 1)
}

func TestDeletePageRetryFinishesIndexCleanupAfterSystemFailure(t *testing.T) {
	conn, pageID, _, _, _ := setupDeletePageStoreTest(t)
	conn.Close()
	sdb, err := OpenSystem()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sdb.Exec(`
		CREATE TRIGGER reject_active_page_delete
		BEFORE DELETE ON active_page
		BEGIN
			SELECT RAISE(ABORT, 'active delete rejected');
		END
	`); err != nil {
		t.Fatal(err)
	}
	sdb.Close()

	if err := DeletePage(pageID); err == nil || !strings.Contains(err.Error(), "active delete rejected") {
		t.Fatalf("first DeletePage error = %v", err)
	}
	assertPageIndexCount(t, pageID, 1, 1)

	sdb, err = OpenSystem()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sdb.Exec("DROP TRIGGER reject_active_page_delete"); err != nil {
		t.Fatal(err)
	}
	sdb.Close()

	if err := DeletePage(pageID); err != nil {
		t.Fatalf("retry DeletePage error = %v", err)
	}
	assertPageIndexCount(t, pageID, 0, 0)
}

func setupDeletePageStoreTest(t *testing.T) (*sql.DB, string, int64, int64, string) {
	t.Helper()
	setTestHome(t)
	testCwd := t.TempDir()

	// Create inner DB with schema and template page
	innerPath, err := InnerDbPath("test-roll", "latest")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(innerPath), 0755); err != nil {
		t.Fatal(err)
	}
	innerConn, err := rolldb.Open(innerPath)
	if err != nil {
		t.Fatal(err)
	}
	innerSchema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_inner.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := innerConn.Exec(string(innerSchema)); err != nil {
		t.Fatal(err)
	}
	// Insert template page for InsertPage
	if _, err := innerConn.Exec(`
		INSERT INTO pages (page_id, cwd, context, created_at, updated_at)
		VALUES ('0', '', '{}', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}
	innerConn.Close()

	// Create workspace outer DB in unique temp cwd
	outerPath, err := CwdOuterDbPath(testCwd, "test-roll")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(outerPath), 0755); err != nil {
		t.Fatal(err)
	}
	outerConn, err := rolldb.Open(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	outerSchema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_outer.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outerConn.Exec(string(outerSchema)); err != nil {
		t.Fatal(err)
	}
	outerConn.Close()

	// Open with ATTACH
	conn, err := rolldb.OpenOuter(outerPath, innerPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rolldb.InsertLoopSeed(conn, "review", "normal", "Review memory", "Inspect memories", 0.8); err != nil {
		t.Fatal(err)
	}
	page, err := rolldb.InsertPage(conn, testCwd)
	if err != nil {
		t.Fatal(err)
	}
	main, err := rolldb.StartLoopRun(conn, page.PageID, "review", nil, "null")
	if err != nil {
		t.Fatal(err)
	}
	child, err := rolldb.StartLoopRun(conn, page.PageID, "review", &main.ID, "null")
	if err != nil {
		t.Fatal(err)
	}
	if err := IndexPage("test-roll", "latest", page.PageID, testCwd, outerPath, ""); err != nil {
		t.Fatal(err)
	}
	return conn, page.PageID, main.ID, child.ID, testCwd
}

func assertPageIndexCount(t *testing.T, pageID string, wantIndex, wantActive int) {
	t.Helper()
	sdb, err := OpenSystem()
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()
	for table, want := range map[string]int{"page_index": wantIndex, "active_page": wantActive} {
		var got int
		if err := sdb.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE page_id = ?", pageID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count = %d, want %d", table, got, want)
		}
	}
}
