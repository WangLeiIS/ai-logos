# Memory 系统重构 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 重构 memory 表结构（参考 dna Q&A），去掉 add-memory CLI，补齐 query-memory 和 QueryMemory API。

**Architecture:** 修改 memory 表增加 page_id/name/question/sleep_count 列，DB 层新增 QueryMemory/IncrementSleepCount/UpdateMemoryContent，CLI 重写为 query-memory（默认摘要、--full 返回完整内容）。

**Tech Stack:** Go 1.24, SQLite (go-sqlite3), Cobra CLI

---

### Task 1: 更新 init_schema.sql 中的 memory 表

**Files:**
- Modify: `examples/base-agent/init_schema.sql:8-13`

- [ ] **Step 1: 替换 CREATE TABLE memory**

```sql
CREATE TABLE memory (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_id TEXT NOT NULL,
    name TEXT NOT NULL,
    question TEXT NOT NULL,
    content TEXT NOT NULL,
    importance REAL NOT NULL DEFAULT 0.5,
    sleep_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_memory_page ON memory(page_id, importance);
```

- [ ] **Step 2: 运行现有测试确认 schema 兼容**

Run: `cd iroll && go test ./db/ -v -run "TestLoopSeedLifecycle"`
Expected: 测试通过（memory 表现在依赖 init_schema.sql 但当前没有测试用 memory 表，所以不会 break）

- [ ] **Step 3: Commit**

```bash
git add examples/base-agent/init_schema.sql
git commit -m "feat(memory): update schema with name/question/page_id/sleep_count columns"
```

---

### Task 2: 重写 DB 层 Memory 类型和函数

**Files:**
- Modify: `iroll/db/db.go:28-33` (Memory struct), `iroll/db/db.go:144-161` (InsertMemory)
- Create: `iroll/db/memory.go` (MemorySummary, QueryMemoryParams, QueryMemory, IncrementSleepCount, UpdateMemoryContent)

- [ ] **Step 1: 更新 Memory struct 和新增类型**

在 `iroll/db/db.go` 中，替换 Memory struct 为：

```go
type Memory struct {
	ID         int64   `json:"id"`
	PageID     string  `json:"page_id"`
	Name       string  `json:"name"`
	Question   string  `json:"question"`
	Content    string  `json:"content"`
	Importance float64 `json:"importance"`
	SleepCount int     `json:"sleep_count"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type MemorySummary struct {
	Name       string `json:"name"`
	Question   string `json:"question"`
	ContentLen int    `json:"content_len"`
	SleepCount int    `json:"sleep_count"`
}

type QueryMemoryParams struct {
	Name          string
	Keyword       string
	MinImportance float64
	Since         string
	Before        string
	Limit         int
	Offset        int
}
```

- [ ] **Step 2: 创建 memory.go（DB 层函数）**

创建 `iroll/db/memory.go`：

```go
package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func InsertMemory(db *sql.DB, pageID, name, question, content string, importance float64) (*Memory, error) {
	name = strings.TrimSpace(name)
	question = strings.TrimSpace(question)
	content = strings.TrimSpace(content)
	if name == "" || question == "" || content == "" {
		return nil, fmt.Errorf("insert memory: name, question, content must not be blank")
	}
	if importance < 0 || importance > 1 {
		return nil, fmt.Errorf("insert memory: importance must be 0.0-1.0")
	}

	now := nowISO()
	result, err := db.Exec(`
		INSERT INTO memory (page_id, name, question, content, importance, sleep_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)
	`, pageID, name, question, content, importance, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}
	return &Memory{
		ID:         id,
		PageID:     pageID,
		Name:       name,
		Question:   question,
		Content:    content,
		Importance: importance,
		SleepCount: 0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func QueryMemory(db *sql.DB, pageID string, params QueryMemoryParams) ([]Memory, error) {
	query := "SELECT id, page_id, name, question, content, importance, sleep_count, created_at, updated_at FROM memory WHERE page_id = ?"
	args := []any{pageID}

	if params.Name != "" {
		query += " AND name = ?"
		args = append(args, params.Name)
	}
	if params.Keyword != "" {
		query += " AND (name LIKE ? OR question LIKE ?)"
		kw := "%" + params.Keyword + "%"
		args = append(args, kw, kw)
	}
	if params.MinImportance > 0 {
		query += " AND importance >= ?"
		args = append(args, params.MinImportance)
	}
	if params.Since != "" {
		query += " AND created_at >= ?"
		args = append(args, params.Since)
	}
	if params.Before != "" {
		query += " AND created_at <= ?"
		args = append(args, params.Before)
	}

	query += " ORDER BY importance DESC, created_at DESC"

	if params.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, params.Limit)
	}
	if params.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, params.Offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query memory: %w", err)
	}
	defer rows.Close()

	var result []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.PageID, &m.Name, &m.Question, &m.Content, &m.Importance, &m.SleepCount, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("query memory: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query memory: %w", err)
	}
	return result, nil
}

