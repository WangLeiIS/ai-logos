# Loop Type Field (auto/normal) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `type` column to `loop` table distinguishing auto-start seeds (auto-created in loop_runs on page creation) from normal seeds (manual start only).

**Architecture:** New `type` column with CHECK constraint. `AutoStartLoopSeeds` function called during `page new`. Type flows through all DB scan/insert/update functions. `loop_available` in context filters to normal only; `loop list` shows all seeds with type label.

**Tech Stack:** Go, SQLite (go-sqlite3), Cobra CLI

---

### Task 1: Schema and Type Definitions

**Files:**
- Modify: `examples/base-agent/init_schema.sql:30-39`
- Modify: `iroll/db/loop_types.go`
- Modify: `examples/base-agent/init_data.sql:74-92`

- [ ] **Step 1: Add `type` column to init_schema.sql**

Replace the `loop` table definition:

```sql
CREATE TABLE loop (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL DEFAULT 'normal' CHECK (type IN ('auto', 'normal')),
    describe TEXT NOT NULL,
    content TEXT NOT NULL,
    weight REAL NOT NULL DEFAULT 0.5 CHECK (weight >= 0 AND weight <= 1),
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

- [ ] **Step 2: Add `Type` to `LoopSeed` struct in loop_types.go**

In `LoopSeed` (line 9), add the `Type` field between `Name` and `Describe`:

```go
type LoopSeed struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`       // "auto" | "normal"
	Describe   string  `json:"describe"`
	Content    string  `json:"content"`
	Weight     float64 `json:"weight"`
	ArchivedAt *string `json:"archived_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}
```

- [ ] **Step 3: Add `Type` to `LoopSeedPatch` struct in loop_types.go**

In `LoopSeedPatch` (line 19), add `Type` field:

```go
type LoopSeedPatch struct {
	Type     *string
	Describe *string
	Content  *string
	Weight   *float64
}
```

- [ ] **Step 4: Update init_data.sql loop insertions**

Add `type` to each INSERT:

```sql
INSERT INTO loop (name, type, describe, content, weight, archived_at, created_at, updated_at) VALUES
    (
        'self-cognition',
        'auto',
        '自我认知',
        '阅读所有 context 和 dna，了解自己的身份',
        0.9,
        NULL,
        datetime('now'),
        datetime('now')
    ),
    (
        'daily-check',
        'auto',
        '日常检查',
        '检查 dna 和 memory，决定当前需要关注的事项',
        0.8,
        NULL,
        datetime('now'),
        datetime('now')
    );
```

- [ ] **Step 5: Build check — db types only**

```bash
cd iroll && go build ./db/...
```

Expected: fails — `scanLoopSeed` doesn't scan `type`, `InsertLoopSeed` doesn't write it.

- [ ] **Step 6: Commit**

```bash
git add examples/base-agent/init_schema.sql iroll/db/loop_types.go examples/base-agent/init_data.sql
git commit -m "feat: add type column to loop table schema and Go types

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: DB Seed Layer — Insert, Update, Scan, Validate

**Files:**
- Modify: `iroll/db/loop_seed.go`

- [ ] **Step 1: Add type validation function**

After `validateLoopSeedWeight` (line 251), add:

```go
func validateLoopSeedType(loopType string) (string, error) {
	loopType = strings.TrimSpace(loopType)
	switch loopType {
	case "auto", "normal":
		return loopType, nil
	default:
		return "", fmt.Errorf("loop seed type must be 'auto' or 'normal', got %q: %w", loopType, ErrInvalidLoopSeed)
	}
}
```

- [ ] **Step 2: Update `InsertLoopSeed` signature and body**

Change function signature (line 19) from `(name, describe, content string, weight float64)` to include `loopType string`:

```go
func InsertLoopSeed(conn *sql.DB, name, loopType, describe, content string, weight float64) (*LoopSeed, error) {
	name, loopType, describe, content, err := validateLoopSeed(name, loopType, describe, content, weight)
	// ...rest unchanged except the INSERT
```

Update `validateLoopSeed` (line 220) to accept `loopType string` and validate it:

