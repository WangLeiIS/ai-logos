# Loop Context Design

## Goal

Loop defines what an agent may choose to pursue. Logos does not execute, schedule, pause, or plan loop work. It stores reusable behavior seeds, records the agent's autonomous runs, and dynamically presents loop state as an important part of page context.

Pages are independent workspaces. The same loop seed may be pursued concurrently by multiple pages, with separate plans, progress, outcomes, and reflections.

## Design Principles

- The agent chooses what to pursue and how to pursue it.
- Logos manages context and records; it is not a task runner.
- A loop seed describes a reusable intention, not a globally active todo item.
- A loop run is an immutable life record after it ends.
- Each page has independent loop runs.
- Runtime state has one source of truth.
- Keep the state machine and hierarchy deliberately small.

## Considered Approaches

### 1. Store Loop Runtime State in `pages.context`

Each page copies loop definitions and runtime state into its context JSON.

This makes loop state immediately visible, but creates difficult partial updates, duplicates seed data, makes history fragile when pages are deleted, and couples lifecycle commands to arbitrary context JSON edits.

### 2. Store Runtime State in Both Context and `loop_runs`

The page context and `loop_runs` both contain the full current plan and progress.

This preserves history, but introduces two sources of truth and requires transactional synchronization for every update.

### 3. Dynamic Context View Backed by `loop_runs` — Selected

The `loop` table stores reusable behavior seeds. The `loop_runs` table stores page-specific runtime state and life records. `pages.context` stores no loop runtime data. When Logos returns page context, it dynamically injects the page's focus and the available loop seeds.

This keeps loop central to context without duplicating state.

## Conceptual Model

### Loop Seed

A loop seed describes something the agent may decide to pursue. Seeds are reusable and have no global execution status. Logos does not decide whether a seed should run again.

Examples:

- Understand the current identity and constraints.
- Review memory for contradictions.
- Improve knowledge coverage for a topic.

### Loop Run

A loop run records one autonomous decision by an agent, within one page, to pursue one seed. It contains the agent's plan, current progress, final result, and later reflection.

Multiple pages may run the same seed concurrently. Each run remains independent.

### Page Focus

Each page may have:

- At most one active main run.
- Zero or more active child runs.
- Child runs only one level deep.

Detailed temporary steps belong in the main run's `plan`; they do not become loop seeds or child runs.

## Data Model

### `loop`

