# page default + alias + get-context 统一入口 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) to implement this plan task-by-task.

**Goal:** 跨目录访问 page——通过 `--roll`（默认 page）、`--alias`（别名）、`--page`（ID）。

**Architecture:** system.db config 表存 per-iroll 默认 page；page_index 加 alias 列支持别名查找。get-context 按 `--page > --alias > --roll > --cwd` 优先级解析。

**Tech Stack:** Go, SQLite, Cobra CLI

---

### Task 1: Schema — pages + page_index 加 alias 列

**Files:**
- `examples/base-agent/init_inner.sql` — pages 表加 `alias TEXT`
- `examples/base-agent/init_outer.sql` — pages 表加 `alias TEXT`
- `iroll/store/system.go` — ensureSystemTables 加 alias migration，IndexPage 签名加 alias

**Steps:**

1. `init_inner.sql` 的 `CREATE TABLE pages` — `cwd TEXT` 后加 `alias TEXT,`
2. `init_outer.sql` 同上
3. `ensureSystemTables` 末尾加：检测 `page_index.alias` 列是否存在，不存在则 `ALTER TABLE page_index ADD COLUMN alias TEXT`
4. `IndexPage` 签名改为 `func IndexPage(irollName, version, pageID, cwd, outerDbPath, alias string) error`，INSERT 包含 alias
5. 更新所有 `IndexPage` 调用者传 `""`（暂不设 alias）
6. `go build ./...` 确保编译通过
7. Commit

---

### Task 2: Store 层 — default page + alias lookup 函数

**Files:**
- `iroll/store/system.go` — 新增 6 个函数

**新增函数：**

```go
// SetDefaultPage / GetDefaultPage / ClearDefaultPage — 操作 config 表
func SetDefaultPage(name, pageID string) error       // config.default_page:<name> = pageID
func GetDefaultPage(name string) (string, error)      // 查 config
func ClearDefaultPage(name string) error              // DELETE FROM config

// LookupPageByAlias / LookupPageByID / SetPageAlias — 操作 page_index
func LookupPageByAlias(alias string) (name, version, pageID, outerDbPath string, error)
func LookupPageByID(pageID string) (name, version, outerDbPath string, error)
func SetPageAlias(pageID, alias string) error         // UPDATE page_index SET alias
```

**Steps:**

1. 实现 6 个函数
2. `go build ./store/...`
3. Commit

---

### Task 3: CMD 层 — 命令适配

**Files:**
- `iroll/cmd/page.go` — 新增 `pageDefaultCmd`，`pageNewCmd` 自动设默认
- `iroll/cmd/context.go` — 重构 `resolvePage`，`get-context`/`update-context` 加 `--roll`/`--alias`/`--set-alias`

**核心变更 — `resolvePage` 重构为统一入口：**

```
--page → LookupPageByID → 打开
--alias → LookupPageByAlias → 打开
--roll → GetDefaultPage → LookupPageByID → 打开
args[0] (iroll name) → GetDefaultPage → 打开
无参数 → openOuterFromActive(cwd)
```

**Steps:**

1. `context.go`：重构 `resolvePage(args, flagPage, flagAlias, flagRoll, cwd)`，按优先级返回 `(name, version, pageID, *sql.DB)`
2. `context.go`：`getContextCmd` 加 `--page`/`--alias`/`--roll` flags；`updateContextCmd` 加 `--page`/`--alias`/`--roll`/`--set-alias` flags
3. `context.go`：`--set-alias` 时同时更新 iroll pages 表 + system.db page_index（调用 `store.SetPageAlias`）
4. `page.go`：新增 `pageDefaultCmd`（设/查/清默认 page）
5. `page.go`：`pageNewCmd` — workspace 模式（无显式 cwd）建完后自动 `store.SetDefaultPage`
6. `page.go`：注册 `pageDefaultCmd` 到 `pageCmd`
7. `go build ./cmd/...`
8. Commit

---

### Task 4: 测试修复

**Files:** 所有受影响的 `*_test.go`

**Steps:**

1. 更新所有 `IndexPage` 调用，加 alias 参数（传 `""`）
2. 适配新的 `resolvePage` 签名
3. `go test -count=1 ./...` 全部通过
4. Commit
