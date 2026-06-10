package db

import (
	"database/sql"
	"testing"
)

func openMemoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	applyLoopTestSchema(t, conn)
	return conn
}

func TestInsertMemoryValidation(t *testing.T) {
	conn := openMemoryTestDB(t)

	_, err := InsertMemory(conn, "page-1", "", "q", "content", 0.5)
	if err == nil {
		t.Fatal("expected error for blank name")
	}

	_, err = InsertMemory(conn, "page-1", "name", "", "content", 0.5)
	if err == nil {
		t.Fatal("expected error for blank question")
	}

	_, err = InsertMemory(conn, "page-1", "name", "q", "", 0.5)
	if err == nil {
		t.Fatal("expected error for blank content")
	}

	_, err = InsertMemory(conn, "", "name", "q", "content", 0.5)
	if err == nil {
		t.Fatal("expected error for blank page_id")
	}

	_, err = InsertMemory(conn, "page-1", "name", "q", "content", 1.5)
	if err == nil {
		t.Fatal("expected error for importance > 1.0")
	}

	_, err = InsertMemory(conn, "page-1", "name", "q", "content", -0.1)
	if err == nil {
		t.Fatal("expected error for importance < 0.0")
	}
}

func TestInsertAndQueryMemory(t *testing.T) {
	conn := openMemoryTestDB(t)

	mem, err := InsertMemory(conn, "page-1", "user-prefers-python", "用户偏好什么 Python 版本？", "用户偏好 Python 3.12+", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	if mem.SleepCount != 0 {
		t.Fatalf("new memory sleep_count = %d, want 0", mem.SleepCount)
	}
	if mem.PageID != "page-1" {
		t.Fatalf("page_id = %s, want page-1", mem.PageID)
	}

	results, err := QueryMemory(conn, "page-1", QueryMemoryParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d memories, want 1", len(results))
	}

	results, err = QueryMemory(conn, "page-2", QueryMemoryParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d memories for page-2, want 0", len(results))
	}
}

func TestQueryMemoryFilters(t *testing.T) {
	conn := openMemoryTestDB(t)

	InsertMemory(conn, "page-1", "python-version", "Python 版本？", "Python 3.12", 0.8)
	InsertMemory(conn, "page-1", "go-version", "Go 版本？", "Go 1.24", 0.5)
	InsertMemory(conn, "page-1", "rust-interest", "用户对 Rust 感兴趣吗？", "用户想学 Rust", 0.3)

	// Keyword search
	results, err := QueryMemory(conn, "page-1", QueryMemoryParams{Keyword: "Python"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("keyword 'Python' got %d results, want 1", len(results))
	}

	// Min importance filter
	results, err = QueryMemory(conn, "page-1", QueryMemoryParams{MinImportance: 0.7})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "python-version" {
		t.Fatalf("min importance 0.7 got %d results, want 1 (python-version)", len(results))
	}

	// Limit
	results, err = QueryMemory(conn, "page-1", QueryMemoryParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("limit 2 got %d results", len(results))
	}

	// Order: most important first
	results, err = QueryMemory(conn, "page-1", QueryMemoryParams{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Name != "python-version" || results[2].Name != "rust-interest" {
		t.Fatal("results not ordered by importance DESC")
	}
}

func TestIncrementSleepCount(t *testing.T) {
	conn := openMemoryTestDB(t)

	mem, _ := InsertMemory(conn, "page-1", "test", "test?", "content", 0.5)
	if err := IncrementSleepCount(conn, mem.ID); err != nil {
		t.Fatal(err)
	}

	results, _ := QueryMemory(conn, "page-1", QueryMemoryParams{})
	if len(results) != 1 || results[0].SleepCount != 1 {
		t.Fatalf("sleep_count = %d, want 1", results[0].SleepCount)
	}

	// Verify error on non-existent ID
	err := IncrementSleepCount(conn, 99999)
	if err == nil {
		t.Fatal("expected error for non-existent memory ID")
	}
}

func TestUpdateMemoryContent(t *testing.T) {
	conn := openMemoryTestDB(t)

	mem, _ := InsertMemory(conn, "page-1", "test", "test?", "original content", 0.5)
	if err := UpdateMemoryContent(conn, mem.ID, "refined content", 0.9); err != nil {
		t.Fatal(err)
	}

	results, _ := QueryMemory(conn, "page-1", QueryMemoryParams{})
	if len(results) != 1 {
		t.Fatal("memory not found after update")
	}
	if results[0].Content != "refined content" {
		t.Fatalf("content = %s, want 'refined content'", results[0].Content)
	}
	if results[0].Importance != 0.9 {
		t.Fatalf("importance = %f, want 0.9", results[0].Importance)
	}

	// Verify error on non-existent ID
	err := UpdateMemoryContent(conn, 99999, "content", 0.5)
	if err == nil {
		t.Fatal("expected error for non-existent memory ID")
	}
}

func TestQueryMemoryByName(t *testing.T) {
	conn := openMemoryTestDB(t)

	InsertMemory(conn, "page-1", "python-version", "Python 版本？", "Python 3.12", 0.8)
	InsertMemory(conn, "page-1", "go-version", "Go 版本？", "Go 1.24", 0.5)

	// Exact name match
	results, err := QueryMemory(conn, "page-1", QueryMemoryParams{Name: "python-version"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "python-version" {
		t.Fatalf("got %d results, want 1 with name python-version", len(results))
	}

	// Non-existent name
	results, err = QueryMemory(conn, "page-1", QueryMemoryParams{Name: "nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results for nonexistent name, want 0", len(results))
	}
}