func IncrementSleepCount(db *sql.DB, memoryID int64) error {
	now := nowISO()
	_, err := db.Exec("UPDATE memory SET sleep_count = sleep_count + 1, updated_at = ? WHERE id = ?", now, memoryID)
	if err != nil {
		return fmt.Errorf("increment sleep count for memory %d: %w", memoryID, err)
	}
	return nil
}

func UpdateMemoryContent(db *sql.DB, memoryID int64, content string, importance float64) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("update memory content: content must not be blank")
	}
	if importance < 0 || importance > 1 {
		return fmt.Errorf("update memory content: importance must be 0.0-1.0")
	}

	now := nowISO()
	_, err := db.Exec(
		"UPDATE memory SET content = ?, importance = ?, updated_at = ? WHERE id = ?",
		content, importance, now, memoryID,
	)
	if err != nil {
		return fmt.Errorf("update memory content for %d: %w", memoryID, err)
	}
	return nil
}
```

- [ ] **Step 3: 删除 db.go 中旧的 InsertMemory 函数**

删除 `iroll/db/db.go` 第 144-161 行（旧的 InsertMemory）。

- [ ] **Step 4: 验证编译通过**

Run: `cd iroll && go build ./...`
Expected: 编译成功

- [ ] **Step 5: Commit**

```bash
git add iroll/db/db.go iroll/db/memory.go
git commit -m "feat(memory): add QueryMemory, IncrementSleepCount, UpdateMemoryContent"
```

---

### Task 3: 编写 DB 层测试

**Files:**
- Create: `iroll/db/memory_test.go`

- [ ] **Step 1: 编写 memory_test.go**

```go
package db

import (
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
}

