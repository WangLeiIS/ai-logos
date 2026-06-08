# iroll 分层构建实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 iroll CLI 新增 `build`（解析 Layerfile 并分层构建）、`history`（查询构建历史）、`inspect`（查看包详情）三个命令

**Architecture:** 新增 `builder/` 包负责 Layerfile 解析和构建流程，新增 `db/build.go` 负责构建相关的数据库操作（history 表、layer.json hash），新增三个 cmd 文件注册 Cobra 子命令

**Tech Stack:** Go 1.24, Cobra, go-sqlite3, crypto/sha256, encoding/json

---

## File Structure

| 文件 | 操作 | 职责 |
|------|------|------|
| `iroll/builder/layerfile.go` | 创建 | Layerfile 解析器（FROM/MIGRATE/COPY 三条指令） |
| `iroll/builder/build.go` | 创建 | 构建引擎（复制基础层 → 执行 SQL → 复制文件 → 生成 layer.json → 记录 history → 打包） |
| `iroll/db/build.go` | 创建 | 构建相关数据库操作（执行 SQL 文件、记录 history、查询 history） |
| `iroll/cmd/build.go` | 创建 | `build` Cobra 子命令 |
| `iroll/cmd/history.go` | 创建 | `history` Cobra 子命令 |
| `iroll/cmd/inspect.go` | 创建 | `inspect` Cobra 子命令 |

---

### Task 1: Layerfile 解析器

**Files:**
- Create: `iroll/builder/layerfile.go`

- [ ] **Step 1: 创建 builder/layerfile.go**

```go
package builder

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InstructionType int

const (
	InstFrom InstructionType = iota
	InstMigrate
	InstCopy
)

type Instruction struct {
	Type InstructionType
	Args []string
}

type Layerfile struct {
	Instructions []Instruction
	Dir          string
}

func ParseLayerfile(path string) (*Layerfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open layerfile: %w", err)
	}
	defer f.Close()

	absPath, _ := filepath.Abs(path)
	dir := filepath.Dir(absPath)

	lf := &Layerfile{Dir: dir}
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		switch strings.ToUpper(parts[0]) {
		case "FROM":
			if len(parts) != 2 {
				return nil, fmt.Errorf("line %d: FROM requires exactly 1 argument", lineNum)
			}
			lf.Instructions = append(lf.Instructions, Instruction{Type: InstFrom, Args: []string{parts[1]}})

		case "MIGRATE":
			if len(parts) != 2 {
				return nil, fmt.Errorf("line %d: MIGRATE requires exactly 1 argument", lineNum)
			}
			lf.Instructions = append(lf.Instructions, Instruction{Type: InstMigrate, Args: []string{parts[1]}})

		case "COPY":
			if len(parts) != 3 {
				return nil, fmt.Errorf("line %d: COPY requires exactly 2 arguments", lineNum)
			}
			lf.Instructions = append(lf.Instructions, Instruction{Type: InstCopy, Args: []string{parts[1], parts[2]}})

		default:
			return nil, fmt.Errorf("line %d: unknown instruction %q", lineNum, parts[0])
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read layerfile: %w", err)
	}

	return lf, nil
}
```

- [ ] **Step 2: 验证编译**

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && go build ./...
```

Expected: 编译成功

---

### Task 2: 构建相关数据库操作

**Files:**
- Create: `iroll/db/build.go`

- [ ] **Step 1: 创建 db/build.go**

```go
package db

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type HistoryEntry struct {
	ID           int    `json:"id"`
	FromLayer    string `json:"from_layer"`
	Description  string `json:"description"`
	LayerID      string `json:"layer_id"`
	Instructions string `json:"instructions"`
	CreatedAt    string `json:"created_at"`
}

func ExecuteSQL(db *sql.DB, sqlPath string) error {
	content, err := ioutil.ReadFile(sqlPath)
	if err != nil {
		return fmt.Errorf("read sql file: %w", err)
	}

	_, err = db.Exec(string(content))
	if err != nil {
		return fmt.Errorf("execute sql %s: %w", filepath.Base(sqlPath), err)
	}
	return nil
}

func EnsureHistoryTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_layer TEXT,
			description TEXT NOT NULL,
			layer_id TEXT NOT NULL,
			instructions TEXT,
			created_at TEXT NOT NULL
		)
	`)
	return err
}

func InsertHistory(db *sql.DB, fromLayer string, description string, layerID string, instructions string) error {
	_, err := db.Exec(
		"INSERT INTO history (from_layer, description, layer_id, instructions, created_at) VALUES (?, ?, ?, ?, ?)",
		fromLayer, description, layerID, instructions, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func QueryHistory(db *sql.DB) ([]HistoryEntry, error) {
	rows, err := db.Query("SELECT id, from_layer, description, layer_id, instructions, created_at FROM history ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HistoryEntry
	for rows.Next() {
		var h HistoryEntry
		var fromLayer sql.NullString
		if err := rows.Scan(&h.ID, &fromLayer, &h.Description, &h.LayerID, &h.Instructions, &h.CreatedAt); err != nil {
			return nil, err
		}
		if fromLayer.Valid {
			h.FromLayer = fromLayer.String
		}
		result = append(result, h)
	}
	return result, nil
}

func ComputeLayerHash(buildDir string) (string, error) {
	var sb strings.Builder

	filepath.Walk(buildDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(buildDir, path)
		sb.WriteString(rel)
		sb.WriteString(fmt.Sprintf("%d", info.Size()))
		sb.WriteString(info.ModTime().String())
		return nil
	})

	return fmt.Sprintf("sha256:%x", hashString(sb.String())), nil
}

func hashString(s string) [32]byte {
	h := [32]byte{}
	copy(h[:], s)
	return h
}

func QueryTableStats(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + name).Scan(&count); err != nil {
			count = -1
		}
		stats[name] = count
	}
	return stats, nil
}

func QueryAllMetadata(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM metadata")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, nil
}

func ListResources(name string) ([]string, error) {
	resDir := filepath.Join(filepath.Join(filepath.Join(os.UserHomeDir(), ".iroll"), name), "Resources")
	var files []string
	filepath.Walk(resDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(resDir, path)
		files = append(files, rel)
		return nil
	})
	return files, nil
}
```

- [ ] **Step 2: 验证编译**

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && go build ./...
```

Expected: 编译成功

---

### Task 3: 构建引擎

**Files:**
- Create: `iroll/builder/build.go`

- [ ] **Step 1: 创建 builder/build.go**

```go
package builder

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"iroll/db"

	_ "github.com/mattn/go-sqlite3"
)

type LayerJSON struct {
	LayerID      string `json:"layer_id"`
	Parent       string `json:"parent,omitempty"`
	Description  string `json:"description"`
	CreatedAt    string `json:"created_at"`
	SchemaVersion int   `json:"schema_version"`
}

type BuildResult struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	LayerID string `json:"layer_id"`
}

func Build(lf *Layerfile, tagName string) (*BuildResult, error) {
	tmpDir, err := os.MkdirTemp("", "iroll-build-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	var parentLayerID string

	// Process instructions
	for _, inst := range lf.Instructions {
		switch inst.Type {
		case InstFrom:
			parentLayerID, err = processFrom(tmpDir, inst.Args[0])
			if err != nil {
				return nil, err
			}

		case InstMigrate:
			err = processMigrate(tmpDir, lf.Dir, inst.Args[0])
			if err != nil {
				return nil, err
			}

		case InstCopy:
			err = processCopy(tmpDir, lf.Dir, inst.Args[0], inst.Args[1])
			if err != nil {
				return nil, err
			}
		}
	}

	// Compute layer hash
	layerID, err := computeDirHash(tmpDir)
	if err != nil {
		return nil, err
	}

	// Write layer.json
	now := time.Now().UTC().Format(time.RFC3339Nano)
	lj := LayerJSON{
		LayerID:       layerID,
		Parent:        parentLayerID,
		Description:   fmt.Sprintf("build from Layerfile for %s", tagName),
		CreatedAt:     now,
		SchemaVersion: 1,
	}
	ljBytes, _ := json.MarshalIndent(lj, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "layer.json"), ljBytes, 0644); err != nil {
		return nil, err
	}

	// Record history in the database
	dbPath := filepath.Join(tmpDir, "ai_roll.db")
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := db.EnsureHistoryTable(conn); err != nil {
		return nil, err
	}

	instrSummary, _ := json.Marshal(lf.Instructions)
	if err := db.InsertHistory(conn, parentLayerID, lj.Description, layerID, string(instrSummary)); err != nil {
		return nil, err
	}

	// Pack into .iroll and load
	irollPath := filepath.Join(os.TempDir(), tagName+".iroll")
	if err := packToZip(tmpDir, irollPath); err != nil {
		return nil, err
	}
	defer os.Remove(irollPath)

	// Load into ~/.iroll/<name>/
	dest := filepath.Join(filepath.Join(os.UserHomeDir(), ".iroll"), tagName)
	if _, err := os.Stat(dest); err == nil {
		return nil, fmt.Errorf("iroll '%s' already exists", tagName)
	}
	if err := os.Rename(tmpDir, dest); err != nil {
		return nil, fmt.Errorf("move to store: %w", err)
	}

	// Rename succeeded, cancel cleanup
	os.MkdirTemp("", "dummy") // tmpDir already moved, RemoveAll on defer is harmless

	return &BuildResult{
		Name:    tagName,
		Path:    dest,
		LayerID: layerID,
	}, nil
}

