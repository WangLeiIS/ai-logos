# CLI 三段式输出 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 改造 CLI 输出为三段式 JSON（status → data → hints），先改 `page new` 一个命令。

**Architecture:** root.go 新增 `outputOK`/`outputFail`/`Hint`/错误码常量，db.go 新增 `PageBrief`，page.go 的 `pageNewCmd` 改用新输出。旧函数保留不动。

**Tech Stack:** Go, Cobra CLI, encoding/json

---

### Task 1: root.go — 新输出函数 + Hint + 错误码

**Files:**
- Modify: `iroll/cmd/root.go`

- [ ] **Step 1: 新增 Hint struct、错误码常量、输出函数**

在 `root.go` 的现有 `import` 之后、`var Version` 之前，插入以下代码：

```go
// Hint is a structured suggestion for the next command.
type Hint struct {
	Action string `json:"action"`
	Cmd    string `json:"cmd"`
}

// Error codes for structured output.
const (
	ErrCodeInvalidTag    = "invalid_tag"
	ErrCodeIrollNotFound = "iroll_not_found"
	ErrCodeNoDefaultPage = "no_default_page"
	ErrCodePageNotFound  = "page_not_found"
	ErrCodeDBOpen        = "db_open_failed"
	ErrCodeInternal      = "internal"
)

func jsonLine(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// outputOK prints success three-line output to stdout.
// data and hints are optional (nil/empty = line skipped).
func outputOK(data interface{}, hints []Hint) {
	fmt.Println(jsonLine(map[string]string{"status": "ok"}))
	if data != nil {
		fmt.Println(jsonLine(data))
	}
	if len(hints) > 0 {
		fmt.Println(jsonLine(map[string]interface{}{"hints": hints}))
	}
}

// outputFail prints error three-line output to stdout, then os.Exit(1).
// hints is optional (nil/empty = line skipped).
func outputFail(code, errMsg string, hints []Hint) {
	fmt.Println(jsonLine(map[string]string{
		"status": "error",
		"code":   code,
		"error":  errMsg,
	}))
	if len(hints) > 0 {
		fmt.Println(jsonLine(map[string]interface{}{"hints": hints}))
	}
	os.Exit(1)
}
```

- [ ] **Step 2: Build 验证**

```bash
cd iroll && go build ./...
```

Expected: 编译成功（`outputOK`/`outputFail` 暂未被调用，不会报 unused 错误——它们在同一个 package 内，Go 允许未使用的导出函数）。

如果 `"fmt"` 和 `"encoding/json"` 尚未 import，编译会报错。检查 `root.go` 已有 import（`outputJSON` 和 `outputError` 已经在用），无需额外 import。

- [ ] **Step 3: Commit**

```bash
git add iroll/cmd/root.go
git commit -m "feat: add outputOK, outputFail, Hint, and error code constants"
```

---

### Task 2: db/db.go — PageBrief struct

**Files:**
- Modify: `iroll/db/db.go`

- [ ] **Step 1: 在 Page struct 定义后新增 PageBrief**

在 `db.go` 的 `Page` struct 定义之后（`type Page struct { ... }` 后面）插入：

```go
// PageBrief is a lightweight page representation without context,
// used for structured CLI output to encourage agent to call get-context.
type PageBrief struct {
	PageID    string `json:"page_id"`
	Cwd       string `json:"cwd"`
	Alias     string `json:"alias,omitempty"`
	CreatedAt string `json:"created_at"`
}
```

- [ ] **Step 2: Build 验证**

```bash
cd iroll && go build ./...
```

Expected: 编译成功。

- [ ] **Step 3: Commit**

```bash
git add iroll/db/db.go
git commit -m "feat: add PageBrief struct for lightweight page output"
```

---

### Task 3: page.go — pageNewCmd 三段式改造

**Files:**
- Modify: `iroll/cmd/page.go:102-158`

- [ ] **Step 1: 改造 pageNewCmd 的 Run 函数**

将 `pageNewCmd` 的 Run 函数中的所有 `outputError` 调用改为 `outputFail`，最后的 `outputJSON(p)` 改为 `outputOK`。

改造后的完整 `pageNewCmd`：

