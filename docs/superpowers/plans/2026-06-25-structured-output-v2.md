# CLI 三段式输出 整体改造 V2 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 page 组剩余命令 + 高频运行时命令（loop_run, loop_seed, memory, evolving）全面改造为三段式 JSON 输出，删除多余的 page current，统一 root.go 辅助函数。

**Architecture:** 每个命令的 `outputJSON`/`outputError` 调用点 → `outputOK`/`outputFail` + 内联 hints。loop 命令因工厂模式只需改 output 函数体。`outputJSON`/`outputError` 保留不动（仍有 ~80 处其他命令调用）。

**Tech Stack:** Go, Cobra CLI, encoding/json

---

### Task 1: root.go — 新增错误码 + 改造辅助函数

**Files:**
- Modify: `iroll/cmd/root.go`

- [ ] **Step 1: 新增 ErrCodeNoActivePage**

在 `ErrCodeInternal` 后添加：

```go
ErrCodeNoActivePage = "no_active_page"
```

- [ ] **Step 2: 改造 checkedIrollPath**

将第 98 行 `outputError(err.Error())` 替换为：

```go
outputFail(ErrCodeInternal, err.Error(), nil)
```

- [ ] **Step 3: 改造 checkedInnerPath**

将第 111 行 `outputError(err.Error())` 替换为：

```go
outputFail(ErrCodeInternal, err.Error(), nil)
```

- [ ] **Step 4: 改造 openOuterFromActive**

将第 121 行和 126 行的两处 `outputError(...)` 替换为：

第 121 行（`store.GetActive` 失败 — 没有 active page）：
```go
outputFail(ErrCodeNoActivePage, err.Error(), []Hint{
    {Action: "Create a new page and auto-set it as the active page for this directory", Cmd: "logos page new <iroll-name>"},
    {Action: "List all pages to find an existing one", Cmd: "logos page list -a"},
})
```

第 126 行（db 打开失败）：
```go
outputFail(ErrCodeDBOpen, err.Error(), nil)
```

- [ ] **Step 5: Build 验证**

```bash
cd iroll && go build ./...
```

Expected: 编译成功。

- [ ] **Step 6: Commit**

```bash
git add iroll/cmd/root.go
git commit -m "feat: add ErrCodeNoActivePage, convert helper functions to outputFail"
```

---

### Task 2: page.go — 删除 page current + 剩余命令三段式

**Files:**
- Modify: `iroll/cmd/page.go`

#### 2a: 删除 page current

- [ ] **Step 1: 删除 pageCurrentCwd 变量声明（第 21 行）**

```go
// DELETE: var pageCurrentCwd string
```

- [ ] **Step 2: 删除 pageCurrentCmd 整个命令定义（第 23-44 行）**

删除从 `var pageCurrentCmd = &cobra.Command{` 到 `},` 的所有行。

- [ ] **Step 3: 删除 init() 中的 pageCurrentCmd 注册和 flag 绑定**

在 init() 中删除：
```go
pageCurrentCmd.Flags().StringVar(&pageCurrentCwd, "cwd", ".", "Working directory")
```
和
```go
pageCmd.AddCommand(pageCurrentCmd)
```

#### 2b: page list — 无参路径（第 54-68 行）

- [ ] **Step 4: 改造无参路径**

将 `outputError(err.Error())` 和 `outputJSON(pages)` 替换为：

```go
pages, err := store.ListAllPages(cwd)
if err != nil {
    outputFail(ErrCodeInternal, err.Error(), nil)
}
if pages == nil {
    pages = []map[string]interface{}{}
}
hints := []Hint{}
if len(pages) > 0 {
    first := pages[0]
    if pid, ok := first["page_id"].(string); ok {
        hints = append(hints, Hint{
            Action: "Get the full context for the first page listed",
            Cmd:    fmt.Sprintf("logos page get-context --page %s", pid),
        })
    }
}
hints = append(hints, Hint{
    Action: "Create a new page for a fresh context",
    Cmd: "logos page new <iroll-name>",
})
outputOK(pages, hints)
```

