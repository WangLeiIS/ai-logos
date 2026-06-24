# Design: Loop Command Restructure

**Date:** 2026-06-24
**Status:** Approved

## Problem

The `loop current` command conflates two distinct concerns:
1. What loop runs are currently active for this page (focus)
2. What loop seeds are available to start (available + stats)

Logos is a stateless CLI tool — "current" has no meaningful state beyond what the `active_page` index provides. The command name misleads, and the output mixes run-level and seed-level information.

Additionally, there is no simple way to list active runs (`docker ps` style).

## Solution

Remove `loop current`. Replace with `loop ps` for run listing. Merge available-seeds stats into `loop list --stats`.

## New Command Tree (13 commands)

```
loop ps [-a]              NEW: list active (or all) runs for the current page
loop list [--stats]       ENHANCED: list seeds, optionally with run stats
loop inspect <name>       show one seed's details
loop add <name>           create a seed
loop edit <name>          edit a seed
loop remove <name>        delete a seed (only if no run history)
loop archive <name>       archive a seed
loop restore <name>       restore an archived seed

loop run <name>           start a run from a seed
loop update [run-id]      update active run plan/progress
loop complete [run-id]    complete a run with result
loop abort [run-id]       abort a run with reason
loop reflect <run-id>     append reflection to an ended run
loop show <run-id>        show full run detail
loop history <name>       show run history for a seed
```

## Removed

- `loop current` — replaced by `loop ps` + `loop list --stats`

## New Commands

### `loop ps`

List runs for the current page.

```
logos loop ps        # active runs only (main first, then children)
logos loop ps -a     # all runs (active + completed + aborted)
```

Output: array of `LoopRun` objects, ordered by status (active first), then by id.

### `loop list --stats`

When `--stats` is set, each seed entry includes run counts and last result:

```json
[
  {
    "id": 1,
    "name": "review",
    "describe": "Review memory",
    "weight": 0.8,
    "stats": {
      "active": 1,
      "completed": 5,
      "aborted": 0,
      "last_ended_at": "...",
      "last_result": {...}
    }
  }
]
```

When `--stats` is not set, output is unchanged from current behavior (flat seed list).

## Changes to `page get-context`

The `loop` field in context currently contains `LoopContextView { focus, available }`. Replace it with two fields:

```json
{
  "loop_focus":     [LoopRun],        // from loop ps (active runs)
  "loop_available": [AvailableLoopSeed]  // from loop list --stats
}
```

This decouples the two concerns in the context output as well.

## Files to Change

| File | Change |
|---|---|
| `cmd/loop.go` | Remove `newLoopCurrentCmd`, add `newLoopPsCmd` |
| `cmd/loop_run.go` | Add `outputLoopPs` + `runLoopPs` functions |
| `cmd/loop_seed.go` | Modify `newLoopListCmd` to accept `--stats` flag, update `outputLoopList` |
| `db/loop_context.go` | Split `BuildLoopContext` into `ListActiveRuns(pageID)` and keep `listAvailableLoopSeeds`. Update or remove `LoopContextView`. |
| `db/context resolution` | Update `BuildLoopContext` call site to use two separate calls |
| Tests | Update all tests referencing `loop current`, `LoopContextView`, `BuildLoopContext` |

## Not Changed

- Seed CRUD: `add`, `edit`, `remove`, `archive`, `restore`, `inspect`
- Run lifecycle: `run`, `update`, `complete`, `abort`, `reflect`
- Run inspection: `show`, `history`
