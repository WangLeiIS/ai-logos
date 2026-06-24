# Design: Loop Seed Type Field (auto/normal)

**Date:** 2026-06-24
**Status:** Approved

## Problem

All loop seeds are currently equal — every seed requires the user or agent to manually execute `loop run <name>` to create a loop_runs record. There is no way to mark certain seeds as "auto-start": seeds that should automatically create a loop_runs entry when a new page is created.

Use case: system-predefined loop seeds (like `self-cognition`, `daily-check`) should start running immediately when a page is born, without waiting for the agent to discover and manually invoke them.

## Solution

Add a `type` column to the `loop` table with two values: `auto` and `normal`.

- `normal` (default): current behavior — must be manually started via `loop run`
- `auto`: on `page new`, all non-archived `auto` seeds are automatically written to `loop_runs` as active runs for the new page

Auto seeds behave identically to normal seeds in all other respects — they can be manually run, edited, archived, and appear in seed lists.

## Schema Change

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

## Go Type Changes

### LoopSeed

```go
type LoopSeed struct {
    ID         int64   `json:"id"`
    Type       string  `json:"type"`      // "auto" | "normal"
    Name       string  `json:"name"`
    Describe   string  `json:"describe"`
    Content    string  `json:"content"`
    Weight     float64 `json:"weight"`
    ArchivedAt *string `json:"archived_at"`
    CreatedAt  string  `json:"created_at"`
    UpdatedAt  string  `json:"updated_at"`
}
```

### LoopSeedPatch

```go
type LoopSeedPatch struct {
    Type     *string
    Describe *string
    Content  *string
    Weight   *float64
}
```

## DB Layer Changes

### Modified functions

| Function | Change |
|---|---|
| `InsertLoopSeed` | Add `loopType string` parameter, insert into `type` column |
| `UpdateLoopSeed` | `LoopSeedPatch` already has `Type` field — handle type updates |
| `scanLoopSeed` | Scan the `type` column |
| `ListAvailableLoopSeeds` | SELECT includes `loop.type`; WHERE only filters `archived_at IS NULL` (auto seeds still appear with their type label) |

### New function

```go
// AutoStartLoopSeeds starts all non-archived auto-type loop seeds for a new page.
// Each seed becomes one active loop_runs row with plan "null".
func AutoStartLoopSeeds(conn *sql.DB, pageID string) ([]LoopRun, error)
```

This queries `SELECT id, name FROM loop WHERE type = 'auto' AND archived_at IS NULL`, then calls `StartLoopRun` for each seed with `plan = "null"` and `parentRunID = nil`. Returns all created runs.

## Page Creation Flow

`logos page new` calls `InsertPage`, then immediately calls `AutoStartLoopSeeds`:

```
page new → InsertPage → AutoStartLoopSeeds → IndexPage → resolve context
```

If `AutoStartLoopSeeds` fails, the page creation fails (transactional — page is inserted but auto-start failure is reported as an error).

## Context Resolution

- `loop_focus`: unchanged — lists active runs for the page (includes auto-started runs). Each `LoopRun` already carries `seed_name`, so the agent can distinguish them.
- `loop_available`: `ListAvailableLoopSeeds` returns **all** non-archived seeds (both auto and normal) with their `type` label. `ResolveContext` then filters to `type = 'normal'` only when setting `loop_available`, because auto seeds were already auto-started and appear in `loop_focus`.

## Command Changes

### `loop add`

```
logos loop add <name> --type auto   # create auto-start seed
logos loop add <name>               # defaults to --type normal
```

New `--type` flag with values `auto` and `normal` (default: `normal`).

### `loop edit`

```
logos loop edit <name> --type normal
```

Can change a seed's type between `auto` and `normal`.

### `loop list` / `loop list --stats`

Output includes `type` field for each seed. Both auto and normal seeds are listed.

### CLI validator

Add a `validateLoopSeedType` function that accepts only `"auto"` and `"normal"`.

## Files to Change

| File | Change |
|---|---|
| `examples/base-agent/init_schema.sql` | Add `type` column to `loop` table |
| `iroll/db/loop_types.go` | Add `Type` to `LoopSeed`, `LoopSeedPatch` |
| `iroll/db/loop_seed.go` | Update `InsertLoopSeed`, `UpdateLoopSeed`, `scanLoopSeed`, `ListAvailableLoopSeeds`; add validation |
| `iroll/db/loop_run.go` | Add `AutoStartLoopSeeds` |
| `iroll/db/loop_context.go` | `availableLoopSeedsQuery` updated with type filter |
| `iroll/cmd/page.go` | `pageNewCmd` calls `AutoStartLoopSeeds` after page creation |
| `iroll/cmd/loop_seed.go` | `loop add` gets `--type` flag; `loop edit` can update type; `loop list` output includes type |
| `iroll/db/db.go` | `ResolveContext` uses updated `ListAvailableLoopSeeds` |
| Tests | Update all loop-related tests |

## Not Changed

- Loop run lifecycle (`loop_runs` schema unchanged)
- Run status management (complete/abort/reflect)
- `loop ps`, `loop show`, `loop history` commands
- `loop archive` / `loop restore` / `loop remove`
- Page lifecycle beyond creation