#### 2c: page list — 有参路径（第 70-99 行）

- [ ] **Step 5: 改造有参路径**

所有 `outputError(...)` → `outputFail(ErrCodeInternal, ..., nil)`（第 73 行 `invalid tag` 用 `ErrCodeInvalidTag`）。

`outputJSON(pages)` 替换为：将 `[]db.Page` 转为 `[]db.PageBrief`，然后用 `outputOK` + hints：

```go
name, version, err := builder.ParseTag(args[0])
if err != nil {
    outputFail(ErrCodeInvalidTag, fmt.Sprintf("invalid tag: %v", err), []Hint{
        {Action: "List all available iroll packages", Cmd: "logos status --list"},
    })
}
innerPath := checkedInnerPath(name, version)
outerPath, err := store.WorkspaceOuterDbPath(name, version)
if err != nil {
    outputFail(ErrCodeInternal, err.Error(), nil)
}
conn, err := db.OpenOuter(outerPath, innerPath)
if err != nil {
    outputFail(ErrCodeDBOpen, err.Error(), nil)
}
defer conn.Close()

var listCwd string
if !pageListAll {
    listCwd, _ = filepath.Abs(pageListCwd)
}
pages, err := db.ListPagesByCwd(conn, listCwd)
if err != nil {
    outputFail(ErrCodeInternal, err.Error(), nil)
}

briefs := make([]db.PageBrief, 0, len(pages))
for _, p := range pages {
    briefs = append(briefs, db.PageBrief{
        PageID:    p.PageID,
        Cwd:       p.Cwd,
        Alias:     p.Alias,
        CreatedAt: p.CreatedAt,
    })
}
hints := []Hint{}
if len(briefs) > 0 {
    hints = append(hints, Hint{
        Action: "Get the full context for the first page listed",
        Cmd:    fmt.Sprintf("logos page get-context --page %s", briefs[0].PageID),
    })
}
hints = append(hints, Hint{
    Action: "Create a new page for a fresh context",
    Cmd: fmt.Sprintf("logos page new %s", args[0]),
})
outputOK(briefs, hints)
```

#### 2d: page switch（第 170-187 行）

- [ ] **Step 6: 改造 page switch**

```go
var pageSwitchCmd = &cobra.Command{
    Use:   "switch <page-id>",
    Short: "Switch active page",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        pageID := args[0]
        irollName, _, err := store.SwitchPage(pageID)
        if err != nil {
            outputFail(ErrCodePageNotFound, err.Error(), nil)
        }
        outputOK(map[string]string{
            "active":     "true",
            "iroll_name": irollName,
            "page_id":    pageID,
        }, []Hint{
            {Action: "Get the full context of the newly active page", Cmd: fmt.Sprintf("logos page get-context --page %s", pageID)},
        })
    },
}
```

#### 2e: page delete（第 189-204 行）

- [ ] **Step 7: 改造 page delete**

```go
var pageDeleteCmd = &cobra.Command{
    Use:   "delete <page-id>",
    Short: "Delete a page",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        pageID := args[0]
        if err := store.DeletePage(pageID); err != nil {
            outputFail(ErrCodeInternal, err.Error(), nil)
        }
        outputOK(map[string]string{
            "deleted": "true",
            "page_id": pageID,
        }, []Hint{
            {Action: "List remaining pages", Cmd: "logos page list -a"},
            {Action: "Create a new page for a fresh context", Cmd: "logos page new <iroll-name>"},
        })
    },
}
```

#### 2f: page default（第 209-265 行）

- [ ] **Step 8: 改造 page default — set 路径（第 214-229 行）**

