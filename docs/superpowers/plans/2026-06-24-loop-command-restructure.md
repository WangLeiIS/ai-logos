# Loop Command Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove `loop current`, add `loop ps [-a]`, enhance `loop list --stats`, split `BuildLoopContext` into `ListActiveRuns` + `listAvailableLoopSeeds`, and update context resolution to output `loop_focus` + `loop_available`.

**Architecture:** DB layer gets a new exported `ListActiveRuns(pageID)` function (extracted from `BuildLoopContext`). `BuildLoopContext` and `LoopContextView` are removed. Cmd layer adds `loop ps` command, enhances `loop list` with `--stats` flag, removes `loop current`. Context resolution injects two separate fields instead of one merged `loop` object.

**Tech Stack:** Go, SQLite (go-sqlite3), Cobra CLI

---

### Task 1: db/loop_context.go — Split BuildLoopContext

**Files:**
- Modify: `iroll/db/loop_context.go`

- [ ] **Step 1: Add exported `ListActiveRuns` function**

Replace `listActivePageLoopRuns` (unexported) with exported `ListActiveRuns(pageID)`. Also add `ListAllRuns(pageID)` for `loop ps -a`. Keep `listAvailableLoopSeeds` exported.

Add at top of loop_context.go (after imports):

```go
// ListActiveRuns returns all active loop runs for a page, main run first then children.
func ListActiveRuns(conn *sql.DB, pageID string) ([]LoopRun, error) {
    rows, err := conn.Query(`
        SELECT `+loopRunColumns+`
        FROM loop_runs
        WHERE page_id = ? AND status = 'active'
        ORDER BY CASE WHEN parent_run_id IS NULL THEN 0 ELSE 1 END, id
    `, pageID)
    if err != nil {
        return nil, fmt.Errorf("list active runs for page %q: %w", pageID, err)
    }
    defer rows.Close()
    return scanLoopRuns(rows)
}

// ListAllRuns returns all loop runs (any status) for a page, newest first.
func ListAllRuns(conn *sql.DB, pageID string) ([]LoopRun, error) {
    rows, err := conn.Query(`
        SELECT `+loopRunColumns+`
        FROM loop_runs
        WHERE page_id = ?
        ORDER BY id DESC
    `, pageID)
    if err != nil {
        return nil, fmt.Errorf("list all runs for page %q: %w", pageID, err)
    }
    defer rows.Close()
    return scanLoopRuns(rows)
}

func scanLoopRuns(rows *sql.Rows) ([]LoopRun, error) {
    runs := make([]LoopRun, 0)
    for rows.Next() {
        run, err := scanLoopRun(rows)
        if err != nil {
            return nil, err
        }
        runs = append(runs, *run)
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }
    return runs, nil
}
```

- [ ] **Step 2: Remove `BuildLoopContext`, `LoopContextView`, `LoopSeedStats`, `AvailableLoopSeed`**

Delete these types and function from loop_context.go:
- `LoopSeedStats` struct
- `AvailableLoopSeed` struct
- `LoopContextView` struct (with Focus and Available)
- `BuildLoopContext` function
- `listActivePageLoopRuns` function (replaced by `ListActiveRuns`)