func TestQueryMemoryByName(t *testing.T) {
	conn := openMemoryTestDB(t)

	InsertMemory(conn, "page-1", "python-version", "Python 版本？", "Python 3.12", 0.8)
	InsertMemory(conn, "page-1", "go-version", "Go 版本？", "Go 1.24", 0.5)

	results, err := QueryMemory(conn, "page-1", QueryMemoryParams{Name: "python-version"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "python-version" {
		t.Fatalf("got %d results, want 1 with name python-version", len(results))
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `cd iroll && go test ./db/ -v -run "TestInsertMemoryValidation|TestInsertAndQueryMemory|TestQueryMemoryFilters|TestIncrementSleepCount|TestUpdateMemoryContent|TestQueryMemoryByName"`
Expected: 全部 PASS

- [ ] **Step 3: Commit**

```bash
git add iroll/db/memory_test.go
git commit -m "test(memory): add tests for InsertMemory, QueryMemory, sleep count, and content update"
```

---

### Task 4: 重写 CLI memory 命令

**Files:**
- Modify: `iroll/cmd/memory.go` (完整重写)

- [ ] **Step 1: 重写 memory.go**

```go
package cmd

import (
	"path/filepath"

	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var queryMemoryKeyword string
var queryMemoryMinImportance float64
var queryMemorySince string
var queryMemoryBefore string
var queryMemoryLimit int
var queryMemoryFull bool
var queryMemoryCwd string

var queryMemoryCmd = &cobra.Command{
	Use:   "query-memory [name]",
	Short: "Query memories",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(queryMemoryCwd)
		irollName, _, err := store.GetActive(cwd)
		if err != nil {
			outputError(err.Error())
		}

		conn, err := db.Open(checkedDbPath(irollName))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		params := db.QueryMemoryParams{
			MinImportance: queryMemoryMinImportance,
			Since:         queryMemorySince,
			Before:        queryMemoryBefore,
			Limit:         queryMemoryLimit,
		}
		if len(args) > 0 {
			params.Name = args[0]
		} else if queryMemoryKeyword != "" {
			params.Keyword = queryMemoryKeyword
		}

		results, err := db.QueryMemory(conn, irollName, params)
		if err != nil {
			outputError(err.Error())
		}

		if queryMemoryFull {
			if results == nil {
				results = []db.Memory{}
			}
			outputJSON(results)
		} else {
			summaries := make([]db.MemorySummary, len(results))
			for i, m := range results {
				summaries[i] = db.MemorySummary{
					Name:       m.Name,
					Question:   m.Question,
					ContentLen: len(m.Content),
					SleepCount: m.SleepCount,
				}
			}
			if summaries == nil {
				summaries = []db.MemorySummary{}
			}
			outputJSON(summaries)
		}
	},
}

func init() {
	queryMemoryCmd.Flags().StringVar(&queryMemoryKeyword, "keyword", "", "Search keyword (matches name and question)")
	queryMemoryCmd.Flags().Float64Var(&queryMemoryMinImportance, "min-importance", 0, "Minimum importance (0.0-1.0)")
	queryMemoryCmd.Flags().StringVar(&queryMemorySince, "since", "", "Return memories after this ISO timestamp")
	queryMemoryCmd.Flags().StringVar(&queryMemoryBefore, "before", "", "Return memories before this ISO timestamp")
	queryMemoryCmd.Flags().IntVar(&queryMemoryLimit, "limit", 20, "Maximum results (1-100)")
	queryMemoryCmd.Flags().BoolVar(&queryMemoryFull, "full", false, "Return full records including content")
	queryMemoryCmd.Flags().StringVar(&queryMemoryCwd, "cwd", ".", "Working directory")

	pageCmd.AddCommand(queryMemoryCmd)
}
```

注意：删除原有的 `add-memory` 相关变量和函数，用上述代码完整替换 `memory.go` 的全部内容。

- [ ] **Step 2: 验证编译**

Run: `cd iroll && go build ./...`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add iroll/cmd/memory.go
git commit -m "feat(memory): replace add-memory with query-memory CLI command"
```

---

### Task 5: 运行全部测试，go vet，最终提交

**Files:**
- (无文件变更，仅验证)

- [ ] **Step 1: 运行全部测试**

Run: `cd iroll && go test ./... -v`
Expected: 所有测试 PASS

- [ ] **Step 2: 运行 go vet**

Run: `cd iroll && go vet ./...`
Expected: 无警告

- [ ] **Step 3: 更新文档中已完成项**

将 `docs/todo.md` 第 9 行更新为：
```
- [x] memory 重构 — 表结构增加 name/question/page_id 列（参考 dna Q&A），去掉 add-memory CLI（memory 由 context 压缩自动插入），补齐 query-memory CLI
```

将 `docs/rebot-roll.md` 第 317 行 "memory 重构" 移至已完成列表。

- [ ] **Step 4: Commit**

```bash
git add docs/todo.md docs/rebot-roll.md
git commit -m "docs: mark memory redesign as complete"
```