```go
// Set: logos page default <page-id>
pageID := args[0]
name, _, _, err := store.LookupPageByID(pageID)
if err != nil {
    outputFail(ErrCodePageNotFound, err.Error(), nil)
}
if err := store.SetDefaultPage(name, pageID); err != nil {
    outputFail(ErrCodeInternal, err.Error(), nil)
}
outputOK(map[string]string{
    "status":  "ok",
    "message": fmt.Sprintf("default page for '%s' set to %s", name, pageID),
}, []Hint{
    {Action: "Get the full context of the new default page", Cmd: fmt.Sprintf("logos page get-context --page %s", pageID)},
})
```

- [ ] **Step 9: 改造 page default — clear 路径（第 233-241 行）**

```go
if pageDefaultClear && pageDefaultRoll != "" {
    if err := store.ClearDefaultPage(pageDefaultRoll); err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(map[string]string{
        "status":  "ok",
        "message": fmt.Sprintf("default page for '%s' cleared", pageDefaultRoll),
    }, []Hint{
        {Action: "Set a new default page", Cmd: fmt.Sprintf("logos page default <page-id>")},
    })
    return
}
```

- [ ] **Step 10: 改造 page default — show 路径（第 244-261 行）**

```go
if pageDefaultRoll != "" {
    pageID, err := store.GetDefaultPage(pageDefaultRoll)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    if pageID == "" {
        outputOK(map[string]string{
            "iroll":        pageDefaultRoll,
            "default_page": "",
        }, []Hint{
            {Action: "Create a new page and auto-set it as default", Cmd: fmt.Sprintf("logos page new %s", pageDefaultRoll)},
            {Action: "List all pages to find one to set as default", Cmd: "logos page list -a"},
        })
        return
    }
    outputOK(map[string]string{
        "iroll":        pageDefaultRoll,
        "default_page": pageID,
    }, []Hint{
        {Action: "Get the full context of the default page", Cmd: fmt.Sprintf("logos page get-context --page %s", pageID)},
        {Action: "Clear the default page setting", Cmd: fmt.Sprintf("logos page default --roll %s --clear", pageDefaultRoll)},
    })
    return
}
```

- [ ] **Step 11: 改造 page default — error 路径（第 263 行）**

```go
outputFail(ErrCodeInternal, "usage: logos page default <page-id>  OR  logos page default --roll <name> [--clear]", nil)
```

#### 2g: 清理 import

- [ ] **Step 12: 检查 page.go 的 import**

删除了 page current（无 filepath 引用了？检查：page list 仍需要 filepath，page new 也需要。保留所有 import。`"io"` 是否还在用？`copyFile` 在 page new 中使用，保留。）

- [ ] **Step 13: Build 验证**

```bash
cd iroll && go build ./...
```

Expected: 编译成功。若 `fmt.Sprintf` 未 import，添加 `"fmt"` 到 import。

- [ ] **Step 14: Commit**

```bash
git add iroll/cmd/page.go
git commit -m "feat: convert page list/switch/delete/default to three-line output, remove page current"
```

---

### Task 3: query_dna.go — 三段式改造

**Files:**
- Modify: `iroll/cmd/query_dna.go`

- [ ] **Step 1: 改造 query-dna**

将 `outputError(err.Error())` 和 `outputJSON(results)` 替换为：

```go
run: func(cmd *cobra.Command, args []string) {
    name := args[0]

    cwd, _ := filepath.Abs(queryDnaCwd)
    conn, _, _, _ := openOuterFromActive(cwd)
    defer conn.Close()

    results, err := db.QueryDna(conn, name, queryDnaType)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }

    if results == nil {
        results = []db.Dna{}
    }

    hints := []Hint{}
    if len(results) > 0 {
        hints = append(hints, Hint{
            Action: "Use a DNA answer in your page context",
            Cmd:    fmt.Sprintf("logos page update-context --page <page-id> --content '{\"dna_answer\":\"%s\"}'", results[0].Answer),
        })
    }
    hints = append(hints, Hint{
        Action: "Get the full page context including DNA",
        Cmd: "logos page get-context",
    })
    outputOK(results, hints)
},
```

