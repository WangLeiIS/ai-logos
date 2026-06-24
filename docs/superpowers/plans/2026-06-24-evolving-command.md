# Evolving Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `logos roll evolving` command to execute arbitrary SQL against an iroll's `ai_roll.db`.

**Architecture:** Two new files: `iroll/db/evolving.go` for the SQL execution engine (splitting, executing, dry-run) and `iroll/cmd/evolving.go` for the Cobra command (flag parsing, input routing, output). Follows existing patterns: `db.Open()` for connections, `builder.ParseTag()` + `store.GetActive()` for target resolution, `outputJSON`/`outputError` for CLI output.

**Tech Stack:** Go 1.x, Cobra CLI, go-sqlite3, existing `logos/db`, `logos/store`, `logos/builder` packages.

---

### Task 1: SQL Execution Engine — Types and SplitSQL

**Files:**
- Create: `iroll/db/evolving.go`

- [ ] **Step 1: Create `iroll/db/evolving.go` with types and `SplitSQL`**

```go
package db

import (
	"strings"
)

// EvolvingResult is the result of executing a single SQL statement.
type EvolvingResult struct {
	Type         string     `json:"type"`          // "rows" or "affected"
	Statement    string     `json:"statement"`
	Columns      []string   `json:"columns,omitempty"`
	Rows         [][]string `json:"rows,omitempty"`
	Count        int        `json:"count"`
	AffectedRows int64      `json:"affected_rows,omitempty"`
}

// SplitSQL splits raw SQL by semicolons, trimming whitespace and skipping empty statements.
func SplitSQL(raw string) []string {
	parts := strings.Split(raw, ";")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// isQuery returns true if the statement is a read-only query (SELECT, PRAGMA, EXPLAIN, WITH).
func isQuery(stmt string) bool {
	upper := strings.ToUpper(strings.TrimSpace(stmt))
	return strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "PRAGMA") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "WITH")
}
```

- [ ] **Step 2: Write unit tests for `SplitSQL` and `isQuery`**

**Files:**
- Create: `iroll/db/evolving_test.go`

```go
package db

import (
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
```

- [ ] **Step 3: Run unit tests for SplitSQL and isQuery**

Run: `cd iroll && go test ./db/ -run "TestSplitSQL|TestIsQuery" -v`
Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add iroll/db/evolving.go iroll/db/evolving_test.go
git commit -m "feat: add SplitSQL and EvolvingResult type for evolving command

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: SQL Execution Engine — ExecuteOne and ExecuteAll

**Files:**
- Modify: `iroll/db/evolving.go` — add imports and execution functions

- [ ] **Step 1: Add imports and execution functions to `iroll/db/evolving.go`**

Change the import block to:
```go
import (
	"database/sql"
	"fmt"
	"strings"
)
```

Append after `isQuery`:

```go
// ExecuteOne executes a single SQL statement against the database.
// If dryRun is true, the statement is executed inside a transaction that is rolled back.
func ExecuteOne(db *sql.DB, stmt string, dryRun bool) (EvolvingResult, error) {
	if dryRun {
		tx, err := db.Begin()
		if err != nil {
			return EvolvingResult{}, fmt.Errorf("begin dry-run transaction: %w", err)
		}
		defer tx.Rollback()

		result, err := executeOneConn(tx, stmt)
		if err != nil {
			return EvolvingResult{}, err
		}
		// tx.Rollback() via defer — never committed
		return result, nil
	}

	return executeOneConn(db, stmt)
}

func executeOneConn(exe execer, stmt string) (EvolvingResult, error) {
	if isQuery(stmt) {
		return executeQuery(exe, stmt)
	}
	return executeMutation(exe, stmt)
}

type execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

func executeQuery(exe execer, stmt string) (EvolvingResult, error) {
	rows, err := exe.Query(stmt)
	if err != nil {
		return EvolvingResult{}, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return EvolvingResult{}, fmt.Errorf("columns: %w", err)
	}

	var result [][]string
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return EvolvingResult{}, fmt.Errorf("scan: %w", err)
		}
		row := make([]string, len(columns))
		for i, v := range values {
			if v == nil {
				row[i] = "null"
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		result = append(result, row)
	}
	if result == nil {
		result = [][]string{}
	}

	return EvolvingResult{
		Type:      "rows",
		Statement: stmt,
		Columns:   columns,
		Rows:      result,
		Count:     len(result),
	}, nil
}

func executeMutation(exe execer, stmt string) (EvolvingResult, error) {
	res, err := exe.Exec(stmt)
	if err != nil {
		return EvolvingResult{}, fmt.Errorf("exec: %w", err)
	}

	affected, _ := res.RowsAffected()

	return EvolvingResult{
		Type:         "affected",
		Statement:    stmt,
		AffectedRows: affected,
	}, nil
}

// ExecuteAll splits raw SQL and executes all statements sequentially.
// Stops at the first error, returning results for all statements that succeeded.
func ExecuteAll(db *sql.DB, rawSQL string, dryRun bool) ([]EvolvingResult, error) {
	statements := SplitSQL(rawSQL)
	if len(statements) == 0 {
		return []EvolvingResult{}, nil
	}

	if dryRun {
		tx, err := db.Begin()
		if err != nil {
			return nil, fmt.Errorf("begin dry-run transaction: %w", err)
		}
		defer tx.Rollback()

		var results []EvolvingResult
		for i, stmt := range statements {
			result, err := executeOneConn(tx, stmt)
			if err != nil {
				return results, fmt.Errorf("statement %d/%d failed: %w", i+1, len(statements), err)
			}
			results = append(results, result)
		}
		return results, nil
	}

	var results []EvolvingResult
	for i, stmt := range statements {
		result, err := ExecuteOne(db, stmt, false)
		if err != nil {
			return results, fmt.Errorf("statement %d/%d failed: %w", i+1, len(statements), err)
		}
		results = append(results, result)
	}
	return results, nil
}
```

