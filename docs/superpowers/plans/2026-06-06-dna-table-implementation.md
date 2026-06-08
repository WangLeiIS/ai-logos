# DNA Table Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add dna table (decision-making DNA as Q&A pairs), integrate into context via @sql, and add query-dna command for fuzzy lookup.

**Architecture:** New `dna` table in ai_roll.db schema with 8 columns. Template page context gains a dna key that loads questions (without answers) via @sql. New `logos page query-dna` subcommand for on-demand fuzzy lookup by name, returning full records including answers. `resolveSQL` in db.go needs updating to handle multi-column result sets.

**Tech Stack:** Go 1.24, Cobra, SQLite (go-sqlite3)

---

### Task 1: Add dna table to schema

**Files:**
- Modify: `examples/base-agent/init_schema.sql`

- [ ] **Step 1: Add dna table DDL**

Append to `examples/base-agent/init_schema.sql`:

```sql
CREATE TABLE dna (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    weight REAL DEFAULT 0.5,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

### Task 2: Add seed data and update template context

**Files:**
- Modify: `examples/base-agent/init_data.sql`

- [ ] **Step 1: Add seed dna rows**

Append to `examples/base-agent/init_data.sql`:

```sql
INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) VALUES
    ('handle-correction', '认知观', '用户说你错了，但你确定自己正确', '坚持己见，给出理由而非争论', 0.9, datetime('now'), datetime('now')),
    ('truth-vs-feelings', '伦理观', '说实话会伤害用户感情', '坦诚告知，但用建设性的方式表达', 0.8, datetime('now'), datetime('now')),
    ('minimal-vs-complete', '审美观', '3行能跑但不够健壮 vs 50行覆盖所有边界', '先交付简洁方案，告知边界条件', 0.7, datetime('now'), datetime('now'));
```

- [ ] **Step 2: Update template page context to include dna query**

Replace the template page insert row. The current row is:

```sql
INSERT INTO pages (page_id, cwd, context, created_at, updated_at) VALUES
    ('0', '', '{"system_prompt":"你是一个AI助手","greeting":{"@file":"Resources/greeting.txt"},"description":{"@sql":"SELECT value FROM metadata WHERE key = ''description''"}}', datetime('now'), datetime('now'));
```

Replace with:

```sql
INSERT INTO pages (page_id, cwd, context, created_at, updated_at) VALUES
    ('0', '', '{"system_prompt":"你是一个AI助手","greeting":{"@file":"Resources/greeting.txt"},"description":{"@sql":"SELECT value FROM metadata WHERE key = ''description''"},"dna":{"@sql":"SELECT type, weight, question FROM dna ORDER BY weight DESC"}}', datetime('now'), datetime('now'));
```

### Task 3: Update resolveSQL for multi-column queries

**Files:**
- Modify: `iroll/db/db.go:187-207`

**Why:** The dna query `SELECT type, weight, question FROM dna` returns 3 columns, but `resolveSQL` currently scans each row into a single string. Multi-column queries need to return column-name-keyed maps.

- [ ] **Step 1: Replace resolveSQL function**

Replace lines 187-207 in `iroll/db/db.go`:

```go
func resolveSQL(db *sql.DB, query string) interface{} {
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Sprintf("[sql error: %s]", err.Error())
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Sprintf("[sql error: %s]", err.Error())
	}

	// Single-column: keep existing behavior (string or []string)
	if len(columns) == 1 {
		var results []string
		for rows.Next() {
			var val string
			if err := rows.Scan(&val); err != nil {
				return fmt.Sprintf("[sql error: %s]", err.Error())
			}
			results = append(results, val)
		}
		if len(results) == 1 {
			return results[0]
		}
		return results
	}

	// Multi-column: return []map[string]interface{} keyed by column name
	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Sprintf("[sql error: %s]", err.Error())
		}
		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}
	return results
}
```

### Task 4: Add Dna struct and QueryDna function

**Files:**
- Modify: `iroll/db/db.go` (add struct and function)

- [ ] **Step 1: Add Dna struct**

Add after the `Memory` struct (after line 29):

```go
type Dna struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Question  string  `json:"question"`
	Answer    string  `json:"answer"`
	Weight    float64 `json:"weight"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}
```

- [ ] **Step 2: Add QueryDna function**

Add at the end of `iroll/db/db.go`:

```go
func QueryDna(db *sql.DB, name string, dnaType string) ([]Dna, error) {
	query := "SELECT id, name, type, question, answer, weight, created_at, updated_at FROM dna WHERE name LIKE ?"
	args := []interface{}{"%" + name + "%"}
	if dnaType != "" {
		query += " AND type = ?"
		args = append(args, dnaType)
	}
	query += " ORDER BY weight DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query dna: %w", err)
	}
	defer rows.Close()

	var result []Dna
	for rows.Next() {
		var d Dna
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &d.Question, &d.Answer, &d.Weight, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("query dna: %w", err)
		}
		result = append(result, d)
	}
	return result, nil
}
```

### Task 5: Create query-dna command

**Files:**
- Create: `iroll/cmd/query_dna.go`

- [ ] **Step 1: Write the command file**

Create `iroll/cmd/query_dna.go`:

```go
package cmd

import (
	"path/filepath"

	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var queryDnaType string
var queryDnaCwd string

var queryDnaCmd = &cobra.Command{
	Use:   "query-dna <name-keyword>",
	Short: "Query dna entries by name (fuzzy match)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		cwd, _ := filepath.Abs(queryDnaCwd)
		irollName, _, err := store.GetActive(cwd)
		if err != nil {
			outputError(err.Error())
		}

		conn, err := db.Open(store.DbPath(irollName))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		results, err := db.QueryDna(conn, name, queryDnaType)
		if err != nil {
			outputError(err.Error())
		}

		if results == nil {
			results = []db.Dna{}
		}
		outputJSON(results)
	},
}

func init() {
	queryDnaCmd.Flags().StringVar(&queryDnaType, "type", "", "Filter by type (认知观/伦理观/审美观/本体观)")
	queryDnaCmd.Flags().StringVar(&queryDnaCwd, "cwd", ".", "Working directory")
}
```

### Task 6: Register query-dna as page subcommand

**Files:**
- Modify: `iroll/cmd/page.go` (add one line to init())

- [ ] **Step 1: Add query-dna to pageCmd**

In `iroll/cmd/page.go`, inside `init()`, add after the last `pageCmd.AddCommand(pageDeleteCmd)`:

```go
pageCmd.AddCommand(queryDnaCmd)
```

The init() function becomes:

```go
func init() {
	pageListCmd.Flags().StringVar(&pageListCwd, "cwd", ".", "Working directory to filter by")
	pageListCmd.Flags().BoolVarP(&pageListAll, "all", "a", false, "List all pages across all directories")
	pageNewCmd.Flags().StringVar(&pageNewCwd, "cwd", ".", "Working directory for the page")
	pageCurrentCmd.Flags().StringVar(&pageCurrentCwd, "cwd", ".", "Working directory")

	pageCmd.AddCommand(pageListCmd)
	pageCmd.AddCommand(pageNewCmd)
	pageCmd.AddCommand(pageSwitchCmd)
	pageCmd.AddCommand(pageCurrentCmd)
	pageCmd.AddCommand(pageDeleteCmd)
	pageCmd.AddCommand(queryDnaCmd)
	rootCmd.AddCommand(pageCmd)
}
```

### Task 7: Build and verify

- [ ] **Step 1: Build**

```bash
cd iroll && go build -o ../logos.exe .
```

Expected: Build succeeds with no errors.

- [ ] **Step 2: Rebuild example agent**

```bash
# Delete old ~/.iroll/ and rebuild
rm -rf ~/.iroll/
cd examples/base-agent && ../../logos.exe roll build -f Layerfile -t test-agent
```

- [ ] **Step 3: Verify dna table exists**

```bash
../../logos.exe roll inspect test-agent
```

Expected: Output includes dna table in the table listing.

- [ ] **Step 4: Verify context resolution**

```bash
../../logos.exe page new test-agent --cwd .
../../logos.exe page get-context --cwd .
```

Expected: context includes `dna` key with array of `{type, weight, question}` objects (no `answer` field).

- [ ] **Step 5: Verify query-dna command**

```bash
../../logos.exe page query-dna correction
```

Expected: Returns the `handle-correction` entry with all fields including `answer`.

```bash
../../logos.exe page query-dna correction --type 认知观
```

Expected: Same result, filtered by type.

```bash
../../logos.exe page query-dna nonexistent
```

Expected: Returns `[]` (empty array).

### Task 8: Update documentation

**Files:**
- Modify: `docs/rebot-roll.md`
- Modify: `skills/logos-1/skill.md`

- [ ] **Step 1: Update rebot-roll.md**

In `docs/rebot-roll.md`, section 3.1 (自我部分), change the dna row in the table from:

```
| dna | 否 | agent 的 DNA/人格定义（待定义） |
```

to:

```
| dna | 否 | agent 的决策 DNA，Q&A 对定义底层决策机制 |
```

Add a subsection "3.1.1 dna 表结构" after the metadata table. Insert after the metadata table ends (before section 3.2):

```markdown

**dna 表结构：**

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PRIMARY KEY AUTOINCREMENT | 主键 |
| name | TEXT | NOT NULL | 唯一标识，如 `handle-correction` |
| type | TEXT | NOT NULL | 决策维度：认知观 / 伦理观 / 审美观 / 本体观 |
| question | TEXT | NOT NULL | 决策困境 |
| answer | TEXT | NOT NULL | 这个 agent 的选择 |
| weight | REAL | DEFAULT 0.5 | 权重，越高越核心 |
| created_at | TEXT | NOT NULL | 创建时间 |
| updated_at | TEXT | NOT NULL | 更新时间 |
```

- [ ] **Step 2: Update skill.md command reference**

In `skills/logos-1/skill.md`, add to the command reference table after the `add-memory` row:

```
| `logos page query-dna <name> [--type <type>] [--cwd .]` | Fuzzy search dna by name |
```

Also update the Key Concepts section to mention dna. Add after the memory bullet:

```
- **dna** — decision-making Q&A pairs defining agent behavior. Context loads questions only (no answers); use `query-dna` to retrieve full records on demand
```

- [ ] **Step 3: Validate plan completeness**

After the plan, check that all spec requirements map to tasks:
- Schema: Task 1
- Seed data + template context: Task 2
- resolveSQL multi-column fix: Task 3
- QueryDna function: Task 4
- query-dna command: Tasks 5, 6
- Build/verify: Task 7
- Docs: Task 8