- [ ] **Step 2: 确保 import 中有 `"fmt"`**

当前 query_dna.go 无 `"fmt"` import，需添加（hints 中用了 `fmt.Sprintf`）。

- [ ] **Step 3: Build 验证**

```bash
cd iroll && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add iroll/cmd/query_dna.go
git commit -m "feat: convert query-dna to structured three-line output"
```

---

### Task 4: loop_run.go — 全部 output 函数三段式

**Files:**
- Modify: `iroll/cmd/loop_run.go`

每个 output 函数将 `outputError(err.Error())` → `outputFail(ErrCodeInternal, err.Error(), nil)`，`outputJSON(data)` → `outputOK(data, hints)`。

- [ ] **Step 1: 改造 outputLoopRun（第 211-218 行）**

```go
func outputLoopRun(cwd, seedName string, parentRunID *int64, plan string) error {
    run, err := runLoopStart(cwd, seedName, parentRunID, plan)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(run, []Hint{
        {Action: "Get the latest loop run details", Cmd: fmt.Sprintf("logos loop ps")},
        {Action: "Get the page context with active loop focus", Cmd: "logos page get-context"},
    })
    return nil
}
```

- [ ] **Step 2: 改造 outputLoopUpdate（第 220-227 行）**

```go
func outputLoopUpdate(cwd string, runID *int64, plan, progress *string) error {
    run, err := runLoopUpdate(cwd, runID, plan, progress)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(run, []Hint{
        {Action: "Get the page context with active loop focus", Cmd: "logos page get-context"},
    })
    return nil
}
```

- [ ] **Step 3: 改造 outputLoopComplete（第 229-236 行）**

```go
func outputLoopComplete(cwd string, runID *int64, result string) error {
    run, err := runLoopComplete(cwd, runID, result)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(run, []Hint{
        {Action: "Reflect on the completed run", Cmd: fmt.Sprintf("logos loop reflect %d --content <reflection>", run.ID)},
        {Action: "List all loop runs", Cmd: "logos loop ps -a"},
        {Action: "Start a new loop run", Cmd: "logos loop run <seed-name>"},
    })
    return nil
}
```

- [ ] **Step 4: 改造 outputLoopAbort（第 238-245 行）**

```go
func outputLoopAbort(cwd string, runID *int64, reason, result string) error {
    run, err := runLoopAbort(cwd, runID, reason, result)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(run, []Hint{
        {Action: "Reflect on the aborted run", Cmd: fmt.Sprintf("logos loop reflect %d --content <reflection>", run.ID)},
        {Action: "Start a new loop run with a different seed", Cmd: "logos loop run <seed-name>"},
        {Action: "List all loop seeds", Cmd: "logos loop list"},
    })
    return nil
}
```

- [ ] **Step 5: 改造 outputLoopReflect（第 247-254 行）**

```go
func outputLoopReflect(cwd string, runID int64, content string) error {
    run, err := runLoopReflect(cwd, runID, content)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(run, []Hint{
        {Action: "View run history for this seed", Cmd: "logos loop history <seed-name>"},
        {Action: "List all loop runs", Cmd: "logos loop ps -a"},
    })
    return nil
}
```

- [ ] **Step 6: 改造 outputLoopPs（第 256-266 行）**

```go
func outputLoopPs(cwd string, all bool) error {
    runs, err := runLoopPs(cwd, all)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    if runs == nil {
        runs = []db.LoopRun{}
    }
    outputOK(runs, []Hint{
        {Action: "Start a new loop run from a seed", Cmd: "logos loop run <seed-name>"},
        {Action: "List available loop seeds", Cmd: "logos loop list"},
    })
    return nil
}
```

- [ ] **Step 7: 改造 outputLoopHistory（第 280-287 行）**

