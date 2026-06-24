# Version-Aware Global Index Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `iroll_version` to `page_index` and `active_page` tables, thread version through every store function and cmd caller, eliminate all `"latest"` hardcodes that should use the stored version.

**Architecture:** Change happens in two layers — store (data access signatures) then cmd (all callers). Store layer changes first (breaking signature changes), then all cmd callers and tests are updated to match.

**Tech Stack:** Go, SQLite (go-sqlite3), Cobra CLI

---

### Task 1: store/system.go — Schema and IndexPage

**Files:**
- Modify: `iroll/store/system.go:33-81`

- [ ] **Step 1: Add `iroll_version` to CREATE TABLE statements**

Edit `ensureSystemTables()`. Add `iroll_version TEXT NOT NULL DEFAULT 'latest'` column to both `page_index` and `active_page`:

```go
func ensureSystemTables(db *sql.DB) error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS page_index (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            iroll_name TEXT NOT NULL,
            iroll_version TEXT NOT NULL DEFAULT 'latest',
            page_id TEXT NOT NULL,
            cwd TEXT NOT NULL,
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS active_page (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            cwd TEXT NOT NULL UNIQUE,
            iroll_name TEXT NOT NULL,
            iroll_version TEXT NOT NULL DEFAULT 'latest',
            page_id TEXT NOT NULL,
            updated_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS config (
            key TEXT PRIMARY KEY,
            value TEXT NOT NULL
        )
    `)
    return err
}
```

- [ ] **Step 2: Update `IndexPage` signature and INSERTs**

Change signature from `IndexPage(irollName, pageID, cwd)` to `IndexPage(irollName, version, pageID, cwd)` and add version to both INSERTs:

```go
func IndexPage(irollName string, version string, pageID string, cwd string) error {
    db, err := OpenSystem()
    if err != nil {
        return err
    }
    defer db.Close()

    now := time.Now().UTC().Format(time.RFC3339Nano)

    _, err = db.Exec(
        "INSERT INTO page_index (iroll_name, iroll_version, page_id, cwd, created_at) VALUES (?, ?, ?, ?, ?)",
        irollName, version, pageID, cwd, now,
    )
    if err != nil {
        return fmt.Errorf("index page: %w", err)
    }

    _, err = db.Exec(`
        INSERT INTO active_page (cwd, iroll_name, iroll_version, page_id, updated_at) VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(cwd) DO UPDATE SET iroll_name=excluded.iroll_name, iroll_version=excluded.iroll_version, page_id=excluded.page_id, updated_at=excluded.updated_at
    `, cwd, irollName, version, pageID, now)
    return err
}
```

- [ ] **Step 3: Verify compilation fails (callers not yet updated)**

```bash
cd iroll && go build ./store/...
```

Expected: store package compiles (no callers here), but `go build ./...` will fail due to cmd callers.

---

### Task 2: store/system.go — GetActive, ListAllPages

**Files:**
- Modify: `iroll/store/system.go:83-143`

- [ ] **Step 1: Update `ListAllPages` to SELECT and return `iroll_version`**

```go
func ListAllPages(cwd string) ([]map[string]interface{}, error) {
    db, err := OpenSystem()
    if err != nil {
        return nil, err
    }
    defer db.Close()

    query := `
        SELECT p.iroll_name, p.iroll_version, p.page_id, p.cwd, p.created_at,
            CASE WHEN a.page_id IS NOT NULL THEN 1 ELSE 0 END AS active
        FROM page_index p
        LEFT JOIN active_page a ON p.cwd = a.cwd AND p.page_id = a.page_id
    `
    var rows *sql.Rows
    if cwd != "" {
        query += " WHERE p.cwd = ? ORDER BY p.created_at DESC"
        rows, err = db.Query(query, cwd)
    } else {
        query += " ORDER BY p.created_at DESC"
        rows, err = db.Query(query)
    }
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var result []map[string]interface{}
    for rows.Next() {
        var name, version, pid, c, t string
        var active int
        if err := rows.Scan(&name, &version, &pid, &c, &t, &active); err != nil {
            return nil, err
        }
        result = append(result, map[string]interface{}{
            "iroll_name":    name,
            "iroll_version": version,
            "page_id":       pid,
            "cwd":           c,
            "created_at":    t,
            "active":        active == 1,
        })
    }
    return result, nil
}
```

- [ ] **Step 2: Update `GetActive` to return version**

Change signature from `(string, string, error)` to `(string, string, string, error)`:

```go
func GetActive(cwd string) (string, string, string, error) {
    db, err := OpenSystem()
    if err != nil {
        return "", "", "", err
    }
    defer db.Close()

    var name, version, pid string
    err = db.QueryRow("SELECT iroll_name, iroll_version, page_id FROM active_page WHERE cwd = ?", cwd).Scan(&name, &version, &pid)
    if err == sql.ErrNoRows {
        return "", "", "", fmt.Errorf("no active page for cwd '%s', run 'logos page new <name>' first", cwd)
    }
    return name, version, pid, err
}
```

- [ ] **Step 3: Build store package**

```bash
cd iroll && go build ./store/...
```

---

### Task 3: store/system.go — DeletePage, SwitchPage, CleanIndex

**Files:**
- Modify: `iroll/store/system.go:145-231`

- [ ] **Step 1: Update `DeletePage` to read version from index**

In `DeletePage`, change the SELECT at line 155 to also read `iroll_version`, and use it in `DbPath`:

```go
func DeletePage(pageID string) error {
    sdb, err := OpenSystem()
    if err != nil {
        return err
    }
    defer sdb.Close()

    var irollName, irollVersion string
    err = sdb.QueryRow("SELECT iroll_name, iroll_version FROM page_index WHERE page_id = ?", pageID).Scan(&irollName, &irollVersion)
    if err == sql.ErrNoRows {
        return fmt.Errorf("page '%s' not found in index", pageID)
    }
    if err != nil {
        return err
    }

    dbPath, err := DbPath(irollName, irollVersion)
    if err != nil {
        return err
    }
    // ... rest unchanged (rolldb.Open, rolldb.DeletePage, tx commits)
    conn, err := rolldb.Open(dbPath)
    if err != nil {
        return err
    }
    defer conn.Close()
    if err := rolldb.DeletePage(conn, pageID); err != nil && !errors.Is(err, rolldb.ErrPageNotFound) {
        return err
    }

    tx, err := sdb.Begin()
    if err != nil {
        return fmt.Errorf("begin deleting page %q from index: %w", pageID, err)
    }
    defer tx.Rollback()
    if _, err := tx.Exec("DELETE FROM page_index WHERE page_id = ?", pageID); err != nil {
        return fmt.Errorf("delete page %q from index: %w", pageID, err)
    }
    if _, err := tx.Exec("DELETE FROM active_page WHERE page_id = ?", pageID); err != nil {
        return fmt.Errorf("clear active page %q: %w", pageID, err)
    }
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("commit deleting page %q from index: %w", pageID, err)
    }
    return nil
}
```

- [ ] **Step 2: Update `SwitchPage` to read/return/upsert version**

Change signature from `(string, error)` to `(string, string, error)`:

```go
func SwitchPage(pageID string) (string, string, error) {
    db, err := OpenSystem()
    if err != nil {
        return "", "", err
    }
    defer db.Close()

    var irollName, irollVersion, cwd string
    err = db.QueryRow("SELECT iroll_name, iroll_version, cwd FROM page_index WHERE page_id = ?", pageID).Scan(&irollName, &irollVersion, &cwd)
    if err == sql.ErrNoRows {
        return "", "", fmt.Errorf("page '%s' not found in index", pageID)
    }
    if err != nil {
        return "", "", err
    }

    now := time.Now().UTC().Format(time.RFC3339Nano)
    _, err = db.Exec(`
        INSERT INTO active_page (cwd, iroll_name, iroll_version, page_id, updated_at) VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(cwd) DO UPDATE SET iroll_name=excluded.iroll_name, iroll_version=excluded.iroll_version, page_id=excluded.page_id, updated_at=excluded.updated_at
    `, cwd, irollName, irollVersion, pageID, now)
    if err != nil {
        return "", "", err
    }
    return irollName, irollVersion, nil
}
```

- [ ] **Step 3: CleanIndex unchanged** (signature stays the same)

- [ ] **Step 4: Build store package**

```bash
cd iroll && go build ./store/...
```

---

### Task 4: cmd/page.go — pageNew, pageCurrent, pageList, pageSwitch

**Files:**
- Modify: `iroll/cmd/page.go`

- [ ] **Step 1: Update `pageNewCmd` — pass version to `IndexPage`**

Change line 121 from `store.IndexPage(name, p.PageID, cwd)` to:

```go
if err := store.IndexPage(name, version, p.PageID, cwd); err != nil {
```

- [ ] **Step 2: Update `pageCurrentCmd` — destructure version from GetActive**

Change lines 23-32 from:
```go
irollName, pageID, err := store.GetActive(cwd)
```
to:
```go
irollName, irollVersion, pageID, err := store.GetActive(cwd)
```
And change `checkedDbPath(irollName, "latest")` to `checkedDbPath(irollName, irollVersion)`.

- [ ] **Step 3: Update `pageListCmd` — ParseTag when name from CLI arg**

The `pageListCmd` Run function (lines 59-96). When `len(args) == 1`, the name should be parsed:

```go
name, version, err := builder.ParseTag(args[0])
if err != nil {
    outputError(fmt.Sprintf("invalid tag: %v", err))
}
conn, err := db.Open(checkedDbPath(name, version))
```
(already done in earlier bugfix — verify it's still correct)

- [ ] **Step 4: Update `pageSwitchCmd` — destructure version from SwitchPage**

Change line 135 from:
```go
irollName, err := store.SwitchPage(pageID)
```
to:
```go
irollName, _, err := store.SwitchPage(pageID)
```

- [ ] **Step 5: Build check**

```bash
cd iroll && go build ./cmd/...
```

---

### Task 5: cmd/context.go — resolvePage with version

**Files:**
- Modify: `iroll/cmd/context.go`

- [ ] **Step 1: Update `resolvePage` to return `(name, version, pageID)`**

Replace the entire `resolvePage` function:

```go
func resolvePage(args []string, flagPage string, cwd string) (string, string, string) {
    if len(args) > 0 {
        name, version, err := builder.ParseTag(args[0])
        if err != nil {
            outputError(fmt.Sprintf("invalid tag: %v", err))
        }
        return name, version, flagPage
    }
    name, version, pageID, err := store.GetActive(cwd)
    if err != nil {
        outputError(err.Error())
    }
    if flagPage != "" {
        return name, version, flagPage
    }
    return name, version, pageID
}
```

Add `"fmt"` and `"logos/builder"` to imports if not already present.

- [ ] **Step 2: Update `getContextCmd` — use version**

Change line 24 from:
```go
name, pageID := resolvePage(args, getContextPage, cwd)
```
to:
```go
name, version, pageID := resolvePage(args, getContextPage, cwd)
```
Change `checkedDbPath(name, "latest")` to `checkedDbPath(name, version)` and `checkedIrollPath(name, "latest")` to `checkedIrollPath(name, version)`.

- [ ] **Step 3: Update `updateContextCmd` — use version**

Same pattern: destructure `name, version, pageID`, use `checkedDbPath(name, version)`.

- [ ] **Step 4: Build check**

```bash
cd iroll && go build ./cmd/...
```

---

### Task 6: cmd/loop.go — openActiveLoop with version

**Files:**
- Modify: `iroll/cmd/loop.go`

- [ ] **Step 1: Destructure version from GetActive, use in DbPath**

Replace `openActiveLoop` function body (lines 41-59):

```go
func openActiveLoop(cwd string) (string, string, *sql.DB, error) {
    absoluteCwd, err := filepath.Abs(cwd)
    if err != nil {
        return "", "", nil, fmt.Errorf("resolve cwd: %w", err)
    }
    name, version, pageID, err := store.GetActive(absoluteCwd)
    if err != nil {
        return "", "", nil, err
    }
    dbPath, err := store.DbPath(name, version)
    if err != nil {
        return "", "", nil, err
    }
    conn, err := db.Open(dbPath)
    if err != nil {
        return "", "", nil, err
    }
    return name, pageID, conn, nil
}
```

- [ ] **Step 2: Build check**

```bash
cd iroll && go build ./cmd/...
```

---

### Task 7: cmd/memory.go, cmd/query_dna.go — GetActive destructure

**Files:**
- Modify: `iroll/cmd/memory.go`
- Modify: `iroll/cmd/query_dna.go`

- [ ] **Step 1: Update `memory.go`**

Change line 26 from:
```go
irollName, pageID, err := store.GetActive(cwd)
```
to:
```go
irollName, irollVersion, pageID, err := store.GetActive(cwd)
```
Change `checkedDbPath(irollName, "latest")` to `checkedDbPath(irollName, irollVersion)`.

- [ ] **Step 2: Update `query_dna.go`**

Change line 23 from:
```go
irollName, _, err := store.GetActive(cwd)
```
to:
```go
irollName, irollVersion, _, err := store.GetActive(cwd)
```
Change `checkedDbPath(irollName, "latest")` to `checkedDbPath(irollName, irollVersion)`.

- [ ] **Step 3: Build check**

```bash
cd iroll && go build ./cmd/...
```

---

### Task 8: cmd/book.go — resolveBookRoll + openBookDB with version

**Files:**
- Modify: `iroll/cmd/book.go`

- [ ] **Step 1: Update `resolveBookRoll` to return `(name, version, error)`**

```go
func resolveBookRoll(cwd string, names []string) (string, string, error) {
    if len(names) > 1 {
        return "", "", fmt.Errorf("at most one iroll name may be specified")
    }
    if len(names) == 1 {
        name, version, err := builder.ParseTag(names[0])
        if err != nil {
            return "", "", fmt.Errorf("invalid tag: %w", err)
        }
        if _, err := store.IrollPath(name, version); err != nil {
            return "", "", err
        }
        return name, version, nil
    }
    absoluteCwd, err := filepath.Abs(cwd)
    if err != nil {
        return "", "", fmt.Errorf("resolve cwd: %w", err)
    }
    name, version, _, err := store.GetActive(absoluteCwd)
    return name, version, err
}
```

Add `"logos/builder"` to imports if not present.

- [ ] **Step 2: Update `openBookDB` to accept `(name, version)`**

```go
func openBookDB(name, version string) (*sql.DB, error) {
    path, err := store.DbPath(name, version)
    if err != nil {
        return nil, err
    }
    return db.Open(path)
}
```

- [ ] **Step 3: Update all caller functions: `runBookList`, `runBookInspect`, `runBookQuery`**

In each, change `resolveBookRoll` destructure to get version, pass to `openBookDB`, and in `runBookQuery` use version for `store.IrollPath(name, version)`:

`runBookList`:
```go
func runBookList(cwd string, names []string) ([]book.Book, error) {
    name, version, err := resolveBookRoll(cwd, names)
    if err != nil {
        return nil, err
    }
    conn, err := openBookDB(name, version)
    ...
}
```

`runBookInspect`:
```go
func runBookInspect(cwd, bookID string, names []string) (*book.Book, error) {
    name, version, err := resolveBookRoll(cwd, names)
    if err != nil {
        return nil, err
    }
    conn, err := openBookDB(name, version)
    ...
}
```

`runBookQuery`:
```go
func runBookQuery(ctx context.Context, cwd string, query book.Query) (*book.QueryResponse, error) {
    ...
    name, version, err := resolveBookRoll(cwd, nil)
    ...
    conn, err := openBookDB(name, version)
    ...
    rollRoot, err := store.IrollPath(name, version)
    ...
}
```

- [ ] **Step 4: Build check**

```bash
cd iroll && go build ./cmd/...
```

---

### Task 9: cmd/skill.go — resolveSkillRoll + openSkillDB with version

**Files:**
- Modify: `iroll/cmd/skill.go`

- [ ] **Step 1: Update `resolveSkillRoll` to return `(name, version, error)`**

```go
func resolveSkillRoll(cwd string, names []string) (string, string, error) {
    if len(names) > 1 {
        return "", "", fmt.Errorf("at most one iroll name may be specified")
    }
    if len(names) == 1 {
        name, version, err := builder.ParseTag(names[0])
        if err != nil {
            return "", "", fmt.Errorf("invalid tag: %w", err)
        }
        if _, err := store.IrollPath(name, version); err != nil {
            return "", "", err
        }
        return name, version, nil
    }
    absoluteCwd, err := filepath.Abs(cwd)
    if err != nil {
        return "", "", fmt.Errorf("resolve cwd: %w", err)
    }
    name, version, _, err := store.GetActive(absoluteCwd)
    return name, version, err
}
```

Add `"logos/builder"` to imports.

- [ ] **Step 2: Update `openSkillDB` to accept `(name, version)`**

```go
func openSkillDB(name, version string) (*sql.DB, error) {
    path, err := store.DbPath(name, version)
    if err != nil {
        return nil, err
    }
    return db.Open(path)
}
```

- [ ] **Step 3: Update `skillListCmd` and `skillShowCmd`**

In both, destructure version from `resolveSkillRoll`, pass to `openSkillDB` and `store.IrollPath(name, version)`:

`skillListCmd`:
```go
name, version, err := resolveSkillRoll(skillListCwd, args)
...
conn, err := openSkillDB(name, version)
...
rollRoot, err := store.IrollPath(name, version)
```

`skillShowCmd`:
```go
name, version, err := resolveSkillRoll(skillShowCwd, args[1:])
...
conn, err := openSkillDB(name, version)
...
rollRoot, err := store.IrollPath(name, version)
```

- [ ] **Step 4: Build all**

```bash
cd iroll && go build ./...
```

Expected: clean build, zero errors.

---

### Task 10: Test updates — store/system_test.go

**Files:**
- Modify: `iroll/store/system_test.go`

- [ ] **Step 1: Update `setupDeletePageStoreTest` — add version to `IndexPage` call**

Change line 174 from:
```go
if err := IndexPage("test-roll", page.PageID, "/work"); err != nil {
```
to:
```go
if err := IndexPage("test-roll", "latest", page.PageID, "/work"); err != nil {
```

- [ ] **Step 2: Update `GetActive` callers — destructure 3 values**

Line 35: `if _, _, err := GetActive("/work"); err == nil {` — already `_, _, err`, no change needed (third `_` now absorbs version).

Line 58: `name, activePageID, err := GetActive("/work")` → `name, _, activePageID, err := GetActive("/work")`.

- [ ] **Step 3: Run store tests**

```bash
cd iroll && go test ./store/... -v -run TestDelete
```

Expected: all 4 DeletePage tests pass.

---

### Task 11: Test updates — e2e/testenv/setup.go

**Files:**
- Modify: `iroll/e2e/testenv/setup.go`

- [ ] **Step 1: Update `CreatePage` — add version param**

```go
func (e *Env) CreatePage(name, version, pageID, cwd string) (*db.Page, error) {
    e.t.Helper()
    conn, err := e.DB(name)
    if err != nil {
        return nil, err
    }
    page, err := db.InsertPage(conn, cwd)
    if err != nil {
        return nil, err
    }
    if err := store.IndexPage(name, version, page.PageID, cwd); err != nil {
        return nil, err
    }
    return page, nil
}
```

- [ ] **Step 2: Build check**

```bash
cd iroll && go build ./e2e/...
```

Expected: fails — e2e tests call `CreatePage` with old signature.

---

### Task 12: Test updates — e2e/scenario_page_test.go

**Files:**
- Modify: `iroll/e2e/scenario_page_test.go`

- [ ] **Step 1: Update all `CreatePage` calls — add version argument**

Line 69: `env.CreatePage("page-test", "", cwd)` → `env.CreatePage("page-test", "latest", "", cwd)`.

- [ ] **Step 2: Update all `IndexPage` calls — add version "latest"**

Line 138: `store.IndexPage("page-test", page1.PageID, cwd)` → `store.IndexPage("page-test", "latest", page1.PageID, cwd)`.
Line 156: `store.IndexPage("page-test", page2.PageID, cwd)` → `store.IndexPage("page-test", "latest", page2.PageID, cwd)`.
Line 373: `store.IndexPage("page-test", page.PageID, cwd)` → `store.IndexPage("page-test", "latest", page.PageID, cwd)`.

- [ ] **Step 3: Update `GetActive` callers — destructure 3 values**

Line 74: `name, pageID, err := store.GetActive(cwd)` → `name, version, pageID, err := store.GetActive(cwd)`. Add version check: `if version != "latest" { t.Fatalf(...) }`.

Line 143: `name, activeID, err := store.GetActive(cwd)` → `name, _, activeID, err := store.GetActive(cwd)`.

Line 171: `name, activeID, err = store.GetActive(cwd)` → `name, _, activeID, err = store.GetActive(cwd)`.

Line 383-384: `name, activeID, err := store.GetActive(cwd)` → `name, _, activeID, err := store.GetActive(cwd)`.

Line 402: `if _, _, err := store.GetActive(cwd); err == nil {` — already discards all values, no change needed.

- [ ] **Step 4: Update `SwitchPage` callers — destructure 2 values**

Line 161: `if _, err := store.SwitchPage(page1.PageID); err != nil {` — already discards first value, now discards both name and version, no change needed.

Line 166: Same pattern.

- [ ] **Step 5: Run e2e page tests**

```bash
cd iroll && go test ./e2e/... -v -run TestPage
```

Expected: all page e2e tests pass.

---

### Task 13: Test updates — cmd/book_test.go, cmd/loop_test.go

**Files:**
- Modify: `iroll/cmd/book_test.go`
- Modify: `iroll/cmd/loop_test.go`

- [ ] **Step 1: Update `book_test.go` — add version to `IndexPage` call**

Line 121: `store.IndexPage(rollName, "page-one", cwd)` → `store.IndexPage(rollName, "latest", "page-one", cwd)`.

- [ ] **Step 2: Update `loop_test.go` — add version to `IndexPage` call**

Line 498: `store.IndexPage("test-roll", "page-one", absoluteCwd)` → `store.IndexPage("test-roll", "latest", "page-one", absoluteCwd)`.

- [ ] **Step 3: Run cmd tests**

```bash
cd iroll && go test ./cmd/... -v -run "TestBook|TestLoop"
```

Expected: tests pass.

---

### Task 14: Full build and test

**Files:**
- No changes, verification only.

- [ ] **Step 1: Build everything**

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

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat: add iroll_version to global index, thread version through all store/cmd layers

- page_index and active_page tables now store iroll_version
- IndexPage/GetActive/SwitchPage/DeletePage all use version
- All cmd callers pass version instead of hardcoding 'latest'
- Tests updated for new signatures

Co-Authored-By: Claude <noreply@anthropic.com>"
```