- [ ] **Step 2: Write unit tests for `ExecuteOne` and `ExecuteAll`**

Append to `iroll/db/evolving_test.go`:

```go
import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
)

func openEvolvingTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })

	schema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_schema.sql"))
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
```

- [ ] **Step 3: Run unit tests**

Run: `cd iroll && go test ./db/ -run "TestExecuteOne|TestExecuteAll|TestSplitSQL|TestIsQuery" -v`
Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add iroll/db/evolving.go iroll/db/evolving_test.go
git commit -m "feat: add ExecuteOne and ExecuteAll SQL execution engine

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 3: Cobra Command — evolving.go

**Files:**
- Create: `iroll/cmd/evolving.go`

- [ ] **Step 1: Create `iroll/cmd/evolving.go`**

```go
package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"logos/builder"
	"logos/db"
	"logos/safepath"
	"logos/store"

	"github.com/spf13/cobra"
)

var evolvingSQL string
var evolvingFile string
var evolvingDryRun bool
var evolvingCwd string

var evolvingCmd = &cobra.Command{
	Use:   "evolving [name:version] [sql]",
	Short: "Execute SQL against an iroll's ai_roll.db",
	Long: `Execute arbitrary SQL statements against an iroll's ai_roll.db database.
Supports SELECT (returns JSON rows) and mutations (returns affected row count).

Target iroll: specify a name:version tag explicitly, or omit to auto-detect from --cwd.
SQL input (priority order): --sql flag, positional arguments, --file flag, stdin.`,
	Args: cobra.ArbitraryArgs,
	Run:  runEvolving,
}

func runEvolving(cmd *cobra.Command, args []string) {
	name, version := resolveEvolvingTarget(args)
	dbPath := checkedDbPath(name, version)

	sql := resolveEvolvingSQL(args)
	if sql == "" {
		outputError("no SQL provided (use --sql, positional args, --file, or stdin)")
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		outputError(err.Error())
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, sql, evolvingDryRun)
	if err != nil {
		if len(results) > 0 {
			outputJSON(results)
		}
		outputError(err.Error())
	}

	outputJSON(results)
}

// resolveEvolvingTarget resolves the target iroll (name, version).
// If the first positional argument looks like a tag, use it; otherwise detect from --cwd.
func resolveEvolvingTarget(args []string) (string, string) {
	if len(args) > 0 {
		first := args[0]
		// Tags never contain spaces
		if !strings.Contains(first, " ") {
			if err := safepath.ValidateName(strings.SplitN(first, ":", 2)[0]); err == nil {
				name, version, err := builder.ParseTag(first)
				if err == nil {
					return name, version
				}
			}
		}
	}

	// Auto-detect from cwd
	name, version, _, err := store.GetActive(evolvingCwd)
	if err != nil {
		outputError(err.Error())
	}
	return name, version
}

// isTagArg returns true if the first positional argument was consumed as a tag (not SQL).
func isTagArg(args []string) bool {
	if len(args) == 0 {
		return false
	}
	first := args[0]
	if strings.Contains(first, " ") {
		return false
	}
	if err := safepath.ValidateName(strings.SplitN(first, ":", 2)[0]); err != nil {
		return false
	}
	_, _, err := builder.ParseTag(first)
	return err == nil
}

// resolveEvolvingSQL resolves the SQL input from flags, args, file, or stdin.
func resolveEvolvingSQL(args []string) string {
	// Priority 1: --sql flag
	if evolvingSQL != "" {
		return evolvingSQL
	}

	// Priority 2: positional args (skip first if it's a tag)
	if len(args) > 0 {
		start := 0
		if isTagArg(args) {
			start = 1
		}
		if len(args) > start {
			return strings.Join(args[start:], " ")
		}
	}

	// Priority 3: --file flag
	if evolvingFile != "" {
		data, err := os.ReadFile(evolvingFile)
		if err != nil {
			outputError(fmt.Sprintf("read file %q: %v", evolvingFile, err))
		}
		return string(data)
	}

	// Priority 4: stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			outputError(fmt.Sprintf("read stdin: %v", err))
		}
		return string(data)
	}

	return ""
}

func init() {
	evolvingCmd.Flags().StringVar(&evolvingSQL, "sql", "", "SQL statement(s) to execute")
	evolvingCmd.Flags().StringVar(&evolvingFile, "file", "", "Path to SQL file")
	evolvingCmd.Flags().BoolVar(&evolvingDryRun, "dry-run", false, "Preview mode: execute in transaction then rollback")
	evolvingCmd.Flags().StringVar(&evolvingCwd, "cwd", ".", "Working directory (auto-detect mode)")

	rollCmd.AddCommand(evolvingCmd)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd iroll && go build -ldflags "-X logos/cmd.Version=0.1.0" -o ../logos .`