Keep: `availableLoopSeedsQuery` constant and `listAvailableLoopSeeds` (it's still used by `loop list --stats`).

- [ ] **Step 3: Export `ListAvailableLoopSeeds`**

Rename `listAvailableLoopSeeds` → `ListAvailableLoopSeeds` (exported, for use by cmd layer and context resolution). Keep `AvailableLoopSeed` and `LoopSeedStats` if they're still needed by `ListAvailableLoopSeeds`.

Wait — `AvailableLoopSeed` and `LoopSeedStats` are return types of `listAvailableLoopSeeds`. These must stay. Only `LoopContextView` and `BuildLoopContext` are removed. `AvailableLoopSeed` stays.

Actually, re-reading the spec more carefully, the available seeds are still needed for `loop list --stats`. So we keep:
- `LoopSeedStats` struct
- `AvailableLoopSeed` struct
- `availableLoopSeedsQuery` + `ListAvailableLoopSeeds` (exported)

Remove only:
- `LoopContextView` struct
- `BuildLoopContext` function
- `listActivePageLoopRuns` (replaced by `ListActiveRuns`)

- [ ] **Step 4: Build check**

```bash
cd iroll && go build ./db/...
```

Expected: db compiles. `go build ./...` will fail because callers reference `BuildLoopContext`.

- [ ] **Step 5: Commit**

```bash
git add iroll/db/loop_context.go
git commit -m "refactor: split BuildLoopContext into ListActiveRuns + ListAllRuns

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: cmd/loop.go — Remove loop current, register loop ps

**Files:**
- Modify: `iroll/cmd/loop.go`

- [ ] **Step 1: Remove `newLoopCurrentCmd` from command registration**

In `newLoopCmd()`, remove the line:
```go
newLoopCurrentCmd(outputLoopCurrent),
```

- [ ] **Step 2: Add `newLoopPsCmd` registration**

Add before `newLoopHistoryCmd`:
```go
newLoopPsCmd(outputLoopPs),
```

- [ ] **Step 3: Build check (will fail, other files reference removed functions)**

```bash
cd iroll && go build ./cmd/...
```

Expected: fails — `outputLoopCurrent` and `runLoopCurrent` still referenced in loop_run.go.

---

### Task 3: cmd/loop_run.go — Add loop ps, remove loop current

**Files:**
- Modify: `iroll/cmd/loop_run.go`

- [ ] **Step 1: Add `newLoopPsCmd` function**

Insert after the imports, before `parseLoopRunID`:

```go
func newLoopPsCmd(run func(string, bool) error) *cobra.Command {
    var cwd string
    var all bool
    command := &cobra.Command{
        Use:   "ps",
        Short: "List loop runs for the current page",
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            defer resetLoopSeedFlags(cmd)
            return run(cwd, all)
        },
    }
    command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
    command.Flags().BoolVarP(&all, "all", "a", false, "Show all runs including completed/aborted")
    isolateLoopSeedCommand(command)
    return command
}
```

- [ ] **Step 2: Add `outputLoopPs` and `runLoopPs`**

Add after existing `outputLoopCurrent`:

```go
func outputLoopPs(cwd string, all bool) error {
    runs, err := runLoopPs(cwd, all)
    if err != nil {
        outputError(err.Error())
    }
    if runs == nil {
        runs = []db.LoopRun{}
    }
    outputJSON(runs)
    return nil
}