```go
func outputLoopHistory(cwd, seedName, pageID string, limit int) error {
    runs, err := runLoopHistory(cwd, seedName, pageID, limit)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(runs, []Hint{
        {Action: "Start a new loop run from this seed", Cmd: fmt.Sprintf("logos loop run %s", seedName)},
        {Action: "Inspect the loop seed", Cmd: fmt.Sprintf("logos loop inspect %s", seedName)},
    })
    return nil
}
```

- [ ] **Step 8: 改造 outputLoopShow（第 289-296 行）**

```go
func outputLoopShow(cwd string, runID int64) error {
    run, err := runLoopShow(cwd, runID)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(run, []Hint{
        {Action: "Update this run's plan or progress", Cmd: fmt.Sprintf("logos loop update %d --plan <json>", runID)},
        {Action: "Complete this run with a result", Cmd: fmt.Sprintf("logos loop complete %d --result <json>", runID)},
    })
    return nil
}
```

- [ ] **Step 9: 在 loop_run.go 添加 `"fmt"` import**

当前 import 为 `"fmt", "strconv", "strings"` → 已有 `"fmt"`，无需改动。

- [ ] **Step 10: Build 验证**

```bash
cd iroll && go build ./...
```

- [ ] **Step 11: Commit**

```bash
git add iroll/cmd/loop_run.go
git commit -m "feat: convert all loop-run output functions to three-line output"
```

---

### Task 5: loop_seed.go — 全部 output 函数三段式

**Files:**
- Modify: `iroll/cmd/loop_seed.go`

- [ ] **Step 1: 改造 outputLoopList（第 144-159 行）**

```go
func outputLoopList(cwd string, includeArchived bool, stats bool) error {
    if stats {
        seeds, err := runLoopListStats(cwd, includeArchived)
        if err != nil {
            outputFail(ErrCodeInternal, err.Error(), nil)
        }
        outputOK(seeds, []Hint{
            {Action: "Start a loop run from an available seed", Cmd: "logos loop run <seed-name>"},
            {Action: "Inspect a loop seed for details", Cmd: "logos loop inspect <name>"},
        })
        return nil
    }
    seeds, err := runLoopList(cwd, includeArchived)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    hints := []Hint{}
    if len(seeds) > 0 {
        hints = append(hints, Hint{
            Action: "Start a loop run from this seed",
            Cmd:    fmt.Sprintf("logos loop run %s", seeds[0].Name),
        })
    }
    hints = append(hints, Hint{
        Action: "Add a new loop seed",
        Cmd: "logos loop add <name> --describe <desc> --content <json>",
    })
    outputOK(seeds, hints)
    return nil
}
```

- [ ] **Step 2: 改造 outputLoopInspect（第 170-177 行）**

```go
func outputLoopInspect(cwd, name string) error {
    seed, err := runLoopInspect(cwd, name)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(seed, []Hint{
        {Action: "Start a loop run from this seed", Cmd: fmt.Sprintf("logos loop run %s", name)},
        {Action: "Edit this seed's configuration", Cmd: fmt.Sprintf("logos loop edit %s --describe <desc>", name)},
        {Action: "List all loop seeds", Cmd: "logos loop list"},
    })
    return nil
}
```

- [ ] **Step 3: 改造 outputLoopAdd（第 179-186 行）**

```go
func outputLoopAdd(cwd, name, seedType, describe, content string, weight float64) error {
    seed, err := runLoopAdd(cwd, name, seedType, describe, content, weight)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(seed, []Hint{
        {Action: "Start a loop run from the new seed", Cmd: fmt.Sprintf("logos loop run %s", name)},
        {Action: "List all loop seeds", Cmd: "logos loop list"},
    })
    return nil
}
```

- [ ] **Step 4: 改造 outputLoopEdit（第 188-195 行）**