Expected: builds without errors

- [ ] **Step 3: Commit**

```bash
git add iroll/cmd/evolving.go
git commit -m "feat: add evolving Cobra command with flag parsing and input routing

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 4: Integration Tests for Evolving Command

**Files:**
- Create: `iroll/cmd/evolving_test.go`

- [ ] **Step 1: Create `iroll/cmd/evolving_test.go`**

```go
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"logos/db"
	"logos/store"
)

func setupEvolvingTest(t *testing.T) (string, string) {
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

	conn, err := db.Open(filepath.Join(rollRoot, "ai_roll.db"))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_schema.sql"))
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if _, err := conn.Exec(string(schema)); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	conn.Close()

	if err := store.IndexPage(rollName, "latest", "page-one", cwd); err != nil {
		t.Fatal(err)
	}
	return cwd, rollRoot
}

func TestResolveEvolvingTargetWithExplicitTag(t *testing.T) {
	name, version := resolveEvolvingTarget([]string{"my-agent:v0.1.0"})
	if name != "my-agent" || version != "v0.1.0" {
		t.Fatalf("resolveEvolvingTarget = (%q, %q), want (my-agent, v0.1.0)", name, version)
	}
}

func TestResolveEvolvingTargetWithDefaultVersion(t *testing.T) {
	name, version := resolveEvolvingTarget([]string{"my-agent"})
	if name != "my-agent" || version != "latest" {
		t.Fatalf("resolveEvolvingTarget = (%q, %q), want (my-agent, latest)", name, version)
	}
}

func TestResolveEvolvingTargetAutoDetect(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	evolvingCwd = cwd
	defer func() { evolvingCwd = origCwd }()

	name, version := resolveEvolvingTarget(nil)
	if name != "test-roll" || version != "latest" {
		t.Fatalf("resolveEvolvingTarget = (%q, %q), want (test-roll, latest)", name, version)
	}
}

func TestResolveEvolvingSQLFromPositionalArgs(t *testing.T) {
	origSQL := evolvingSQL
	origFile := evolvingFile
	evolvingSQL = ""
	evolvingFile = ""
	defer func() {
		evolvingSQL = origSQL
		evolvingFile = origFile
	}()

	sql := resolveEvolvingSQL([]string{"SELECT 1"})
	if sql != "SELECT 1" {
		t.Fatalf("resolveEvolvingSQL = %q, want 'SELECT 1'", sql)
	}

	sql = resolveEvolvingSQL([]string{"my-agent", "SELECT 1"})
	if sql != "SELECT 1" {
		t.Fatalf("resolveEvolvingSQL = %q, want 'SELECT 1'", sql)
	}
}

func TestResolveEvolvingSQLFromFlag(t *testing.T) {
	origSQL := evolvingSQL
	origFile := evolvingFile
	evolvingSQL = "UPDATE dna SET weight=0.5"
	evolvingFile = ""
	defer func() {
		evolvingSQL = origSQL
		evolvingFile = origFile
	}()

	sql := resolveEvolvingSQL([]string{"my-agent"})
	if sql != "UPDATE dna SET weight=0.5" {
		t.Fatalf("resolveEvolvingSQL = %q, want flag SQL", sql)
	}
}

func TestResolveEvolvingSQLFromFile(t *testing.T) {
	origSQL := evolvingSQL
	origFile := evolvingFile
	evolvingSQL = ""
	evolvingFile = filepath.Join(t.TempDir(), "test.sql")
	defer func() {
		evolvingSQL = origSQL
		evolvingFile = origFile
	}()

	if err := os.WriteFile(evolvingFile, []byte("SELECT 2"), 0644); err != nil {
		t.Fatal(err)
	}

	sql := resolveEvolvingSQL(nil)
	if sql != "SELECT 2" {
		t.Fatalf("resolveEvolvingSQL = %q, want 'SELECT 2'", sql)
	}
}

