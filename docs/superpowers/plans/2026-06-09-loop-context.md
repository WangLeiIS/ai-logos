# Loop Context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the old global todo-style loop table with reusable loop seeds, page-independent loop runs, dynamic loop context injection, and CLI commands for seed and run lifecycle management.

**Architecture:** The `db` package owns schema migration, seed CRUD, run lifecycle rules, aggregate views, and dynamic context construction. The `cmd` package exposes thin Cobra commands that resolve the active page and delegate to `db`. Raw `pages.context` never stores loop runtime state; `ResolveContext` injects a reserved `loop` view from `loop` and `loop_runs`.

**Tech Stack:** Go 1.24, Cobra, SQLite/go-sqlite3, standard `encoding/json`, existing Logos page/store patterns

---

## File Structure

Create focused database files rather than expanding the already broad `db/db.go`:

- `iroll/db/loop_types.go` — public loop seed/run/context types and JSON-or-text normalization.
- `iroll/db/loop_schema.go` — idempotent schema creation and migration from the existing todo-style `loop` table.
- `iroll/db/loop_seed.go` — seed CRUD, archive/restore, deletion rules, and aggregate seed listing.
- `iroll/db/loop_run.go` — run creation, updates, lifecycle transitions, history, reflection, and page cleanup.
- `iroll/db/loop_context.go` — constructs the dynamic `loop.focus` and `loop.available` context view.
- `iroll/cmd/loop.go` — root `logos loop` command and active-page resolution helper.
- `iroll/cmd/loop_seed.go` — seed management commands.
- `iroll/cmd/loop_run.go` — run lifecycle and history commands.

Modify:

- `iroll/db/db.go` — inject dynamic loop view during context resolution.
- `iroll/cmd/page.go` and `iroll/cmd/context.go` — pass `page_id` into context resolution.
- `iroll/store/system.go` — abort active runs transactionally before deleting a page.
- `examples/base-agent/init_schema.sql` and `examples/base-agent/init_data.sql` — build new rolls with the new schema and seed shape.
- `README.md`, `docs/rebot-roll.md`, and `skills/logos-1/skill.md` — document the approved behavior and commands.

## Task 1: Add Loop Schema and Migrate Existing Rolls

**Files:**
- Create: `iroll/db/loop_schema.go`
- Create: `iroll/db/loop_schema_test.go`
- Modify: `examples/base-agent/init_schema.sql`
- Modify: `examples/base-agent/init_data.sql`

- [ ] **Step 1: Write failing schema creation and migration tests**

Create tests that verify a fresh database receives both tables and indexes, and that the existing loop schema migrates while preserving seed identity fields:

```go
func TestEnsureLoopSchemaCreatesCurrentSchema(t *testing.T) {
	conn := openLoopTestDB(t)
	if err := EnsureLoopSchema(conn); err != nil {
		t.Fatal(err)
	}
	assertTableColumns(t, conn, "loop", []string{
		"id", "name", "describe", "content", "weight", "archived_at", "created_at", "updated_at",
	})
	assertTableColumns(t, conn, "loop_runs", []string{
		"id", "loop_id", "page_id", "parent_run_id",
		"seed_name", "seed_describe", "seed_content", "seed_weight",
		"status", "plan", "progress", "result", "reflection", "abort_reason",
		"started_at", "ended_at", "reflected_at", "updated_at",
	})
}

func TestEnsureLoopSchemaMigratesLegacyLoopSeeds(t *testing.T) {
	conn := openLoopTestDB(t)
	_, err := conn.Exec(`
		CREATE TABLE loop (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			describe TEXT NOT NULL,
			content TEXT NOT NULL,
			status TEXT NOT NULL,
			executed_count INTEGER DEFAULT 0,
			result TEXT DEFAULT '',
			weight REAL DEFAULT 0.5,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		INSERT INTO loop VALUES (
			7, 'once', 'self-cognition', '自我认知', 'Read context',
			'pending', 3, 'old result', 0.9, 'created', 'updated'
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := EnsureLoopSchema(conn); err != nil {
		t.Fatal(err)
	}
	var id int64
	var name, describe, content string
	var weight float64
	if err := conn.QueryRow(`SELECT id, name, describe, content, weight FROM loop`).Scan(
		&id, &name, &describe, &content, &weight,
	); err != nil {
		t.Fatal(err)
	}
	if id != 7 || name != "self-cognition" || describe != "自我认知" ||
		content != "Read context" || weight != 0.9 {
		t.Fatalf("migrated seed = %d %q %q %q %v", id, name, describe, content, weight)
	}
}
```

- [ ] **Step 2: Run the schema tests to verify they fail**

Run:

```bash
cd iroll
go test ./db -run 'TestEnsureLoopSchema' -v
```

Expected: FAIL because `EnsureLoopSchema` and the test helpers do not exist.

- [ ] **Step 3: Implement idempotent schema creation and legacy migration**

Implement:

```go
func EnsureLoopSchema(conn *sql.DB) error
```

Use one transaction:

1. Read `PRAGMA table_info(loop)`.
2. If `loop` does not exist, create the approved seed table.
3. If `loop` exists without `archived_at`, rename it to `loop_legacy`, create the new table, copy `id/name/describe/content/weight/created_at/updated_at`, then drop `loop_legacy`.
4. Create `loop_runs` and all approved indexes with `IF NOT EXISTS`.
5. Commit only after every statement succeeds.

Use these exact migration statements after creating the replacement `loop` table:

```sql
INSERT INTO loop (id, name, describe, content, weight, archived_at, created_at, updated_at)
SELECT id, name, describe, content, weight, NULL, created_at, updated_at
FROM loop_legacy;

DROP TABLE loop_legacy;
```

Use these exact schema statements:

```sql
CREATE TABLE loop (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    describe TEXT NOT NULL,
    content TEXT NOT NULL,
    weight REAL NOT NULL DEFAULT 0.5 CHECK (weight >= 0 AND weight <= 1),
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS loop_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    loop_id INTEGER NOT NULL,
    page_id TEXT NOT NULL,
    parent_run_id INTEGER,
    seed_name TEXT NOT NULL,
    seed_describe TEXT NOT NULL,
    seed_content TEXT NOT NULL,
    seed_weight REAL NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'aborted')),
    plan TEXT NOT NULL DEFAULT 'null',
    progress TEXT NOT NULL DEFAULT 'null',
    result TEXT NOT NULL DEFAULT 'null',
    reflection TEXT NOT NULL DEFAULT 'null',
    abort_reason TEXT,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    reflected_at TEXT,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (loop_id) REFERENCES loop(id),
    FOREIGN KEY (parent_run_id) REFERENCES loop_runs(id)
);