func processFrom(tmpDir string, baseName string) (string, error) {
	src := filepath.Join(filepath.Join(os.UserHomeDir(), ".iroll"), baseName)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return "", fmt.Errorf("base iroll '%s' not found in ~/.iroll/", baseName)
	}

	// Copy entire directory
	if err := copyDir(src, tmpDir); err != nil {
		return "", fmt.Errorf("copy base layer: %w", err)
	}

	// Read parent layer_id from layer.json if exists
	ljPath := filepath.Join(tmpDir, "layer.json")
	if data, err := ioutil.ReadFile(ljPath); err == nil {
		var lj LayerJSON
		if json.Unmarshal(data, &lj) == nil {
			return lj.LayerID, nil
		}
	}
	return "", nil
}

func processMigrate(tmpDir string, lfDir string, sqlFile string) error {
	sqlPath := filepath.Join(lfDir, sqlFile)
	if _, err := os.Stat(sqlPath); os.IsNotExist(err) {
		return fmt.Errorf("sql file not found: %s", sqlPath)
	}

	dbPath := filepath.Join(tmpDir, "ai_roll.db")
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	return db.ExecuteSQL(conn, sqlPath)
}

func processCopy(tmpDir string, lfDir string, src string, dest string) error {
	srcPath := filepath.Join(lfDir, src)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("source not found: %s", src)
	}

	destPath := filepath.Join(tmpDir, dest)
	os.MkdirAll(filepath.Dir(destPath), 0755)

	return copyDir(srcPath, destPath)
}