```go
var pageNewCmd = &cobra.Command{
	Use:   "new <iroll-name> [cwd]",
	Short: "Create a new page",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		name, version, err := builder.ParseTag(args[0])
		if err != nil {
			outputFail(ErrCodeInvalidTag, fmt.Sprintf("invalid tag: %v", err), []Hint{
				{Action: "List all available iroll packages", Cmd: "logos status --list"},
				{Action: "Build an iroll from an Irollfile", Cmd: "logos roll build -f <Irollfile> -t <name>"},
			})
		}
		cwd, outerPath, err := resolvePageNewCwd(name, version, args)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		innerPath := checkedInnerPath(name, version)

		// Copy outer template if not exists
		if _, err := os.Stat(outerPath); os.IsNotExist(err) {
			templateOuter := filepath.Join(checkedIrollPath(name, version), "roll-outer.db")
			if err := copyFile(templateOuter, outerPath); err != nil {
				outputFail(ErrCodeInternal, fmt.Sprintf("copy outer db template: %v", err), nil)
			}
		}

		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		defer conn.Close()

		p, err := db.InsertPage(conn, cwd)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}

		if _, err := db.AutoStartLoopSeeds(conn, p.PageID); err != nil {
			outputFail(ErrCodeInternal, "auto-start loop seeds: "+err.Error(), nil)
		}

		if err := store.IndexPage(name, version, p.PageID, cwd, outerPath, ""); err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}

		// Auto-set as default page when using workspace (no explicit cwd)
		if pageNewCwd == "" && len(args) < 2 {
			if err := store.SetDefaultPage(name, p.PageID); err != nil {
				outputFail(ErrCodeInternal, "set default page: "+err.Error(), nil)
			}
		}

		brief := &db.PageBrief{
			PageID:    p.PageID,
			Cwd:       p.Cwd,
			Alias:     p.Alias,
			CreatedAt: p.CreatedAt,
		}

		hints := []Hint{
			{Action: "Set an alias for this page, so you can reference it by name later", Cmd: fmt.Sprintf("logos page update-context --page %s --set-alias <name>", p.PageID)},
			{Action: "Get the full context including DNA, loops and system prompt", Cmd: fmt.Sprintf("logos page get-context --page %s", p.PageID)},
		}

		outputOK(brief, hints)
	},
}
```

- [ ] **Step 2: Build 验证**

```bash
cd iroll && go build ./...
```

Expected: 编译成功。

- [ ] **Step 3: 运行所有测试**

```bash
cd iroll && go test -count=1 ./...
```

Expected: 全部通过。现有的 cmd 测试（`TestPageNew` 等）可能需要调整——它们可能断言了旧的输出格式。如果有测试失败，需要更新测试。

- [ ] **Step 4: 手动验证三段输出**

```bash
# 成功场景
./logos page new cat
```

Expected 输出（3 行）：
```json
{"status":"ok"}
{"page_id":"...","cwd":"...","alias":"","created_at":"..."}
{"hints":[{"action":"Set an alias for this page, ...","cmd":"logos page update-context --page ... --set-alias <name>"},{"action":"Get the full context ...","cmd":"logos page get-context --page ..."}]}
```

```bash
# 失败场景
./logos page new not:exist
```

Expected 输出（2-3 行）：
```json
{"status":"error","code":"invalid_tag","error":"..."}
{"hints":[...]}
```

- [ ] **Step 5: Commit**

```bash
git add iroll/cmd/page.go
git commit -m "feat: convert page new to structured three-line output"
```

---

### Task 4: 测试修复（如有）

**Files:** `iroll/cmd/*_test.go` 中受影响的测试

- [ ] **Step 1: 运行测试找到失败的**

```bash
cd iroll && go test -count=1 ./cmd/... 2>&1
```

- [ ] **Step 2: 逐个修复**

如果 `TestPageNew` 等测试断言了旧的 `outputJSON`/`outputError` 输出格式，需要改为匹配新的三段式输出。

- [ ] **Step 3: 确认全部通过**

```bash
cd iroll && go test -count=1 ./...
```

- [ ] **Step 4: Commit**

```bash
git add iroll/cmd/*_test.go
git commit -m "test: update tests for structured three-line output"
```