func TestEvolvingEndToEndInsertAndSelect(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	origSQL := evolvingSQL
	origDryRun := evolvingDryRun
	evolvingCwd = cwd
	evolvingSQL = "INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) " +
		"VALUES ('test-e2e', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now')); " +
		"SELECT name, weight FROM dna WHERE name='test-e2e'"
	evolvingDryRun = false
	defer func() {
		evolvingCwd = origCwd
		evolvingSQL = origSQL
		evolvingDryRun = origDryRun
	}()

	name, version := resolveEvolvingTarget(nil)
	dbPath, err := store.DbPath(name, version)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, evolvingSQL, false)
	if err != nil {
		t.Fatalf("ExecuteAll: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Type != "affected" || results[0].AffectedRows != 1 {
		t.Fatalf("results[0] = %#v", results[0])
	}
	if results[1].Type != "rows" || results[1].Count != 1 {
		t.Fatalf("results[1] = %#v", results[1])
	}
}

func TestEvolvingEndToEndDryRun(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	origSQL := evolvingSQL
	origDryRun := evolvingDryRun
	evolvingCwd = cwd
	evolvingSQL = "INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) " +
		"VALUES ('test-dry', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now'))"
	evolvingDryRun = true
	defer func() {
		evolvingCwd = origCwd
		evolvingSQL = origSQL
		evolvingDryRun = origDryRun
	}()

	name, version := resolveEvolvingTarget(nil)
	dbPath, err := store.DbPath(name, version)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = db.ExecuteAll(conn, evolvingSQL, true)
	if err != nil {
		t.Fatalf("ExecuteAll dry-run: %v", err)
	}

	var count int
	if err := conn.QueryRow("SELECT count(*) FROM dna WHERE name='test-dry'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("dry-run persisted data: count=%d", count)
	}
}

func TestEvolvingEndToEndJSONOutputFormat(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	origSQL := evolvingSQL
	origDryRun := evolvingDryRun
	evolvingCwd = cwd
	evolvingSQL = "SELECT 1 AS value"
	evolvingDryRun = false
	defer func() {
		evolvingCwd = origCwd
		evolvingSQL = origSQL
		evolvingDryRun = origDryRun
	}()

	name, version := resolveEvolvingTarget(nil)
	dbPath, err := store.DbPath(name, version)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, evolvingSQL, false)
	if err != nil {
		t.Fatalf("ExecuteAll: %v", err)
	}

	data, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"type":"rows"`) {
		t.Fatalf("JSON output missing type: %s", string(data))
	}
}

func TestEvolvingInvalidSQLReturnsError(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	origSQL := evolvingSQL
	evolvingCwd = cwd
	evolvingSQL = "NOT A VALID STATEMENT"
	defer func() {
		evolvingCwd = origCwd
		evolvingSQL = origSQL
	}()

	name, version := resolveEvolvingTarget(nil)
	dbPath, err := store.DbPath(name, version)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = db.ExecuteAll(conn, evolvingSQL, false)
	if err == nil {
		t.Fatal("expected error for invalid SQL")
	}
}
```

- [ ] **Step 2: Run integration tests**

Run: `cd iroll && go test ./cmd/ -run "TestResolveEvolving|TestEvolvingEndToEnd|TestEvolvingInvalid" -v`
Expected: all tests PASS

- [ ] **Step 3: Run all tests to verify no regressions**

Run: `cd iroll && go test ./... -v`
Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add iroll/cmd/evolving_test.go
git commit -m "test: add integration tests for evolving command

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 5: Manual Smoke Test

- [ ] **Step 1: Build the CLI**

```bash
cd iroll && go build -ldflags "-X logos/cmd.Version=0.1.0" -o ../logos .
```

Expected: builds without errors

- [ ] **Step 2: Test --help**

```bash
./logos roll evolving --help
```

Expected: shows usage, flags, description

- [ ] **Step 3: Test query with explicit tag**

```bash
./logos roll evolving <your-iroll>:latest --sql "SELECT name, type, weight FROM dna ORDER BY weight DESC LIMIT 3"
```

Expected: JSON array with rows from dna table

- [ ] **Step 4: Test --dry-run**

```bash
./logos roll evolving <your-iroll>:latest --sql "INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) VALUES ('smoke-test', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now'))" --dry-run
```

Expected: shows affected_rows=1, verify with SELECT after that data was NOT persisted

- [ ] **Step 5: Test --file flag**

```bash
./logos roll evolving <your-iroll>:latest --file examples/base-agent/init_data.sql --dry-run
```

Expected: JSON array with results for each statement in the file