func runLoopPs(cwd string, all bool) ([]db.LoopRun, error) {
    _, pageID, conn, err := openActiveLoop(cwd)
    if err != nil {
        return nil, err
    }
    defer conn.Close()
    if all {
        return db.ListAllRuns(conn, pageID)
    }
    return db.ListActiveRuns(conn, pageID)
}
```

- [ ] **Step 3: Remove `newLoopCurrentCmd`, `outputLoopCurrent`, `runLoopCurrent`**

Delete these three functions entirely.

- [ ] **Step 4: Build check**

```bash
cd iroll && go build ./cmd/...
```

Expected: loop_run.go compiles, but callers of `BuildLoopContext` in db.go still fail.

---

### Task 4: cmd/loop_seed.go — Add --stats to loop list

**Files:**
- Modify: `iroll/cmd/loop_seed.go`

- [ ] **Step 1: Update `newLoopListCmd` to add `--stats` flag**

In `newLoopListCmd`:
```go
func newLoopListCmd(run func(string, bool, bool) error) *cobra.Command {
    var cwd string
    var includeArchived bool
    var stats bool
    command := &cobra.Command{
        Use:   "list",
        Short: "List loop seeds",
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            defer resetLoopSeedFlags(cmd)
            return run(cwd, includeArchived, stats)
        },
    }
    command.Flags().BoolVar(&includeArchived, "archived", false, "Include archived loop seeds")
    command.Flags().BoolVar(&stats, "stats", false, "Include run statistics per seed")
    command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
    isolateLoopSeedCommand(command)
    return command
}
```

- [ ] **Step 2: Update `outputLoopList` to pass stats flag**

```go
func outputLoopList(cwd string, includeArchived bool, stats bool) error {
    if stats {
        seeds, err := runLoopListStats(cwd, includeArchived)
        if err != nil {
            outputError(err.Error())
        }
        outputJSON(seeds)
        return nil
    }
    seeds, err := runLoopList(cwd, includeArchived)
    if err != nil {
        outputError(err.Error())
    }
    outputJSON(seeds)
    return nil
}
```

- [ ] **Step 3: Add `runLoopListStats`**

```go
func runLoopListStats(cwd string, includeArchived bool) ([]db.AvailableLoopSeed, error) {
    _, _, conn, err := openActiveLoop(cwd)
    if err != nil {
        return nil, err
    }
    defer conn.Close()
    seeds, err := db.ListAvailableLoopSeeds(conn, includeArchived)
    if err != nil {
        return nil, err
    }
    if seeds == nil {
        seeds = []db.AvailableLoopSeed{}
    }
    return seeds, nil
}
```

Note: `db.ListAvailableLoopSeeds` currently doesn't have `includeArchived` parameter. Need to update it.

- [ ] **Step 4: Update `db.ListAvailableLoopSeeds` to accept `includeArchived`**

In `iroll/db/loop_context.go`, change:
```go
func ListAvailableLoopSeeds(conn *sql.DB, includeArchived bool) ([]AvailableLoopSeed, error) {
```
And modify the query to optionally include archived seeds: when `!includeArchived`, add `WHERE loop.archived_at IS NULL`.

- [ ] **Step 5: Build check and commit**

```bash
cd iroll && go build ./cmd/...
```

---

### Task 5: db/db.go — Update context resolution

**Files:**
- Modify: `iroll/db/db.go:226-230`

- [ ] **Step 1: Replace `loop` field with `loop_focus` + `loop_available`**

Replace:
```go
loop, err := BuildLoopContext(db, pageID)
if err != nil {
    return "", err
}
resolved["loop"] = loop
```

With:
```go
focus, err := ListActiveRuns(db, pageID)
if err != nil {
    return "", err
}
if focus == nil {
    focus = []LoopRun{}
}
resolved["loop_focus"] = focus

available, err := ListAvailableLoopSeeds(db, false)
if err != nil {
    return "", err
}
if available == nil {
    available = []AvailableLoopSeed{}
}
resolved["loop_available"] = available
```

- [ ] **Step 2: Build all**

```bash
cd iroll && go build ./...
```

Expected: clean build, zero errors.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat: split loop context into loop_focus + loop_available

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 6: Update all tests

**Files:**
- Modify: `iroll/db/loop_context_test.go`
- Modify: `iroll/cmd/loop_integration_test.go`
- Modify: `iroll/e2e/scenario_integration_test.go`

- [ ] **Step 1: Update `loop_context_test.go`**

All tests call `BuildLoopContext`. They need to be rewritten:
- `TestBuildLoopContextIsPageScopedWithGlobalAvailableStats` → `TestListActiveRunsIsPageScoped` — test `ListActiveRuns` separately, test `ListAvailableLoopSeeds` separately
- `TestBuildLoopContextSelectsLatestEndAcrossOverlappingRuns` → test `ListAvailableLoopSeeds` stats correctness
- Similar for the other tests

For each test:
1. Replace `BuildLoopContext(conn, pageID)` calls
2. For focus tests: use `ListActiveRuns(conn, pageID)` and assert on the returned slice
3. For available tests: use `ListAvailableLoopSeeds(conn, false)` and assert on stats

- [ ] **Step 2: Update `loop_integration_test.go`**

Lines 58-62: Replace `db.BuildLoopContext` with `db.ListActiveRuns`.

- [ ] **Step 3: Update `scenario_integration_test.go`**

Lines 394-410: Replace `db.BuildLoopContext` with `db.ListActiveRuns` + `db.ListAvailableLoopSeeds`.

- [ ] **Step 4: Run all tests**

```bash
cd iroll && go test ./...
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: update tests for loop command restructure

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 7: Final build and verify

**Files:**
- No changes, verification only.

- [ ] **Step 1: Full build**

```bash
cd iroll && go build ./...
```

- [ ] **Step 2: Run all tests**

```bash
cd iroll && go test ./...
```

- [ ] **Step 3: Build binary**

```bash
cd iroll && go build -ldflags "-X logos/cmd.Version=0.1.0" -o ../logos .
```

- [ ] **Step 4: Commit if any final cleanup needed**