CREATE INDEX IF NOT EXISTS idx_loop_runs_page_status
ON loop_runs(page_id, status);

CREATE INDEX IF NOT EXISTS idx_loop_runs_parent_status
ON loop_runs(parent_run_id, status);

CREATE INDEX IF NOT EXISTS idx_loop_runs_loop_started
ON loop_runs(loop_id, started_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_loop_runs_one_active_main
ON loop_runs(page_id)
WHERE status = 'active' AND parent_run_id IS NULL;
```

- [ ] **Step 4: Update the base-agent SQL**

Replace the old `loop` definition in `examples/base-agent/init_schema.sql` with the approved `loop` and `loop_runs` schema and indexes. Replace legacy seed inserts in `init_data.sql` with:

```sql
INSERT INTO loop (name, describe, content, weight, archived_at, created_at, updated_at) VALUES
    ('self-cognition', '自我认知', '阅读所有 context 和 dna，了解自己的身份', 0.9, NULL, datetime('now'), datetime('now')),
    ('daily-check', '日常检查', '检查 dna 和 memory，决定当前需要关注的事项', 0.8, NULL, datetime('now'), datetime('now'));
```

- [ ] **Step 5: Run schema tests**

Run:

```bash
cd iroll
go test ./db -run 'TestEnsureLoopSchema' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add iroll/db/loop_schema.go iroll/db/loop_schema_test.go examples/base-agent/init_schema.sql examples/base-agent/init_data.sql
git commit -m "feat: add loop seed and run schema"
```

## Task 2: Add Loop Types, JSON Values, and Seed CRUD

**Files:**
- Create: `iroll/db/loop_types.go`
- Create: `iroll/db/loop_seed.go`
- Create: `iroll/db/loop_seed_test.go`

- [ ] **Step 1: Write failing seed CRUD and JSON normalization tests**

Cover:

```go
func TestNormalizeLoopJSONPreservesJSONAndWrapsText(t *testing.T) {
	if got, err := NormalizeLoopJSON(`{"step":1}`); err != nil || got != `{"step":1}` {
		t.Fatalf("JSON = %q, %v", got, err)
	}
	if got, err := NormalizeLoopJSON(`review memory`); err != nil || got != `"review memory"` {
		t.Fatalf("text = %q, %v", got, err)
	}
}

func TestLoopSeedLifecycle(t *testing.T) {
	conn := openLoopTestDB(t)
	mustEnsureLoopSchema(t, conn)
	seed, err := InsertLoopSeed(conn, "review", "Review memory", "Inspect useful memories", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := UpdateLoopSeed(conn, seed.Name, LoopSeedPatch{
		Content: ptr("Inspect memories and contradictions"),
		Weight:  ptr(0.9),
	})
	if err != nil || updated.Content != "Inspect memories and contradictions" || updated.Weight != 0.9 {
		t.Fatalf("updated = %#v, %v", updated, err)
	}
	if _, err := ArchiveLoopSeed(conn, seed.Name); err != nil {
		t.Fatal(err)
	}
	if got, err := ListLoopSeeds(conn, false); err != nil || len(got) != 0 {
		t.Fatalf("active seeds = %#v, %v", got, err)
	}
	if _, err := RestoreLoopSeed(conn, seed.Name); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLoopSeed(conn, seed.Name); err != nil {
		t.Fatal(err)
	}
}
```

Also test duplicate names, blank name/describe/content, weight outside `0..1`, and refusal to remove a seed after a run exists. For the removal-history test, insert a valid `loop_runs` row directly in SQL using the seed snapshot fields; `StartLoopRun` is introduced in Task 3.

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd iroll
go test ./db -run 'TestNormalizeLoopJSON|TestLoopSeed' -v
```

Expected: FAIL because loop types and seed operations do not exist.

- [ ] **Step 3: Add loop public types**

Define these types in `loop_types.go`:

```go
type LoopSeed struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Describe   string  `json:"describe"`
	Content    string  `json:"content"`
	Weight     float64 `json:"weight"`
	ArchivedAt *string `json:"archived_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type LoopSeedPatch struct {
	Describe *string
	Content  *string
	Weight   *float64
}

type LoopRun struct {
	ID           int64           `json:"run_id"`
	LoopID       int64           `json:"loop_id"`
	PageID       string          `json:"page_id"`
	ParentRunID  *int64          `json:"parent_run_id"`
	SeedName     string          `json:"seed_name"`
	SeedDescribe string          `json:"seed_describe"`
	SeedContent  string          `json:"seed_content"`
	SeedWeight   float64         `json:"seed_weight"`
	Status       string          `json:"status"`
	Plan         json.RawMessage `json:"plan"`
	Progress     json.RawMessage `json:"progress"`
	Result       json.RawMessage `json:"result"`
	Reflection   json.RawMessage `json:"reflection"`
	AbortReason  *string         `json:"abort_reason"`
	StartedAt    string          `json:"started_at"`
	EndedAt      *string         `json:"ended_at"`
	ReflectedAt  *string         `json:"reflected_at"`
	UpdatedAt    string          `json:"updated_at"`
}
```

Implement:

```go
func NormalizeLoopJSON(input string) (string, error) {
	if json.Valid([]byte(input)) {
		var compact bytes.Buffer
		if err := json.Compact(&compact, []byte(input)); err != nil {
			return "", err
		}
		return compact.String(), nil
	}
	data, err := json.Marshal(input)
	return string(data), err
}
```

- [ ] **Step 4: Implement seed CRUD**

Implement:

```go
func InsertLoopSeed(conn *sql.DB, name, describe, content string, weight float64) (*LoopSeed, error)
func UpdateLoopSeed(conn *sql.DB, name string, patch LoopSeedPatch) (*LoopSeed, error)
func GetLoopSeedByName(conn *sql.DB, name string) (*LoopSeed, error)
func ListLoopSeeds(conn *sql.DB, includeArchived bool) ([]LoopSeed, error)
func ArchiveLoopSeed(conn *sql.DB, name string) (*LoopSeed, error)
func RestoreLoopSeed(conn *sql.DB, name string) (*LoopSeed, error)
func RemoveLoopSeed(conn *sql.DB, name string) error
```

Every public operation first calls `EnsureLoopSchema`. Validate seed fields before SQL. `RemoveLoopSeed` must check `SELECT COUNT(*) FROM loop_runs WHERE loop_id = ?` and return an error instructing the caller to archive when the count is nonzero.

- [ ] **Step 5: Run seed tests**

Run:

```bash
cd iroll
go test ./db -run 'TestNormalizeLoopJSON|TestLoopSeed' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add iroll/db/loop_types.go iroll/db/loop_seed.go iroll/db/loop_seed_test.go
git commit -m "feat: add loop seed management"
```

## Task 3: Implement Main and Child Run Creation

**Files:**
- Create: `iroll/db/loop_run.go`
- Create: `iroll/db/loop_run_test.go`

- [ ] **Step 1: Write failing run creation tests**

Cover independent pages, one active main per page, multiple children, seed snapshots, archived seed rejection, and grandchild rejection:

```go
func TestStartLoopRunAllowsSameSeedAcrossPages(t *testing.T) {
	conn := setupLoopRunTest(t)
	first, err := StartLoopRun(conn, "page-a", "review", nil, `{"step":1}`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := StartLoopRun(conn, "page-b", "review", nil, `null`)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.PageID == second.PageID {
		t.Fatalf("runs are not independent: %#v %#v", first, second)
	}
}

func TestStartLoopRunRejectsSecondMainAndGrandchild(t *testing.T) {
	conn := setupLoopRunTest(t)
	main, _ := StartLoopRun(conn, "page-a", "review", nil, `null`)
	if _, err := StartLoopRun(conn, "page-a", "review", nil, `null`); err == nil {
		t.Fatal("accepted second active main")
	}
	child, err := StartLoopRun(conn, "page-a", "review", &main.ID, `null`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StartLoopRun(conn, "page-a", "review", &child.ID, `null`); err == nil {
		t.Fatal("accepted grandchild")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd iroll
go test ./db -run 'TestStartLoopRun' -v
```

Expected: FAIL because `StartLoopRun` does not exist.

- [ ] **Step 3: Implement transactional run creation**

Implement:

```go
func StartLoopRun(conn *sql.DB, pageID, seedName string, parentRunID *int64, plan string) (*LoopRun, error)
func GetLoopRun(conn *sql.DB, runID int64) (*LoopRun, error)
func GetActiveMainLoopRun(conn *sql.DB, pageID string) (*LoopRun, error)
func ListActiveChildLoopRuns(conn *sql.DB, mainRunID int64) ([]LoopRun, error)
```

`StartLoopRun` must:

1. Call `EnsureLoopSchema`.
2. Begin a transaction.
3. Load a non-archived seed.
4. For a main run, reject an existing active main.
5. For a child, require the parent to be active, on the same page, and itself a main run.
6. Insert a seed snapshot with status `active`, supplied normalized plan, and `null` for other JSON fields.
7. Commit and return the inserted run.

Convert SQLite unique-index conflicts for a second active main into a stable error message.

- [ ] **Step 4: Run run-creation tests**

Run:

```bash
cd iroll
go test ./db -run 'TestStartLoopRun' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add iroll/db/loop_run.go iroll/db/loop_run_test.go
git commit -m "feat: add page-scoped loop runs"
```

## Task 4: Implement Run Updates, Completion, Abort, Reflection, and History

**Files:**
- Modify: `iroll/db/loop_run.go`
- Modify: `iroll/db/loop_run_test.go`

- [ ] **Step 1: Write failing lifecycle tests**

Add tests for:

- Default main-run resolution.
- Explicit child updates.
- Full replacement of supplied plan/progress fields.
- Rejection of updates to ended runs.
- Rejection of ending a main with active children.
- Completion and abortion timestamps/status.
- Reflection only after ending.
- History filters and stable newest-first ordering.

Representative test:

```go
func TestCompleteLoopRunRequiresChildrenToEndFirst(t *testing.T) {
	conn := setupLoopRunTest(t)
	main, _ := StartLoopRun(conn, "page-a", "review", nil, `null`)
	child, _ := StartLoopRun(conn, "page-a", "review", &main.ID, `null`)

	if _, err := CompleteLoopRun(conn, "page-a", nil, `"done"`); err == nil {
		t.Fatal("completed main with active child")
	}
	if _, err := AbortLoopRun(conn, "page-a", &child.ID, "not needed", `null`); err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteLoopRun(conn, "page-a", nil, `{"summary":"done"}`); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run lifecycle tests to verify they fail**

Run:

```bash
cd iroll
go test ./db -run 'TestUpdateLoopRun|TestCompleteLoopRun|TestAbortLoopRun|TestReflectLoopRun|TestListLoopHistory' -v
```

Expected: FAIL because lifecycle functions do not exist.

- [ ] **Step 3: Implement lifecycle functions**

Implement:

```go
func UpdateLoopRun(conn *sql.DB, pageID string, runID *int64, plan, progress *string) (*LoopRun, error)
func CompleteLoopRun(conn *sql.DB, pageID string, runID *int64, result string) (*LoopRun, error)
func AbortLoopRun(conn *sql.DB, pageID string, runID *int64, reason, result string) (*LoopRun, error)
func ReflectLoopRun(conn *sql.DB, runID int64, reflection string) (*LoopRun, error)
func ListLoopHistory(conn *sql.DB, seedName, pageID string, limit int) ([]LoopRun, error)
```

Use a shared transactional resolver:

```go
func resolveRunForMutation(tx *sql.Tx, pageID string, runID *int64) (*LoopRun, error)
```

When `runID == nil`, resolve the page's active main run. When explicit, require that run to belong to the current page. Normalize every JSON-or-text value before storing. `UpdateLoopRun` replaces only supplied fields. Ended runs reject all mutations except reflection.

- [ ] **Step 4: Run lifecycle tests**

Run:

```bash
cd iroll
go test ./db -run 'TestUpdateLoopRun|TestCompleteLoopRun|TestAbortLoopRun|TestReflectLoopRun|TestListLoopHistory' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add iroll/db/loop_run.go iroll/db/loop_run_test.go
git commit -m "feat: add loop run lifecycle"
```

## Task 5: Inject Loop Focus and Available Seeds into Page Context

**Files:**
- Create: `iroll/db/loop_context.go`
- Create: `iroll/db/loop_context_test.go`
- Modify: `iroll/db/db.go`
- Modify: `iroll/db/db_test.go`
- Modify: `iroll/cmd/page.go`
- Modify: `iroll/cmd/context.go`

- [ ] **Step 1: Write failing dynamic context tests**

Define context view types:

```go
type LoopSeedStats struct {
	Active      int             `json:"active"`
	Completed   int             `json:"completed"`
	Aborted     int             `json:"aborted"`
	LastEndedAt *string         `json:"last_ended_at"`
	LastResult  json.RawMessage `json:"last_result"`
}

type AvailableLoopSeed struct {
	LoopSeed
	Stats LoopSeedStats `json:"stats"`
}

type LoopContextView struct {
	Focus struct {
		Main     *LoopRun  `json:"main"`
		Children []LoopRun `json:"children"`
	} `json:"focus"`
	Available []AvailableLoopSeed `json:"available"`
}
```

Test:

```go
func TestResolveContextInjectsPageLoopViewAndReplacesRawLoop(t *testing.T) {
	conn := setupLoopRunTest(t)
	main, _ := StartLoopRun(conn, "page-a", "review", nil, `{"step":1}`)
	_, _ = StartLoopRun(conn, "page-a", "review", &main.ID, `null`)

	got, err := ResolveContext(`{"system_prompt":"hello","loop":"stale"}`, t.TempDir(), conn, "page-a")
	if err != nil {
		t.Fatal(err)
	}
	var context map[string]any
	if err := json.Unmarshal([]byte(got), &context); err != nil {
		t.Fatal(err)
	}
	loop := context["loop"].(map[string]any)
	focus := loop["focus"].(map[string]any)
	if focus["main"] == nil || len(focus["children"].([]any)) != 1 {
		t.Fatalf("loop view = %#v", loop)
	}
}
```

Also test a new page with `main: null`, page independence, archived seeds excluded from `available`, and global aggregate statistics.

- [ ] **Step 2: Run context tests to verify they fail**

Run:

```bash
cd iroll
go test ./db -run 'TestBuildLoopContext|TestResolveContextInjectsPageLoop' -v
```

Expected: FAIL because loop context construction and the new `ResolveContext` signature do not exist.

- [ ] **Step 3: Implement loop context construction**

Implement:

```go
func BuildLoopContext(conn *sql.DB, pageID string) (*LoopContextView, error)
```

Use one query for active focus and one aggregate query for available seeds. Return empty arrays rather than `nil`. Decode stored JSON fields into `json.RawMessage` so they appear as JSON values rather than escaped strings.

- [ ] **Step 4: Inject loop view during context resolution**

Change:

```go
func ResolveContext(rawContext, irollPath string, conn *sql.DB, pageID string) (string, error)
```

After resolving ordinary `@file` and `@sql` values, call `BuildLoopContext`, assign it to reserved key `loop`, and marshal the final object. Update `page new` and `page get-context` call sites to pass the page ID. Update existing traversal tests to pass an empty page ID and a test database with loop schema.

- [ ] **Step 5: Run context tests and command package tests**

Run:

```bash
cd iroll
go test ./db ./cmd
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add iroll/db/loop_context.go iroll/db/loop_context_test.go iroll/db/db.go iroll/db/db_test.go iroll/cmd/page.go iroll/cmd/context.go
git commit -m "feat: inject loop state into page context"
```

## Task 6: Abort Active Runs When Deleting a Page

**Files:**
- Modify: `iroll/db/loop_run.go`
- Modify: `iroll/db/loop_run_test.go`
- Modify: `iroll/db/db.go`
- Modify: `iroll/store/system.go`
- Create: `iroll/store/system_test.go`

- [ ] **Step 1: Write failing page deletion tests**

Test the database operation and the store-level workflow:

```go
func TestDeletePageAbortsActiveRuns(t *testing.T) {
	home, rollName, pageID := setupIndexedPageTest(t)
	_ = home
	conn, err := db.Open(mustDbPath(t, rollName))
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := db.InsertLoopSeed(conn, "review", "Review", "Review memory", 0.8)
	_ = seed
	run, _ := db.StartLoopRun(conn, pageID, "review", nil, `null`)
	conn.Close()

	if err := DeletePage(pageID); err != nil {
		t.Fatal(err)
	}
	conn, _ = db.Open(mustDbPath(t, rollName))
	defer conn.Close()
	got, _ := db.GetLoopRun(conn, run.ID)
	if got.Status != "aborted" || got.AbortReason == nil || *got.AbortReason != "page_deleted" {
		t.Fatalf("run after page delete = %#v", got)
	}
}
```

- [ ] **Step 2: Run deletion tests to verify they fail**

Run:

```bash
cd iroll
go test ./db ./store -run 'TestAbortActiveRunsForPage|TestDeletePageAbortsActiveRuns' -v
```

Expected: FAIL because deletion does not update runs.

- [ ] **Step 3: Add transactional page cleanup**

Implement a transaction helper in `db/loop_run.go`:

```go
func abortActiveLoopRunsForPage(tx *sql.Tx, pageID, reason string) error
```

Update the existing `db.DeletePage` in `db/db.go` to use one transaction:

1. Set every active run for `page_id` to `aborted`.
2. Set `abort_reason = 'page_deleted'`, `ended_at`, and `updated_at`.
3. Delete the page row.
4. Commit.

Modify `store.DeletePage` to open the roll database and call `db.DeletePage` before removing `page_index` and `active_page` records. Wrap the two system-database deletes in their own transaction, so a roll deletion failure leaves the index intact and a successful roll deletion cannot leave a half-cleared system index.

- [ ] **Step 4: Fix store test home isolation**

Existing store tests set only `USERPROFILE`, while `os.UserHomeDir()` uses `HOME` on macOS. Update each store test setup to set both:

```go
t.Setenv("USERPROFILE", home)
t.Setenv("HOME", home)
```

This removes the existing full-suite failure and ensures loop deletion tests do not touch real `~/.iroll`.

- [ ] **Step 5: Run deletion and store tests**

Run:

```bash
cd iroll
go test ./db ./store -run 'TestAbortActiveRunsForPage|TestDeletePageAbortsActiveRuns|TestExtract' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add iroll/db/loop_run.go iroll/db/loop_run_test.go iroll/db/db.go iroll/store/system.go iroll/store/system_test.go iroll/store/store_test.go
git commit -m "feat: abort loop runs when deleting pages"
```

## Task 7: Add Loop Seed CLI Commands

**Files:**
- Create: `iroll/cmd/loop.go`
- Create: `iroll/cmd/loop_seed.go`
- Create: `iroll/cmd/loop_test.go`

- [ ] **Step 1: Write failing seed command tests**

Follow the existing command-test pattern and test helper functions directly:

```go
func TestRunLoopSeedLifecycleUsesActiveIroll(t *testing.T) {
	cwd, _ := setupLoopCommandTest(t)
	added, err := runLoopAdd(cwd, "review", "Review memory", "Inspect memories", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	if added.Name != "review" {
		t.Fatalf("added = %#v", added)
	}
	if _, err := runLoopArchive(cwd, "review"); err != nil {
		t.Fatal(err)
	}
	active, err := runLoopList(cwd, false)
	if err != nil || len(active) != 0 {
		t.Fatalf("active = %#v, %v", active, err)
	}
}
```

Also test edit with no supplied fields, invalid weight, remove-with-history failure, archive, restore, inspect, and archived listing.

- [ ] **Step 2: Run command tests to verify they fail**

Run:

```bash
cd iroll
go test ./cmd -run 'TestRunLoopSeed' -v
```

Expected: FAIL because loop commands do not exist.

- [ ] **Step 3: Add root loop command and active-page helper**

In `cmd/loop.go`, register:

```go
var loopCmd = &cobra.Command{
	Use:   "loop",
	Short: "Manage loop seeds and autonomous runs",
}
```

Add a helper that resolves absolute cwd through `store.GetActive`, opens the active roll database, and returns `name`, `pageID`, and connection. Seed commands use the active roll even though they do not mutate the page.

- [ ] **Step 4: Implement seed command helpers and Cobra commands**

Implement helper functions:

```go
func runLoopList(cwd string, includeArchived bool) ([]db.LoopSeed, error)
func runLoopInspect(cwd, name string) (*db.LoopSeed, error)
func runLoopAdd(cwd, name, describe, content string, weight float64) (*db.LoopSeed, error)
func runLoopEdit(cwd, name string, patch db.LoopSeedPatch) (*db.LoopSeed, error)
func runLoopRemove(cwd, name string) error
func runLoopArchive(cwd, name string) (*db.LoopSeed, error)
func runLoopRestore(cwd, name string) (*db.LoopSeed, error)
```

Register the approved seed syntax:

```text
logos loop list [--archived] [--cwd .]
logos loop inspect <name> [--cwd .]
logos loop add <name> --describe <text> --content <text> [--weight 0.5] [--cwd .]
logos loop edit <name> [--describe <text>] [--content <text>] [--weight <n>] [--cwd .]
logos loop remove <name> [--cwd .]
logos loop archive <name> [--cwd .]
logos loop restore <name> [--cwd .]
```

Keep Cobra handlers thin: validate argument counts, call helpers, output JSON.

- [ ] **Step 5: Run seed CLI tests**

Run:

```bash
cd iroll
go test ./cmd -run 'TestRunLoopSeed' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add iroll/cmd/loop.go iroll/cmd/loop_seed.go iroll/cmd/loop_test.go
git commit -m "feat: add loop seed commands"
```

## Task 8: Add Loop Run CLI Commands

**Files:**
- Create: `iroll/cmd/loop_run.go`
- Modify: `iroll/cmd/loop_test.go`

- [ ] **Step 1: Write failing run command tests**

Cover the full agent workflow:

```go
func TestRunLoopLifecycleUsesCurrentPageMainByDefault(t *testing.T) {
	cwd, _ := setupLoopCommandTest(t)
	_, _ = runLoopAdd(cwd, "review", "Review memory", "Inspect memories", 0.8)
	main, err := runLoopStart(cwd, "review", nil, `{"step":1}`)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := runLoopUpdate(cwd, nil, nil, ptr(`{"done":1}`))
	if err != nil || string(updated.Progress) != `{"done":1}` {
		t.Fatalf("updated = %#v, %v", updated, err)
	}
	completed, err := runLoopComplete(cwd, nil, `{"summary":"done"}`)
	if err != nil || completed.ID != main.ID || completed.Status != "completed" {
		t.Fatalf("completed = %#v, %v", completed, err)
	}
}
```

Also test explicit child targeting, second-main rejection, child-before-main completion requirement, abort reason, reflection, current view, show, and history filtering.

- [ ] **Step 2: Run command tests to verify they fail**

Run:

```bash
cd iroll
go test ./cmd -run 'TestRunLoopLifecycle|TestRunLoopChild|TestRunLoopHistory' -v
```

Expected: FAIL because run commands do not exist.

- [ ] **Step 3: Implement run command helpers**

Implement:

```go
func runLoopStart(cwd, seedName string, parentRunID *int64, plan string) (*db.LoopRun, error)
func runLoopUpdate(cwd string, runID *int64, plan, progress *string) (*db.LoopRun, error)
func runLoopComplete(cwd string, runID *int64, result string) (*db.LoopRun, error)
func runLoopAbort(cwd string, runID *int64, reason, result string) (*db.LoopRun, error)
func runLoopReflect(cwd string, runID int64, content string) (*db.LoopRun, error)
func runLoopCurrent(cwd string) (*db.LoopContextView, error)
func runLoopHistory(cwd, seedName, pageID string, limit int) ([]db.LoopRun, error)
func runLoopShow(cwd string, runID int64) (*db.LoopRun, error)
```

Every mutation resolves the current page and passes its page ID to `db`. `show` and `history` resolve the active iroll but may inspect ended runs from other pages in that iroll.

- [ ] **Step 4: Register approved run commands**

Add the exact approved syntax:

```text
logos loop run <name> [--parent <main-run-id>] [--plan <json-or-text>] [--cwd .]
logos loop update [run-id] [--plan <json-or-text>] [--progress <json-or-text>] [--cwd .]
logos loop complete [run-id] --result <json-or-text> [--cwd .]
logos loop abort [run-id] --reason <text> [--result <json-or-text>] [--cwd .]
logos loop reflect <run-id> --content <json-or-text> [--cwd .]
logos loop current [--cwd .]
logos loop history <name> [--page <page-id>] [--limit <n>] [--cwd .]
logos loop show <run-id> [--cwd .]
```

Use `strconv.ParseInt` for optional/required run IDs and produce explicit invalid-ID errors.

- [ ] **Step 5: Run loop command tests**

Run:

```bash
cd iroll
go test ./cmd -run 'TestRunLoop' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add iroll/cmd/loop_run.go iroll/cmd/loop_test.go
git commit -m "feat: add loop run commands"
```

## Task 9: Update Agent Documentation and Add End-to-End Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/rebot-roll.md`
- Modify: `skills/logos-1/skill.md`
- Create: `iroll/cmd/loop_integration_test.go`

- [ ] **Step 1: Write failing end-to-end test**

Build a temporary roll from the base example, create two pages, and verify the same seed can run independently:

```go
func TestLoopEndToEndAcrossIndependentPages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	layerfile, err := builder.ParseLayerfile(filepath.Join("..", "..", "examples", "base-agent", "Layerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.Build(layerfile, "loop-e2e"); err != nil {
		t.Fatal(err)
	}

	dbPath, err := store.DbPath("loop-e2e")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	pageA, err := db.InsertPage(conn, filepath.Join(t.TempDir(), "page-a"))
	if err != nil {
		t.Fatal(err)
	}
	pageB, err := db.InsertPage(conn, filepath.Join(t.TempDir(), "page-b"))
	if err != nil {
		t.Fatal(err)
	}

	runA, err := db.StartLoopRun(conn, pageA.PageID, "self-cognition", nil, `{"steps":["read context"]}`)
	if err != nil {
		t.Fatal(err)
	}
	runB, err := db.StartLoopRun(conn, pageB.PageID, "self-cognition", nil, `{"steps":["review dna"]}`)
	if err != nil {
		t.Fatal(err)
	}
	progress := `{"read_context":true}`
	if _, err := db.UpdateLoopRun(conn, pageA.PageID, nil, nil, &progress); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CompleteLoopRun(conn, pageA.PageID, nil, `{"summary":"understood"}`); err != nil {
		t.Fatal(err)
	}

	gotA, err := db.BuildLoopContext(conn, pageA.PageID)
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := db.BuildLoopContext(conn, pageB.PageID)
	if err != nil {
		t.Fatal(err)
	}
	if gotA.Focus.Main != nil {
		t.Fatalf("page A still focused after completion: %#v", gotA.Focus.Main)
	}
	if gotB.Focus.Main == nil || gotB.Focus.Main.ID != runB.ID || runA.ID == runB.ID {
		t.Fatalf("page B focus = %#v; runs A=%d B=%d", gotB.Focus.Main, runA.ID, runB.ID)
	}

	rows, err := conn.Query(`PRAGMA table_info(loop)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	legacy := map[string]bool{
		"type": true, "status": true, "executed_count": true, "result": true,
	}
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if legacy[name] {
			t.Fatalf("built loop table still contains legacy column %q", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
```

Import `logos/builder`, `logos/db`, `logos/store`, `path/filepath`, and `testing`. Use existing builder and database helpers rather than spawning the CLI binary.

- [ ] **Step 2: Run end-to-end test to verify it fails**

Run:

```bash
cd iroll
go test ./cmd -run TestLoopEndToEndAcrossIndependentPages -v
```

Expected: FAIL until all loop integration wiring is complete.

- [ ] **Step 3: Update current documentation**

Document:

- Loop seeds versus page-scoped loop runs.
- Logos manages context and records but never executes work.
- Dynamic `loop.focus` and `loop.available` context.
- Independent pages and concurrent use of the same seed.
- Main/child one-level hierarchy.
- `active → completed | aborted` lifecycle.
- Seed and run command reference.
- Reflection and history behavior.

Remove old statements describing `once`, `periodic`, global loop status, and `executed_count`.

- [ ] **Step 4: Run end-to-end and full test suites**

Run:

```bash
cd iroll
go test ./cmd -run TestLoopEndToEndAcrossIndependentPages -v
go test ./...
```

Expected: PASS for all packages, including `store` after its HOME isolation fix.

- [ ] **Step 5: Build and manually verify the CLI workflow**

Run:

```bash
cd iroll
go build -o /tmp/logos-loop-verify .
rm -rf /tmp/logos-loop-home /tmp/logos-loop-work
mkdir -p /tmp/logos-loop-work
HOME=/tmp/logos-loop-home /tmp/logos-loop-verify roll build -f ../examples/base-agent/Layerfile -t loop-verify
HOME=/tmp/logos-loop-home /tmp/logos-loop-verify page new loop-verify --cwd /tmp/logos-loop-work
HOME=/tmp/logos-loop-home /tmp/logos-loop-verify loop list --cwd /tmp/logos-loop-work
HOME=/tmp/logos-loop-home /tmp/logos-loop-verify loop run self-cognition --plan '{"steps":["read context","review dna"]}' --cwd /tmp/logos-loop-work
HOME=/tmp/logos-loop-home /tmp/logos-loop-verify page get-context --cwd /tmp/logos-loop-work
HOME=/tmp/logos-loop-home /tmp/logos-loop-verify loop update --progress '{"completed":["read context"]}' --cwd /tmp/logos-loop-work
HOME=/tmp/logos-loop-home /tmp/logos-loop-verify loop complete --result '{"summary":"identity understood"}' --cwd /tmp/logos-loop-work
HOME=/tmp/logos-loop-home /tmp/logos-loop-verify loop history self-cognition --cwd /tmp/logos-loop-work
```

Expected:

- `loop list` returns the two base seeds.
- `loop run` returns an active main run.
- resolved page context contains the run in `loop.focus.main`.
- `loop complete` removes it from focus.
- history contains the completed immutable life record.

- [ ] **Step 6: Commit**

```bash
git add README.md docs/rebot-roll.md skills/logos-1/skill.md iroll/cmd/loop_integration_test.go
git commit -m "docs: document loop context workflow"
```

## Task 10: Final Verification

**Files:**
- No new files

- [ ] **Step 1: Check formatting and stale terminology**

Run:

```bash
gofmt -w iroll/db/loop_*.go iroll/cmd/loop*.go
rg -n 'once|periodic|executed_count|loop.*global status|heartbeat' README.md docs/rebot-roll.md skills/logos-1/skill.md examples/base-agent iroll --glob '*.md' --glob '*.sql' --glob '*.go'
git diff --check
```

Expected: no current-use documentation or implementation references the removed loop model; historical superseded specs may still contain old terms.

- [ ] **Step 2: Run all tests**

Run:

```bash
cd iroll
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Review the final diff**

Run:

```bash
git status --short
git diff --stat
git log --oneline --max-count=12
```

Expected: only intentional loop implementation, tests, schema, and documentation changes remain; task commits are visible.

- [ ] **Step 4: Commit any final verification-only corrections**

If formatting or terminology checks required corrections:

```bash
git add iroll examples/base-agent README.md docs/rebot-roll.md skills/logos-1/skill.md
git commit -m "chore: finalize loop context implementation"
```

Otherwise, do not create an empty commit.