```go
func outputLoopEdit(cwd, name string, patch db.LoopSeedPatch) error {
    seed, err := runLoopEdit(cwd, name, patch)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(seed, []Hint{
        {Action: "Start a loop run from the updated seed", Cmd: fmt.Sprintf("logos loop run %s", name)},
        {Action: "Inspect the updated seed", Cmd: fmt.Sprintf("logos loop inspect %s", name)},
    })
    return nil
}
```

- [ ] **Step 5: 改造 outputLoopRemove（第 197-203 行）**

```go
func outputLoopRemove(cwd, name string) error {
    if err := runLoopRemove(cwd, name); err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(map[string]string{"removed": name}, []Hint{
        {Action: "List remaining loop seeds", Cmd: "logos loop list"},
        {Action: "Add a new loop seed", Cmd: "logos loop add <name> --describe <desc> --content <json>"},
    })
    return nil
}
```

- [ ] **Step 6: 改造 outputLoopArchive（第 205-212 行）**

```go
func outputLoopArchive(cwd, name string) error {
    seed, err := runLoopArchive(cwd, name)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(seed, []Hint{
        {Action: "List seeds including archived", Cmd: "logos loop list --archived"},
        {Action: "Restore this seed if needed", Cmd: fmt.Sprintf("logos loop restore %s", name)},
    })
    return nil
}
```

- [ ] **Step 7: 改造 outputLoopRestore（第 214-221 行）**

```go
func outputLoopRestore(cwd, name string) error {
    seed, err := runLoopRestore(cwd, name)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    outputOK(seed, []Hint{
        {Action: "Start a loop run from the restored seed", Cmd: fmt.Sprintf("logos loop run %s", name)},
        {Action: "List all loop seeds", Cmd: "logos loop list"},
    })
    return nil
}
```

- [ ] **Step 8: 在 loop_seed.go 添加 `"fmt"` import**

当前 import 为 `"logos/db"` + cobra/pflag，需添加 `"fmt"`。

- [ ] **Step 9: Build 验证**

```bash
cd iroll && go build ./...
```

- [ ] **Step 10: Commit**

```bash
git add iroll/cmd/loop_seed.go
git commit -m "feat: convert all loop-seed output functions to three-line output"
```

---

### Task 6: memory.go — query-memory 三段式

**Files:**
- Modify: `iroll/cmd/memory.go`

- [ ] **Step 1: 改造 queryMemoryCmd**

将 `Run` 函数在 `conn, _, _, pageID` 行后，对 `openOuterFromActive` 的错误处理（目前无——调用无忧，因为 openOuterFromActive 内部 outputError 已 exit。现在 openOuterFromActive 已改为 outputFail，它会 exit。所以这行之后代码只会执行到成功路径）。

改造输出部分：

```go
results, err := db.QueryMemory(conn, pageID, params)
if err != nil {
    outputFail(ErrCodeInternal, err.Error(), nil)
}

hints := []Hint{
    {Action: "Get the full page context", Cmd: "logos page get-context"},
    {Action: "Query memory with a keyword", Cmd: "logos page query-memory --keyword <keyword>"},
}

if queryMemoryFull {
    if results == nil {
        results = []db.Memory{}
    }
    outputOK(results, hints)
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
    outputOK(summaries, hints)
}
```

- [ ] **Step 2: Build 验证**

```bash
cd iroll && go build ./...
```

Expected: 编译成功。

- [ ] **Step 3: Commit**

```bash
git add iroll/cmd/memory.go
git commit -m "feat: convert query-memory to structured three-line output"
```

---

### Task 7: evolving.go — evolving run 三段式

**Files:**
- Modify: `iroll/cmd/evolving.go`

- [ ] **Step 1: 改造 runEvolving（第 35-71 行）**

所有 `outputError(msg)` → `outputFail(ErrCodeInternal, msg, nil)`，`outputJSON(results)` → `outputOK(results, nil)`。

完整改造后：