The seed table contains only reusable behavior definitions.

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
```

Rules:

- `name` is a stable short identifier.
- `describe` is a concise human-readable summary.
- `content` is the complete behavioral seed presented to the agent.
- `weight` helps the agent prioritize available seeds and must be between `0` and `1`.
- Active seeds have `archived_at IS NULL`.
- Seeds with run history cannot be physically deleted; they may only be archived.
- Seeds without run history may be deleted.
- The table has no `type`, global `status`, `executed_count`, or `result`.
- Execution statistics are calculated from `loop_runs`.

### `loop_runs`

The run table is the source of truth for current runtime state and historical life records.

```sql
CREATE TABLE loop_runs (
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
```

Rules:

- `status` is `active`, `completed`, or `aborted`.
- `plan`, `progress`, `result`, and `reflection` contain arbitrary valid JSON.
- CLI text shorthand is encoded as a JSON string.
- Seed snapshot fields preserve the exact intention that started the run.
- Editing or archiving a seed does not change existing runs.
- A child run must belong to the same page as its parent.
- A child run cannot itself have children.
- A page may have at most one active run with `parent_run_id IS NULL`.
- Completed or aborted runs cannot change plan, progress, result, status, or abort reason.
- Reflection may be added or replaced after a run ends.

Recommended indexes:

```sql
CREATE INDEX idx_loop_runs_page_status
ON loop_runs(page_id, status);

CREATE INDEX idx_loop_runs_parent_status
ON loop_runs(parent_run_id, status);

CREATE INDEX idx_loop_runs_loop_started
ON loop_runs(loop_id, started_at DESC);

CREATE UNIQUE INDEX idx_loop_runs_one_active_main
ON loop_runs(page_id)
WHERE status = 'active' AND parent_run_id IS NULL;
```

The partial unique index enforces one active main run per page even when multiple processes issue commands concurrently. Parent ownership, one-level depth, ended-run immutability, and valid lifecycle transitions are enforced transactionally by the application.

## Lifecycle

The lifecycle is intentionally small:

```text
active → completed
       ↘ aborted
```

Planning is content inside `plan`, not a lifecycle state.

Ending a main run is rejected while it has active child runs. The agent must explicitly complete or abort each child first.

Deleting a page automatically aborts all active runs belonging to that page with `abort_reason = "page_deleted"`.

## Dynamic Page Context

Raw `pages.context` does not store loop state or run IDs. Loop context is generated whenever Logos resolves a page context.

Resolved context gains a reserved top-level `loop` field:

```json
{
  "loop": {
    "focus": {
      "main": {
        "run_id": 42,
        "seed": {
          "id": 3,
          "name": "self-cognition",
          "describe": "Understand current identity",
          "content": "Read context and DNA, then form a coherent self-understanding.",
          "weight": 0.9
        },
        "status": "active",
        "plan": {},
        "progress": {},
        "started_at": "2026-06-09T00:00:00Z",
        "updated_at": "2026-06-09T00:10:00Z"
      },
      "children": []
    },
    "available": [
      {
        "id": 4,
        "name": "memory-review",
        "describe": "Review memory",
        "content": "Review memories for useful context and contradictions.",
        "weight": 0.8,
        "stats": {
          "active": 1,
          "completed": 5,
          "aborted": 1,
          "last_ended_at": "2026-06-08T12:00:00Z",
          "last_result": {}
        }
      }
    ]
  }
}
```

Rules:

- `focus.main` is `null` when the page has no active main run.
- `focus.children` contains active children of the active main run.
- `available` contains all non-archived seeds with aggregate run statistics.
- Available statistics are global to the iroll, across all pages.
- Full historical runs are not injected into context.
- If raw context already contains a top-level `loop` field, dynamic resolution replaces it. `loop` is reserved by Logos.
- `page update-context` cannot modify runtime loop state.

## CLI

Loop commands resolve the current page through `--cwd`, following existing page command behavior.

### Seed Management

```bash
logos loop list [--archived] [--cwd .]
logos loop inspect <name> [--cwd .]
logos loop add <name> --describe <text> --content <text> [--weight 0.5] [--cwd .]
logos loop edit <name> [--describe <text>] [--content <text>] [--weight <n>] [--cwd .]
logos loop remove <name> [--cwd .]
logos loop archive <name> [--cwd .]
logos loop restore <name> [--cwd .]
```

`remove` physically deletes only seeds with no runs. Otherwise it fails and instructs the caller to archive.

### Run Lifecycle

```bash
logos loop run <name> [--parent <main-run-id>] [--plan <json-or-text>] [--cwd .]
logos loop update [run-id] [--plan <json-or-text>] [--progress <json-or-text>] [--cwd .]
logos loop complete [run-id] --result <json-or-text> [--cwd .]
logos loop abort [run-id] --reason <text> [--result <json-or-text>] [--cwd .]
logos loop reflect <run-id> --content <json-or-text> [--cwd .]
logos loop current [--cwd .]
logos loop history <name> [--page <page-id>] [--limit <n>] [--cwd .]
logos loop show <run-id> [--cwd .]
```

Rules:

- `loop run <name>` creates a main run for the current page.
- Creating a main run fails when the page already has an active main run.
- `--parent` creates a child run and must reference the current page's active main run.
- Omitting `run-id` from `update`, `complete`, or `abort` targets the current page's active main run.
- Child runs must always be addressed explicitly.
- `update` fully replaces each supplied JSON field; omitted fields remain unchanged.
- `complete` and `abort` fail for a main run with active children.
- `reflect` only accepts completed or aborted runs.

Command names describe record transitions. `loop run` does not cause Logos to execute work.

## Data Flow

### Creating a Page

1. Logos creates a page from the template context.
2. The new page starts with no active runs.
3. Resolved context contains `focus.main = null`, no children, and the available seed view.

### Starting a Main Run

1. The agent reads resolved context and chooses a seed.
2. `logos loop run <name>` verifies the page has no active main run.
3. Logos creates a run with a seed snapshot and optional plan.
4. The next resolved context presents the run as `focus.main`.

### Starting a Child Run

1. The agent explicitly chooses another existing seed.
2. `logos loop run <name> --parent <main-run-id>` verifies the parent is the current page's active main run.
3. Logos verifies the parent is not itself a child.
4. The next resolved context presents the run under `focus.children`.

### Updating a Run

1. The agent revises its plan or records progress.
2. `logos loop update` validates JSON-or-text inputs.
3. Logos replaces supplied fields in `loop_runs`.
4. The updated values appear in the next resolved context.

### Ending a Run

1. Logos verifies the run is active.
2. For a main run, Logos verifies there are no active children.
3. Logos records `completed` with a result, or `aborted` with a reason and optional result.
4. The run disappears from page focus but remains in history and aggregate statistics.

## Error Handling

All failures follow the existing JSON error output convention.

Errors include:

- Seed not found or archived when starting a run.
- Duplicate seed name.
- Invalid weight.
- Invalid JSON input.
- No active page for cwd.
- Active main run already exists.
- Default main-run operation requested when no active main exists.
- Child run does not belong to the current page or parent.
- Attempt to create a grandchild run.
- Attempt to modify an ended run.
- Attempt to reflect on an active run.
- Attempt to end a main run with active children.
- Attempt to remove a seed with run history.

State transitions and page deletion cleanup must be transactional.

## Testing

Tests cover:

- Seed add, edit, archive, restore, and deletion rules.
- Seed snapshot preservation after seed edits.
- Concurrent runs of the same seed across independent pages.
- At most one active main run per page.
- Multiple active children and rejection of grandchildren.
- Main-run default targeting and explicit child targeting.
- Arbitrary JSON and text shorthand for plan, progress, result, and reflection.
- Full replacement semantics for supplied update fields.
- Rejection of updates after completion or abortion.
- Rejection of ending a main run with active children.
- Automatic abortion of active runs when a page is deleted.
- Dynamic context focus and available-seed injection.
- Reserved `loop` context key replacement.
- Aggregate statistics and history queries.
- Transaction rollback on failed lifecycle transitions.

## Out of Scope

- Scheduling, timers, cron behavior, or background execution.
- Logos deciding which loop the agent should pursue.
- Automatic continuation of runs between pages.
- Cross-page run ownership transfer.
- More than one child level.
- Event-by-event progress history.
- Automatic conversion of plan steps into loop seeds.
- Global single-execution or periodic-execution restrictions.
