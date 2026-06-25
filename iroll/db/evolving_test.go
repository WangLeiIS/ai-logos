package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitSQL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single statement",
			input: "SELECT 1",
			want:  []string{"SELECT 1"},
		},
		{
			name:  "multiple statements",
			input: "SELECT 1; SELECT 2; SELECT 3",
			want:  []string{"SELECT 1", "SELECT 2", "SELECT 3"},
		},
		{
			name:  "trailing semicolon",
			input: "SELECT 1;",
			want:  []string{"SELECT 1"},
		},
		{
			name:  "leading semicolon",
			input: ";SELECT 1",
			want:  []string{"SELECT 1"},
		},
		{
			name:  "whitespace trimming",
			input: "  SELECT 1  ;  \nSELECT 2\t\n",
			want:  []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "only semicolons",
			input: ";; ;",
			want:  nil,
		},
		{
			name:  "statement with semicolons in string literal",
			input: "INSERT INTO t VALUES ('hello;world')",
			want:  []string{"INSERT INTO t VALUES ('hello;world')"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitSQL(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("SplitSQL(%q) = %#v (len=%d), want %#v (len=%d)",
					tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("SplitSQL(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsQuery(t *testing.T) {
	tests := []struct {
		stmt string
		want bool
	}{
		{"SELECT * FROM t", true},
		{"select * from t", true},
		{"  SELECT 1", true},
		{"PRAGMA table_info('t')", true},
		{"EXPLAIN SELECT 1", true},
		{"explain select 1", true},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", true},
		{"INSERT INTO t VALUES (1)", false},
		{"UPDATE t SET x=1", false},
		{"DELETE FROM t", false},
		{"CREATE TABLE t (id INT)", false},
		{"ALTER TABLE t ADD COLUMN x INT", false},
		{"DROP TABLE t", false},
	}

	for _, tt := range tests {
		t.Run(tt.stmt, func(t *testing.T) {
			got := isQuery(tt.stmt)
			if got != tt.want {
				t.Fatalf("isQuery(%q) = %v, want %v", tt.stmt, got, tt.want)
			}
		})
	}
}

func openEvolvingTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })

	schema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_inner.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestExecuteOneSelect(t *testing.T) {
	conn := openEvolvingTestDB(t)

	result, err := ExecuteOne(conn, "SELECT 1 AS num", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "rows" {
		t.Fatalf("type = %q, want rows", result.Type)
	}
	if len(result.Columns) != 1 || result.Columns[0] != "num" {
		t.Fatalf("columns = %#v", result.Columns)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "1" {
		t.Fatalf("rows = %#v", result.Rows)
	}
	if result.Count != 1 {
		t.Fatalf("count = %d, want 1", result.Count)
	}
}

func TestExecuteOneInsert(t *testing.T) {
	conn := openEvolvingTestDB(t)

	result, err := ExecuteOne(conn,
		"INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) "+
			"VALUES ('test', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now'))", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "affected" {
		t.Fatalf("type = %q, want affected", result.Type)
	}
	if result.AffectedRows != 1 {
		t.Fatalf("affected_rows = %d, want 1", result.AffectedRows)
	}
}

func TestExecuteOneUpdate(t *testing.T) {
	conn := openEvolvingTestDB(t)
	_, err := conn.Exec("INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) " +
		"VALUES ('test', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now'))")
	if err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteOne(conn, "UPDATE dna SET weight=0.99 WHERE name='test'", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "affected" {
		t.Fatalf("type = %q, want affected", result.Type)
	}
	if result.AffectedRows != 1 {
		t.Fatalf("affected_rows = %d, want 1", result.AffectedRows)
	}
}

func TestExecuteOneDelete(t *testing.T) {
	conn := openEvolvingTestDB(t)
	_, err := conn.Exec("INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) " +
		"VALUES ('test', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now'))")
	if err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteOne(conn, "DELETE FROM dna WHERE name='test'", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "affected" {
		t.Fatalf("type = %q, want affected", result.Type)
	}
	if result.AffectedRows != 1 {
		t.Fatalf("affected_rows = %d, want 1", result.AffectedRows)
	}
}

func TestExecuteOneDryRunDoesNotPersist(t *testing.T) {
	conn := openEvolvingTestDB(t)

	_, err := ExecuteOne(conn,
		"INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) "+
			"VALUES ('test', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now'))", true)
	if err != nil {
		t.Fatal(err)
	}

	var count int
	if err := conn.QueryRow("SELECT count(*) FROM dna WHERE name='test'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("dry-run persisted data: count=%d", count)
	}
}

func TestExecuteAllMultipleStatements(t *testing.T) {
	conn := openEvolvingTestDB(t)

	rawSQL := `
		INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at)
			VALUES ('a', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now'));
		INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at)
			VALUES ('b', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now'));
		SELECT count(*) AS cnt FROM dna WHERE name IN ('a', 'b');
	`
	results, err := ExecuteAll(conn, rawSQL, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[0].Type != "affected" || results[0].AffectedRows != 1 {
		t.Fatalf("results[0] = %#v", results[0])
	}
	if results[1].Type != "affected" || results[1].AffectedRows != 1 {
		t.Fatalf("results[1] = %#v", results[1])
	}
	if results[2].Type != "rows" || results[2].Count != 1 {
		t.Fatalf("results[2] = %#v", results[2])
	}
}

func TestExecuteAllStopsAtFirstError(t *testing.T) {
	conn := openEvolvingTestDB(t)

	rawSQL := "INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) " +
		"VALUES ('a', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now')); " +
		"NOT A VALID STATEMENT; SELECT 1;"
	results, err := ExecuteAll(conn, rawSQL, false)
	if err == nil {
		t.Fatal("expected error for invalid SQL")
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 (first statement succeeded)", len(results))
	}
	if !strings.Contains(err.Error(), "statement 2/3 failed") {
		t.Fatalf("error should contain statement position: %v", err)
	}
}

func TestExecuteAllEmptyInput(t *testing.T) {
	conn := openEvolvingTestDB(t)

	results, err := ExecuteAll(conn, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}

func TestExecuteAllDryRunDoesNotPersist(t *testing.T) {
	conn := openEvolvingTestDB(t)

	rawSQL := "INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) " +
		"VALUES ('test-dry', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now')); " +
		"SELECT count(*) AS cnt FROM dna;"
	results, err := ExecuteAll(conn, rawSQL, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	var count int
	if err := conn.QueryRow("SELECT count(*) FROM dna WHERE name='test-dry'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("dry-run persisted data: count=%d", count)
	}
}