```go
func validateLoopSeed(name, loopType, describe, content string, weight float64) (string, string, string, string, error) {
	name, err := validateLoopSeedName(name)
	if err != nil {
		return "", "", "", "", err
	}
	loopType, err = validateLoopSeedType(loopType)
	if err != nil {
		return "", "", "", "", err
	}
	describe, err = validateLoopSeedText("describe", describe)
	if err != nil {
		return "", "", "", "", err
	}
	content, err = validateLoopSeedText("content", content)
	if err != nil {
		return "", "", "", "", err
	}
	if err := validateLoopSeedWeight(weight); err != nil {
		return "", "", "", "", err
	}
	return name, loopType, describe, content, nil
}
```

Update the INSERT in `InsertLoopSeed` (line 27):

```go
now := nowISO()
result, err := conn.Exec(`
	INSERT INTO loop (name, type, describe, content, weight, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
`, name, loopType, describe, content, weight, now, now)
```

- [ ] **Step 3: Update `scanLoopSeed` to scan type column**

Update `scanLoopSeed` (line 275) to scan an additional column. The query must now include `type` in all SELECT lists. The scan call becomes:

```go
func scanLoopSeed(scanner loopSeedScanner) (*LoopSeed, error) {
	var seed LoopSeed
	var archivedAt sql.NullString
	if err := scanner.Scan(
		&seed.ID, &seed.Name, &seed.Type, &seed.Describe, &seed.Content, &seed.Weight,
		&archivedAt, &seed.CreatedAt, &seed.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if archivedAt.Valid {
		seed.ArchivedAt = &archivedAt.String
	}
	return &seed, nil
}
```

- [ ] **Step 4: Update `loopSeedReturning` constant**

Update the RETURNING clause (line 271) to include `type`:

```go
const loopSeedReturning = `
	RETURNING id, name, type, describe, content, weight, archived_at, created_at, updated_at
`
```

- [ ] **Step 5: Update all SELECT queries in loop_seed.go that use scanLoopSeed**

In `GetLoopSeedByName` (line 106), change SELECT to include `type`:

```go
seed, err := scanLoopSeed(conn.QueryRow(`
	SELECT id, name, type, describe, content, weight, archived_at, created_at, updated_at
	FROM loop
	WHERE name = ?
`, name))
```

In `ListLoopSeeds` (line 121), change SELECT to include `type`:

```go
query := `
	SELECT id, name, type, describe, content, weight, archived_at, created_at, updated_at
	FROM loop
`
```

- [ ] **Step 6: Update `UpdateLoopSeed` to handle type**

In `UpdateLoopSeed` (line 56), update the empty-patch guard to include `Type`:

```go
if patch.Type == nil && patch.Describe == nil && patch.Content == nil && patch.Weight == nil {
	return nil, fmt.Errorf("loop seed update: no fields supplied: %w", ErrInvalidLoopSeed)
}
```

Then in the field-building section (after the existing `Describe`/`Content`/`Weight` blocks), add:

```go
if patch.Type != nil {
	loopType, err := validateLoopSeedType(*patch.Type)
	if err != nil {
		return nil, err
	}
	fields = append(fields, "type = ?")
	args = append(args, loopType)
}
```

- [ ] **Step 7: Update `getActiveLoopSeedForRun` SELECT**

In `getActiveLoopSeedForRun` (line 470) in `loop_seed.go` (it's actually in `loop_run.go`), add `type` to the SELECT:

```go
func getActiveLoopSeedForRun(tx *sql.Tx, name string) (*LoopSeed, error) {
	seed, err := scanLoopSeed(tx.QueryRow(`
		SELECT id, name, type, describe, content, weight, archived_at, created_at, updated_at
		FROM loop
		WHERE name = ? AND archived_at IS NULL
	`, name))
```

Note: This function is in `iroll/db/loop_run.go`, not loop_seed.go.

- [ ] **Step 8: Build check**

```bash
cd iroll && go build ./db/...
```

Expected: fails — all callers of `InsertLoopSeed` now pass the wrong number of args.

- [ ] **Step 9: Commit**

```bash
git add iroll/db/loop_seed.go iroll/db/loop_run.go
git commit -m "feat: add type support to loop seed CRUD functions

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 3: AutoStartLoopSeeds + Fix All Callers

**Files:**
- Create/modify: `iroll/db/loop_run.go` (add `AutoStartLoopSeeds`)
- Modify: `iroll/db/loop_run_test.go` (fix `InsertLoopSeed` calls)
- Modify: `iroll/db/loop_seed_test.go` (fix `InsertLoopSeed` calls)
- Modify: `iroll/db/loop_context_test.go` (fix `InsertLoopSeed` calls)
- Modify: `iroll/db/loop_context.go` (fix `ListAvailableLoopSeeds` to scan type)
- Modify: `iroll/cmd/loop_integration_test.go` (no change needed, uses `builder.Build`)

- [ ] **Step 1: Add `AutoStartLoopSeeds` to loop_run.go**

Add after `StartLoopRun` (line 44):

```go
// AutoStartLoopSeeds starts all non-archived auto-type loop seeds for a new page.
// Returns the created runs. If no auto seeds exist, returns an empty slice.
func AutoStartLoopSeeds(conn *sql.DB, pageID string) ([]LoopRun, error) {
	pageID, err := validateLoopRunText("page ID", pageID)
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(`
		SELECT name FROM loop
		WHERE type = 'auto' AND archived_at IS NULL
		ORDER BY weight DESC, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list auto seeds for page %q: %w", pageID, err)
	}
	defer rows.Close()

	var seedNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan auto seed for page %q: %w", pageID, err)
		}
		seedNames = append(seedNames, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list auto seeds for page %q: %w", pageID, err)
	}

	runs := make([]LoopRun, 0, len(seedNames))
	for _, name := range seedNames {
		run, err := StartLoopRun(conn, pageID, name, nil, "null")
		if err != nil {
			return nil, fmt.Errorf("auto-start seed %q for page %q: %w", name, pageID, err)
		}
		runs = append(runs, *run)
	}
	return runs, nil
}
```

- [ ] **Step 2: Fix all `InsertLoopSeed` callers — pass "normal" as type**

In `iroll/db/loop_seed_test.go` — every `InsertLoopSeed` call. Replace all calls from:

```go
InsertLoopSeed(conn, name, describe, content, weight)
```

To:

```go
InsertLoopSeed(conn, name, "normal", describe, content, weight)
```

Affected lines: ~37, ~99, ~125, ~159, ~181, ~261

In `iroll/db/loop_run_test.go` — `InsertLoopSeed` calls at lines ~704, ~722:

```go
InsertLoopSeed(conn, "review", "normal", "Review memory", "Inspect memories", 0.8)
```

In `iroll/db/loop_run_test.go` — `InsertLoopSeed` calls inside `TestStartLoopRunAllowsMultipleChildrenAndListsThemInStableOrder`, `TestListLoopHistoryFiltersOrdersByDescendingIDAndLimits`, etc. Search for all `InsertLoopSeed` calls.

In `iroll/db/loop_context_test.go` — `InsertLoopSeed` calls at lines ~13, ~15, ~17, ~101, ~103, ~105:

```go
InsertLoopSeed(conn, "heavy", "normal", "Heavy work", "Do heavy work", 0.9)
InsertLoopSeed(conn, "alpha", "normal", "Alpha work", "Do alpha work", 0.8)
InsertLoopSeed(conn, "archived", "normal", "Archived work", "Do archived work", 1)
```

- [ ] **Step 3: Update `ListAvailableLoopSeeds` to scan type**

In `iroll/db/loop_context.go`, update `availableLoopSeedsQuery` (line 105) to include `loop.type`:

```go
const availableLoopSeedsQuery = `
	WITH stats AS (
		SELECT
			loop_id,
			SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) AS active,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completed,
			SUM(CASE WHEN status = 'aborted' THEN 1 ELSE 0 END) AS aborted
		FROM loop_runs
		GROUP BY loop_id
	)
	SELECT
		loop.id, loop.name, loop.type, loop.describe, loop.content, loop.weight,
		loop.archived_at, loop.created_at, loop.updated_at,
		COALESCE(stats.active, 0),
		COALESCE(stats.completed, 0),
		COALESCE(stats.aborted, 0),
		latest_ended.ended_at,
		COALESCE(latest_ended.result, 'null')
	FROM loop
	LEFT JOIN stats ON stats.loop_id = loop.id
	LEFT JOIN loop_runs latest_ended ON latest_ended.id = (
		SELECT candidate.id
		FROM loop_runs candidate
		WHERE candidate.loop_id = loop.id
			AND candidate.status IN ('completed', 'aborted')
			AND candidate.ended_at IS NOT NULL
		ORDER BY candidate.ended_at DESC, candidate.id DESC
		LIMIT 1
	)
	WHERE loop.archived_at IS NULL
	ORDER BY loop.weight DESC, loop.name ASC
`
```

Update the scan in `ListAvailableLoopSeeds` (line 79) to scan `type`:

```go
if err := rows.Scan(
	&seed.ID, &seed.Name, &seed.Type, &seed.Describe, &seed.Content, &seed.Weight,
	&archivedAt, &seed.CreatedAt, &seed.UpdatedAt,
	&seed.Stats.Active, &seed.Stats.Completed, &seed.Stats.Aborted,
	&lastEndedAt, &lastResult,
); err != nil {
```

- [ ] **Step 4: Build check**

```bash
cd iroll && go build ./db/...
```

Expected: db package compiles clean.

- [ ] **Step 5: Run db tests**

```bash
cd iroll && go test ./db/...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add iroll/db/loop_run.go iroll/db/loop_seed_test.go iroll/db/loop_run_test.go iroll/db/loop_context_test.go iroll/db/loop_context.go
git commit -m "feat: add AutoStartLoopSeeds and fix all InsertLoopSeed callers

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 4: Context Resolution — Filter loop_available to Normal Only

**Files:**
- Modify: `iroll/db/db.go:235-242`

- [ ] **Step 1: Filter loop_available to normal seeds only**

In `ResolveContext`, replace lines 235-242:

```go
available, err := ListAvailableLoopSeeds(db)
if err != nil {
	return "", err
}
if available == nil {
	available = []AvailableLoopSeed{}
}
resolved["loop_available"] = available
```

With:

```go
allSeeds, err := ListAvailableLoopSeeds(db)
if err != nil {
	return "", err
}
available := make([]AvailableLoopSeed, 0)
for _, s := range allSeeds {
	if s.Type == "normal" {
		available = append(available, s)
	}
}
resolved["loop_available"] = available
```

- [ ] **Step 2: Build check**

```bash
cd iroll && go build ./db/...
```

Expected: compiles.

- [ ] **Step 3: Run db tests**

```bash
cd iroll && go test ./db/...
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add iroll/db/db.go
git commit -m "feat: filter loop_available context to normal seeds only

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 5: CMD Layer — page new + loop add/edit

**Files:**
- Modify: `iroll/cmd/page.go:103-135`
- Modify: `iroll/cmd/loop_seed.go`

- [ ] **Step 1: Update `pageNewCmd` to call `AutoStartLoopSeeds`**

In `pageNewCmd` Run function (line 107), after `InsertPage` and before `IndexPage`:

```go
p, err := db.InsertPage(conn, cwd)
if err != nil {
	outputError(err.Error())
}

// Auto-start system loop seeds
if _, err := db.AutoStartLoopSeeds(conn, p.PageID); err != nil {
	outputError("auto-start loop seeds: " + err.Error())
}

if err := store.IndexPage(name, version, p.PageID, cwd); err != nil {
	outputError(err.Error())
}
```

- [ ] **Step 2: Update `newLoopAddCmd` to accept `--type` flag**

Add `loopType` variable and flag to `newLoopAddCmd` (line 46). Change signature from 5 params to 6:

```go
func newLoopAddCmd(run func(string, string, string, string, string, float64) error) *cobra.Command {
	var cwd string
	var seedType string
	var describe string
	var content string
	var weight float64
	command := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a loop seed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, args[0], seedType, describe, content, weight)
		},
	}
	command.Flags().StringVar(&seedType, "type", "normal", "Seed type: auto or normal")
	command.Flags().StringVar(&describe, "describe", "", "Loop seed description")
	command.Flags().StringVar(&content, "content", "", "Loop seed content")
	command.Flags().Float64Var(&weight, "weight", 0.5, "Loop seed weight")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	_ = command.MarkFlagRequired("describe")
	_ = command.MarkFlagRequired("content")
	isolateLoopSeedCommand(command)
	return command
}
```

- [ ] **Step 3: Update `outputLoopAdd` and `runLoopAdd`**

Update outputLoopAdd (line 175):

```go
func outputLoopAdd(cwd, name, seedType, describe, content string, weight float64) error {
	seed, err := runLoopAdd(cwd, name, seedType, describe, content, weight)
	if err != nil {
		outputError(err.Error())
	}
	outputJSON(seed)
	return nil
}
```

Update runLoopAdd (line 237):

```go
func runLoopAdd(cwd, name, seedType, describe, content string, weight float64) (*db.LoopSeed, error) {
	_, _, conn, err := openActiveLoop(cwd)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return db.InsertLoopSeed(conn, name, seedType, describe, content, weight)
}
```

- [ ] **Step 4: Update `newLoopEditCmd` to accept `--type` flag**

Add `seedType` variable and flag to `newLoopEditCmd` (line 70):

```go
func newLoopEditCmd(run func(string, string, db.LoopSeedPatch) error) *cobra.Command {
	var cwd string
	var seedType string
	var describe string
	var content string
	var weight float64
	command := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a loop seed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetLoopSeedFlags(cmd)
			return run(cwd, args[0], loopSeedPatchFromFlags(cmd, seedType, describe, content, weight))
		},
	}
	command.Flags().StringVar(&seedType, "type", "", "Seed type: auto or normal")
	command.Flags().StringVar(&describe, "describe", "", "Loop seed description")
	command.Flags().StringVar(&content, "content", "", "Loop seed content")
	command.Flags().Float64Var(&weight, "weight", 0.5, "Loop seed weight")
	command.Flags().StringVar(&cwd, "cwd", ".", "Working directory")
	isolateLoopSeedCommand(command)
	return command
}
```

- [ ] **Step 5: Update `loopSeedPatchFromFlags`**

Update `loopSeedPatchFromFlags` (line 282) to accept and handle `seedType`:

```go
func loopSeedPatchFromFlags(cmd *cobra.Command, seedType, describe, content string, weight float64) db.LoopSeedPatch {
	patch := db.LoopSeedPatch{}
	if cmd.Flags().Changed("type") {
		patch.Type = &seedType
	}
	if cmd.Flags().Changed("describe") {
		patch.Describe = &describe
	}
	if cmd.Flags().Changed("content") {
		patch.Content = &content
	}
	if cmd.Flags().Changed("weight") {
		patch.Weight = &weight
	}
	return patch
}
```

- [ ] **Step 6: Build check**

```bash
cd iroll && go build ./...
```

Expected: compiles clean, including cmd package.

- [ ] **Step 7: Commit**

```bash
git add iroll/cmd/page.go iroll/cmd/loop_seed.go
git commit -m "feat: auto-start loop seeds on page new, add --type to loop add/edit

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 6: Update Tests

**Files:**
- Modify: `iroll/db/loop_context_test.go` (type field in assertions)
- Modify: `iroll/db/loop_seed_test.go` (LoopSeedPatch empty check, ADD type test)
- Verify: `iroll/db/db_test.go` (should still pass)
- Verify: `iroll/cmd/loop_integration_test.go` (uses builder.Build, should still pass)
- Verify: `iroll/cmd/loop_test.go` (if exists, check)
- Verify: `iroll/e2e/scenario_integration_test.go`

- [ ] **Step 1: Update loop_seed_test.go — add type validation tests**

Add new test function `TestLoopSeedRejectsInvalidType`:

```go
func TestLoopSeedRejectsInvalidType(t *testing.T) {
	conn := openLoopTestDB(t)
	_, err := InsertLoopSeed(conn, "test", "invalid", "desc", "content", 0.5)
	if err == nil || !errors.Is(err, ErrInvalidLoopSeed) || !strings.Contains(err.Error(), "type must be") {
		t.Fatalf("invalid type error = %v", err)
	}
}
```

Add new test function `TestLoopSeedDefaultTypeIsNormal`:

```go
func TestLoopSeedDefaultTypeIsNormal(t *testing.T) {
	conn := openLoopTestDB(t)
	seed, err := InsertLoopSeed(conn, "review", "normal", "Review", "Content", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if seed.Type != "normal" {
		t.Fatalf("seed type = %q, want 'normal'", seed.Type)
	}
}
```

- [ ] **Step 2: Update loop_seed_test.go — add type update test**

Add type-update test at the end of `TestLoopSeedUpdateRejectsEmptyAndInvalidPatch`:

```go
autoType := "auto"
seed, err := UpdateLoopSeed(conn, "review", LoopSeedPatch{Type: &autoType})
if err != nil {
	t.Fatal(err)
}
if seed.Type != "auto" {
	t.Fatalf("seed type = %q, want 'auto'", seed.Type)
}
// Change it back
normalType := "normal"
_, err = UpdateLoopSeed(conn, "review", LoopSeedPatch{Type: &normalType})
if err != nil {
	t.Fatal(err)
}
```

- [ ] **Step 3: Update loop_context_test.go — assert type in ListAvailableLoopSeeds**

In `TestListAvailableLoopSeedsIsGlobal` (line 99), after the name order assertion at line 161, add type field checks:

```go
for _, s := range seeds {
	if s.Type != "normal" {
		t.Fatalf("seed %q type = %q, want 'normal'", s.Name, s.Type)
	}
}
```

Add a new test `TestAutoStartLoopSeeds`:

```go
func TestAutoStartLoopSeeds(t *testing.T) {
	conn := setupLoopRunTest(t)
	insertLoopTestPage(t, conn, "page-auto")
	if _, err := InsertLoopSeed(conn, "auto-one", "auto", "Auto one", "Do auto one", 0.9); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertLoopSeed(conn, "auto-two", "auto", "Auto two", "Do auto two", 0.8); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertLoopSeed(conn, "normal-seed", "normal", "Normal", "Normal content", 0.5); err != nil {
		t.Fatal(err)
	}

	runs, err := AutoStartLoopSeeds(conn, "page-auto")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("auto runs = %d, want 2", len(runs))
	}
	if runs[0].SeedName != "auto-one" || runs[1].SeedName != "auto-two" {
		t.Fatalf("auto run order = %#v", runs)
	}
	for _, r := range runs {
		if r.Status != "active" || string(r.Plan) != "null" {
			t.Fatalf("auto run %d = %#v", r.ID, r)
		}
	}

	// Verify normal seed was NOT auto-started
	active, err := ListActiveRuns(conn, "page-auto")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 {
		t.Fatalf("active runs = %d, want 2", len(active))
	}
}
```

- [ ] **Step 4: Update TestResolveContextInjectsPageLoopViewAndReplacesRawLoop**

In `iroll/db/loop_context_test.go:338`, the test should now verify that `loop_available` only contains normal-type seeds. Since the test seeds are all created as `"normal"`, they should still appear — but add an auto seed and verify it's filtered out:

After line 346 (after `StartLoopRun` for child), add:

```go
// Insert an auto seed — should NOT appear in loop_available
if _, err := InsertLoopSeed(conn, "auto-hidden", "auto", "Auto hidden", "Hidden content", 0.9); err != nil {
	t.Fatal(err)
}
```

Then after the `loop_available` check (line 371), verify the auto seed is not present:

```go
for _, s := range context["loop_available"].([]any) {
	seed := s.(map[string]any)
	if seed["name"].(string) == "auto-hidden" {
		t.Fatal("auto seed appeared in loop_available")
	}
}
```

- [ ] **Step 5: Run all tests**

```bash
cd iroll && go test ./...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test: add type validation, AutoStartLoopSeeds, and context filter tests

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 7: Final Build and Verification

**Files:**
- No changes, verification only.

- [ ] **Step 1: Full build**

```bash
cd iroll && go build ./...
```

Expected: zero errors.

- [ ] **Step 2: Run all tests — fresh run**

```bash
cd iroll && go test -count=1 ./...
```

Expected: all packages pass.

- [ ] **Step 3: Build binary**

```bash
cd iroll && go build -ldflags "-X logos/cmd.Version=0.1.0" -o ../logos .
```

Expected: binary built.

- [ ] **Step 4: Smoke test**

```bash
cd iroll && ../logos version
```

Expected: prints version info.

- [ ] **Step 5: Commit if any final cleanup needed**

```bash
git status
```
