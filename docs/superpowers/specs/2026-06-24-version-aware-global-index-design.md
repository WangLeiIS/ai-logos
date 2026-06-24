# Design: Version-Aware Global Index

**Date:** 2026-06-24
**Status:** Approved

## Problem

`system.db`'s `page_index` and `active_page` tables only store `iroll_name` (e.g. `cat`), not `iroll_version` (e.g. `base`). When the iroll build tag system (`name:version`) was introduced, the global index was never updated. Any operation that reads the index—`DeletePage`, `GetActive`, `SwitchPage`—hardcodes `"latest"` as the version, producing a path like `~/.iroll/cat/latest/ai_roll.db` that may not exist.

## Solution

Add `iroll_version` columns to both `page_index` and `active_page`. Thread version through every function and command that touches the index.

## Detailed Changes

### 1. store/system.go — Data Access Layer

**Schema** — `ensureSystemTables()`:
```sql
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
```

**`IndexPage`** — add `version` parameter, INSERT into both tables with version:
```go
func IndexPage(irollName string, version string, pageID string, cwd string) error
```

**`GetActive`** — return version alongside name and pageID:
```go
func GetActive(cwd string) (string, string, string, error)
// returns (name, version, pageID, error)
```

**`ListAllPages`** — SELECT includes `p.iroll_version`, output map includes key `"iroll_version"`.

**`SwitchPage`** — read version from `page_index`, return it, upsert `active_page` with version:
```go
func SwitchPage(pageID string) (string, string, error)
// returns (name, version, error)
```

**`DeletePage`** — read version from `page_index` (not hardcoded `"latest"`), pass to `DbPath(name, version)`.

**`CleanIndex`** — no signature change needed (deletes by name, same behavior).

### 2. cmd/ — Command Layer

**`store` caller reference** — every call site that receives `version` from `store` must pass it to `checkedDbPath`/`checkedIrollPath` instead of hardcoding `"latest"`.

| File | Function | Change |
|---|---|---|
| `page.go` | `pageNewCmd` | `IndexPage(name, version, pageID, cwd)` — already has version |
| `page.go` | `pageCurrentCmd` | Destructure `name, version, pageID := store.GetActive(cwd)`; use `checkedDbPath(name, version)` |
| `page.go` | `pageSwitchCmd` | Destructure `name, version := store.SwitchPage(pageID)` |
| `page.go` | `pageDeleteCmd` | No direct change needed (DeletePage reads version internally) |
| `context.go` | `getContextCmd` | `resolvePage` now returns version when name from CLI (ParseTag) or from GetActive |
| `context.go` | `updateContextCmd` | Same as getContext |
| `context.go` | `resolvePage` | Return `(name, version, pageID)`. CLI path: `ParseTag` + existing pageID logic. GetActive path: returns all three. |
| `memory.go` | `queryMemoryCmd` | `GetActive` returns version, pass to `checkedDbPath` |
| `query_dna.go` | `queryDnaCmd` | Same |
| `loop.go` | `openActiveLoop` | `GetActive` returns version, pass to `DbPath(name, version)` |
| `book.go` | `resolveBookRoll` | Return `(name, version)`. CLI path: `ParseTag`. GetActive path: returns version. |
| `book.go` | `openBookDB` | Accept `(name, version)`, call `DbPath(name, version)` |
| `book.go` | `runBookQuery` | `IrollPath(name, version)` not hardcoded `"latest"` |
| `skill.go` | `resolveSkillRoll` | Same pattern as `resolveBookRoll` |
| `skill.go` | `openSkillDB` | Same pattern as `openBookDB` |
| `skill.go` | `SkillListCmd`/`SkillShowCmd` | `IrollPath(name, version)` not hardcoded `"latest"` |

### 3. Files NOT Changed

- `cmd/hub_pull.go` — does its own name/version parsing, not affected
- `cmd/load.go` — reads name from zip metadata, not from index
- `cmd/rm.go` — takes CLI arg directly, already fixed with `ParseTag`
- `cmd/save.go` — takes CLI arg directly, already fixed with `ParseTag`
- `cmd/history.go` — takes CLI arg directly, already fixed with `ParseTag`
- `cmd/inspect.go` — takes CLI arg directly, already fixed with `ParseTag`

### 4. Tests

Update existing tests that call:
- `IndexPage` — add version argument
- `GetActive` — accept third return value
- `SwitchPage` — accept second return value
- Any test constructing page_index/active_page rows — add version column

## Impact Summary

- **Breaking change**: `GetActive` and `SwitchPage` return signatures change. All callers must be updated.
- **No backward compatibility needed**: project is pre-release.
- **All `"latest"` hardcodes eliminated** from the cmd layer except `load.go` (which reads from zip, not user input, and can be addressed separately if needed).