```go
func runEvolving(cmd *cobra.Command, args []string) {
    name, version := resolveEvolvingTarget(args)
    innerPath := checkedInnerPath(name, version)

    outerPath, err := store.WorkspaceOuterDbPath(name, version)
    if err != nil {
        outputFail(ErrCodeInternal, err.Error(), nil)
    }
    if _, err := os.Stat(outerPath); os.IsNotExist(err) {
        templateOuter := filepath.Join(checkedIrollPath(name, version), "roll-outer.db")
        if err := copyFile(templateOuter, outerPath); err != nil {
            outputFail(ErrCodeInternal, fmt.Sprintf("copy outer db template: %v", err), nil)
        }
    }

    sql := resolveEvolvingSQL(args)
    if sql == "" || strings.TrimSpace(sql) == "" {
        outputFail(ErrCodeInternal, "no SQL provided (use --sql, positional args, --file, or stdin)", nil)
    }

    conn, err := db.OpenOuter(outerPath, innerPath)
    if err != nil {
        outputFail(ErrCodeDBOpen, err.Error(), nil)
    }
    defer conn.Close()

    results, err := db.ExecuteAll(conn, sql, evolvingDryRun)
    if err != nil {
        if len(results) > 0 {
            outputOK(results, nil)
        }
        outputFail(ErrCodeInternal, err.Error(), nil)
    }

    outputOK(results, nil)
}
```

- [ ] **Step 2: 改造 resolveEvolvingTarget（第 75-93 行）**

两处 `outputError` → `outputFail`：

第 86 行（cwd 解析失败）：
```go
outputFail(ErrCodeInternal, fmt.Sprintf("resolve cwd: %v", err), nil)
```

第 90 行（GetActive 失败）：
```go
outputFail(ErrCodeNoActivePage, err.Error(), []Hint{
    {Action: "Create a new page for this directory", Cmd: "logos page new <iroll-name>"},
    {Action: "List all pages", Cmd: "logos page list -a"},
})
```

- [ ] **Step 3: 改造 resolveEvolvingSQL（第 112-149 行）**

第 133 行（读取文件失败）：
```go
outputFail(ErrCodeInternal, fmt.Sprintf("read file %q: %v", evolvingFile, err), nil)
```

第 143 行（读取 stdin 失败）：
```go
outputFail(ErrCodeInternal, fmt.Sprintf("read stdin: %v", err), nil)
```

- [ ] **Step 4: Build 验证**

```bash
cd iroll && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add iroll/cmd/evolving.go
git commit -m "feat: convert evolving to structured three-line output"
```

---

### Task 8: 测试修复 + 全量验证

**Files:**
- Modify: `iroll/cmd/*_test.go`（如有需要）

- [ ] **Step 1: 运行全量测试**

```bash
cd iroll && go test -count=1 ./...
```

- [ ] **Step 2: 修复失败的测试**

若 `TestPageCurrent` 被删除，在测试中移除对应用例。
若任何测试断言了旧的 `outputJSON` 格式（如 `{"error":"msg"}` → stderr），更新断言为新格式（`{"status":"error","code":"...","error":"msg"}` → stdout）。

- [ ] **Step 3: 确认全部通过**

```bash
cd iroll && go test -count=1 ./... 2>&1
```

Expected: PASS for all packages.

- [ ] **Step 4: Commit**

```bash
git add iroll/cmd/*_test.go
git commit -m "test: update tests for structured three-line output v2"
```

---

### 完成验证

- [ ] **Step 1: 手动验证关键命令**

```bash
# page list three-line
./logos page list -a

# page switch three-line
./logos page switch <page-id>

# page delete three-line
./logos page delete <page-id>

# page default three-line
./logos page default <page-id>

# loop ps three-line
./logos loop ps

# memory query three-line
./logos page query-memory
```

Expected: 每个命令输出 2-3 行 JSON（status + data + hints）。

- [ ] **Step 2: 验证删除 page current**

```bash
./logos page current 2>&1
```

Expected: "unknown command" from cobra.