func copyDir(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		os.MkdirAll(dst, 0755)
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyDir(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func computeDirHash(dir string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		h.Write([]byte(rel))
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		io.Copy(h, f)
		f.Close()
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

func packToZip(srcDir string, zipPath string) error {
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
```

- [ ] **Step 2: 验证编译**

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && go build ./...
```

Expected: 编译成功

---

### Task 4: build 命令

**Files:**
- Create: `iroll/cmd/build.go`

- [ ] **Step 1: 创建 cmd/build.go**

```go
package cmd

import (
	"iroll/builder"

	"github.com/spf13/cobra"
)

var buildFile string
var buildTag string

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an iroll from a Layerfile",
	Run: func(cmd *cobra.Command, args []string) {
		lf, err := builder.ParseLayerfile(buildFile)
		if err != nil {
			outputError(err.Error())
		}

		result, err := builder.Build(lf, buildTag)
		if err != nil {
			outputError(err.Error())
		}

		outputJSON(result)
	},
}

func init() {
	buildCmd.Flags().StringVarP(&buildFile, "file", "f", "Layerfile", "Layerfile path")
	buildCmd.Flags().StringVarP(&buildTag, "tag", "t", "", "Output iroll name")
	buildCmd.MarkFlagRequired("tag")

	rootCmd.AddCommand(buildCmd)
}
```

- [ ] **Step 2: 编译并测试 build 命令（基础层构建）**

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && go build -o iroll.exe .
```

Expected: 编译成功

创建测试用 Layerfile 和 SQL：

Run:
```bash
mkdir -p "d:/worklog/code/语料库/ai-roll-mini/examples/base-agent"

cat > "d:/worklog/code/语料库/ai-roll-mini/examples/base-agent/Layerfile" << 'LAYEREOF'
MIGRATE init_schema.sql
MIGRATE init_data.sql
COPY greeting.txt Resources/greeting.txt
LAYEREOF

cat > "d:/worklog/code/语料库/ai-roll-mini/examples/base-agent/init_schema.sql" << 'SQLEOF'
CREATE TABLE metadata (
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    remark TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE memory (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    importance REAL DEFAULT 0.5
);
CREATE TABLE context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    cwd TEXT,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
SQLEOF

cat > "d:/worklog/code/语料库/ai-roll-mini/examples/base-agent/init_data.sql" << 'SQLEOF'
INSERT INTO metadata (key, value, remark, created_at, updated_at) VALUES
    ('name', 'test-agent', 'agent name', datetime('now'), datetime('now')),
    ('version', '0.1.0', 'version', datetime('now'), datetime('now'));
INSERT INTO memory (content, created_at, importance) VALUES
    ('hello from base layer', datetime('now'), 0.9);
SQLEOF

echo "Hello from test-agent" > "d:/worklog/code/语料库/ai-roll-mini/examples/base-agent/greeting.txt"
```

运行构建：

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && ./iroll.exe build -f "../examples/base-agent/Layerfile" -t test-agent
```

Expected: `{"name":"test-agent","path":"<home>/.iroll/test-agent","layer_id":"sha256:..."}`

验证数据：

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && .venv/Scripts/python.exe -c "
import sqlite3
conn = sqlite3.connect('$HOME/.iroll/test-agent/ai_roll.db')
print('--- metadata ---')
for r in conn.execute('SELECT * FROM metadata'): print(r)
print('--- memory ---')
for r in conn.execute('SELECT * FROM memory'): print(r)
print('--- history ---')
for r in conn.execute('SELECT * FROM history'): print(r)
conn.close()
"
```

Expected: metadata 2 rows, memory 1 row, history 1 row

- [ ] **Step 3: 测试叠加层构建**

创建叠加层：

Run:
```bash
mkdir -p "d:/worklog/code/语料库/ai-roll-mini/examples/layer2"

cat > "d:/worklog/code/语料库/ai-roll-mini/examples/layer2/Layerfile" << 'LAYEREOF'
FROM test-agent
MIGRATE add_memory.sql
LAYEREOF

cat > "d:/worklog/code/语料库/ai-roll-mini/examples/layer2/add_memory.sql" << 'SQLEOF'
INSERT INTO memory (content, created_at, importance) VALUES ('layer2 memory', datetime('now'), 0.7);
SQLEOF
```

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && ./iroll.exe build -f "../examples/layer2/Layerfile" -t test-layer2
```

Expected: 成功，test-layer2 继承 test-agent 的所有数据 + 新增 layer2 的 memory

验证叠加层数据：

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && .venv/Scripts/python.exe -c "
import sqlite3
conn = sqlite3.connect('$HOME/.iroll/test-layer2/ai_roll.db')
print('memory count:', conn.execute('SELECT COUNT(*) FROM memory').fetchone()[0])
print('history count:', conn.execute('SELECT COUNT(*) FROM history').fetchone()[0])
for r in conn.execute('SELECT * FROM history'): print(r)
conn.close()
"
```

Expected: memory 2 rows, history 2 rows（含基础层和叠加层记录）

清理测试数据：

Run:
```bash
rm -rf ~/.iroll/test-agent ~/.iroll/test-layer2
```

- [ ] **Step 4: Commit**

```bash
cd "d:/worklog/code/语料库/ai-roll-mini" && git add iroll/ examples/ && git commit -m "feat: add iroll build command with Layerfile parser"
```

---

### Task 5: history 命令

**Files:**
- Create: `iroll/cmd/history.go`

- [ ] **Step 1: 创建 cmd/history.go**

```go
package cmd

import (
	"iroll/db"
	"iroll/store"

	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history <name>",
	Short: "Show build history",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		conn, err := db.Open(store.DbPath(name))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		entries, err := db.QueryHistory(conn)
		if err != nil {
			outputError(err.Error())
		}

		if entries == nil {
			entries = []db.HistoryEntry{}
		}
		outputJSON(entries)
	},
}

func init() {
	rootCmd.AddCommand(historyCmd)
}
```

- [ ] **Step 2: 编译并测试**

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && go build -o iroll.exe .
```

Expected: 编译成功

先构建一个测试包，然后查询历史：

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && ./iroll.exe build -f "../examples/base-agent/Layerfile" -t test-agent && ./iroll.exe history test-agent
```

Expected: 输出包含 1 条 history 记录的 JSON 数组

Run:
```bash
rm -rf ~/.iroll/test-agent
```

- [ ] **Step 3: Commit**

```bash
cd "d:/worklog/code/语料库/ai-roll-mini" && git add iroll/ && git commit -m "feat: add iroll history command"
```

---

### Task 6: inspect 命令

**Files:**
- Create: `iroll/cmd/inspect.go`

- [ ] **Step 1: 创建 cmd/inspect.go**

```go
package cmd

import (
	"iroll/db"
	"iroll/store"

	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <name>",
	Short: "Show iroll details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		conn, err := db.Open(store.DbPath(name))
		if err != nil {
			outputError(err.Error())
		}
		defer conn.Close()

		metadata, err := db.QueryAllMetadata(conn)
		if err != nil {
			outputError(err.Error())
		}

		tableStats, err := db.QueryTableStats(conn)
		if err != nil {
			outputError(err.Error())
		}

		resources, err := db.ListResources(name)
		if err != nil {
			resources = []string{}
		}

		outputJSON(map[string]interface{}{
			"name":      name,
			"metadata":  metadata,
			"tables":    tableStats,
			"resources": resources,
		})
	},
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}
```

- [ ] **Step 2: 编译并测试**

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && go build -o iroll.exe .
```

Expected: 编译成功

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll" && ./iroll.exe build -f "../examples/base-agent/Layerfile" -t test-agent && ./iroll.exe inspect test-agent
```

Expected: 输出包含 metadata、tables、resources 的 JSON

Run:
```bash
rm -rf ~/.iroll/test-agent
```

- [ ] **Step 3: Commit**

```bash
cd "d:/worklog/code/语料库/ai-roll-mini" && git add iroll/ && git commit -m "feat: add iroll inspect command"
```

---

### Task 7: 集成测试

**Files:**
- 无新文件，端到端验证

- [ ] **Step 1: 完整三层构建 + 所有命令测试**

Run:
```bash
cd "d:/worklog/code/语料库/ai-roll-mini/iroll"

# 1. 构建基础层
./iroll.exe build -f "../examples/base-agent/Layerfile" -t test-base
echo "=== build base ==="

# 2. 构建叠加层
./iroll.exe build -f "../examples/layer2/Layerfile" -t test-layer2
echo "=== build layer2 ==="

# 3. 查看历史
./iroll.exe history test-layer2
echo "=== history ==="

# 4. 查看详情
./iroll.exe inspect test-base
echo "=== inspect base ==="

./iroll.exe inspect test-layer2
echo "=== inspect layer2 ==="

# 5. 列出所有
./iroll.exe list
echo "=== list ==="

# 6. 验证叠加层继承了基础层的数据
.venv/Scripts/python.exe -c "
import sqlite3
conn = sqlite3.connect('$HOME/.iroll/test-layer2/ai_roll.db')
mem_count = conn.execute('SELECT COUNT(*) FROM memory').fetchone()[0]
hist_count = conn.execute('SELECT COUNT(*) FROM history').fetchone()[0]
print(f'memory: {mem_count} rows (expect 2)')
print(f'history: {hist_count} rows (expect 2)')
conn.close()
"
echo "=== verify ==="

# 7. 清理
rm -rf ~/.iroll/test-base ~/.iroll/test-layer2
```

Expected:
- build base 成功
- build layer2 成功，继承 test-base 的数据
- history 显示 2 条记录
- inspect 输出 metadata + tables + resources
- list 显示 test-base 和 test-layer2
- verify: memory 2 rows, history 2 rows

- [ ] **Step 2: Final commit**

```bash
cd "d:/worklog/code/语料库/ai-roll-mini" && git add -A && git commit -m "feat: complete iroll layered build system"
```
