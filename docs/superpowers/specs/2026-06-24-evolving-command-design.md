# Evolving Command Design

**Date:** 2026-06-24
**Status:** Draft

## Overview

Add a `logos roll evolving` command that executes arbitrary SQL statements against an iroll's `ai_roll.db` database. Supports both read (SELECT) and write (INSERT/UPDATE/DELETE) operations, with a dry-run mode for safe preview.

## Motivation

Currently there is no direct way to execute SQL against a loaded iroll's database from the CLI. SQL can only be embedded in context via `@sql` markers (read-only, restricted to SELECT). The `evolving` command fills this gap, enabling:

- Batch initialization of DNA, memory, loop seeds via SQL files
- Quick ad-hoc mutations to agent data
- Inspection beyond what `inspect` provides
- Scripted agent evolution

## Command Signature

```
logos roll evolving [name:version] [sql] [flags]
```

### Target iroll Resolution

Two modes, prioritized:

1. **Explicit tag:** First positional argument parsed via `builder.ParseTag()` → `name:version`
2. **Auto-detect:** When no explicit tag, resolve active iroll from `--cwd` via `store.GetActive(cwd)`

Tag detection heuristic:
1. If the first positional argument contains spaces → it's SQL (iroll names never contain spaces)
2. If it passes `safepath.ValidateName()` → attempt `builder.ParseTag()` to parse as `name:version`
3. Otherwise → treat it as part of the SQL

When in doubt, use `--sql` or `--file` to disambiguate.

### SQL Input Sources

Priority order:

1. `--sql` flag — explicit SQL string
2. Positional arguments — remaining args after tag resolution, joined with spaces
3. `--file` flag — read entire file content as SQL
4. stdin — when no SQL args and no `--file`, read from standard input

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cwd` | string | `.` | Working directory (auto-detect mode) |
| `--sql` | string | `""` | SQL statement(s) to execute |
| `--file` | string | `""` | Path to SQL file |
| `--dry-run` | bool | `false` | Preview mode: execute in transaction then rollback |

### Usage Examples

```bash
# Explicit tag + --sql flag
logos roll evolving my-agent:latest --sql "UPDATE dna SET weight=0.99 WHERE name='idea-self-relation'"

# Positional SQL (auto-detect iroll from cwd)
logos roll evolving "SELECT * FROM dna ORDER BY weight DESC LIMIT 5"

# Execute SQL file against specific iroll
logos roll evolving my-agent:v0.1.0 --file init_data.sql

# Dry-run preview
logos roll evolving --file init_data.sql --dry-run

# Pipe from stdin
cat init_data.sql | logos roll evolving my-agent:latest

# No target, auto-detect from cwd, pipe from stdin
echo "SELECT count(*) FROM memory" | logos roll evolving
```

## SQL Execution Engine

### Statement Splitting

- Split raw SQL by `;` separator
- Trim whitespace from each statement
- Skip empty statements
- Execute sequentially in order
- Stop at first error

### Execution Flow

```
open ai_roll.db → for each statement → detect type → execute → collect result
```

**For mutations (INSERT/UPDATE/DELETE/ALTER/etc.):**
- Execute via `db.Exec()`
- Collect `rowsAffected`

**For queries (SELECT/PRAGMA/EXPLAIN):**
- Execute via `db.Query()`
- Collect column names and all rows
- Convert to [][]string

### Dry-Run Mode

```
BEGIN TRANSACTION → execute all statements → ROLLBACK
```

All statements execute against a real connection in a transaction, so `affected_rows` reflects what *would* happen. The ROLLBACK ensures no persistent changes.

Caveat: `affected_rows` may differ slightly from actual execution due to SQLite internals (triggers, cascades). This is noted in help text.

### Error Handling

On error:
- Stop immediately (no further statements executed)
- Return error with `executed` count (how many succeeded before the failure) and `total` count
- SQLite busy/locked errors: use existing retry pattern (exponential backoff, max retries)

## Output Format

### Success

Returns a JSON array, one result object per executed statement:

```json
[
  {
    "type": "affected",
    "statement": "UPDATE dna SET weight=0.99 WHERE name='idea-self-relation'",
    "affected_rows": 1
  },
  {
    "type": "rows",
    "statement": "SELECT name, weight FROM dna ORDER BY weight DESC",
    "columns": ["name", "weight"],
    "rows": [
      ["idea-human-relation", "0.95"],
      ["emotion-selection-principle", "0.9"]
    ],
    "count": 2
  }
]
```

### Error

```json
{
  "error": "near \"UPDAT\": syntax error",
  "executed": 2,
  "total": 5
}
```

## Code Structure

### New Files

| File | Purpose |
|------|---------|
| `iroll/cmd/evolving.go` | Cobra command, flag parsing, input routing |
| `iroll/cmd/evolving_test.go` | Integration tests |
| `iroll/db/evolving.go` | SQL execution engine |
| `iroll/db/evolving_test.go` | Unit tests |

### Core Types & Functions (`iroll/db/evolving.go`)

```go
type EvolvingResult struct {
    Type         string     `json:"type"`          // "rows" | "affected"
    Statement    string     `json:"statement"`
    Columns      []string   `json:"columns,omitempty"`
    Rows         [][]string `json:"rows,omitempty"`
    Count        int        `json:"count"`
    AffectedRows int64      `json:"affected_rows,omitempty"`
}

func SplitSQL(raw string) []string
func ExecuteOne(db *sql.DB, stmt string, dryRun bool) (EvolvingResult, error)
func ExecuteAll(db *sql.DB, rawSQL string, dryRun bool) ([]EvolvingResult, error)
```

### Command Registration

Registered under `rollCmd` in `init()`, sibling to `inspect`, `build`, `load`:

```go
func init() {
    rollCmd.AddCommand(evolvingCmd)
}
```

## Testing Strategy

### Unit Tests (`iroll/db/evolving_test.go`)

- `SplitSQL` with various inputs (single stmt, multiple, empty, trailing semicolon, whitespace)
- `ExecuteOne` with SELECT, INSERT, UPDATE, DELETE against in-memory SQLite
- `ExecuteAll` sequential execution, error stopping
- Dry-run: verify ROLLBACK (data unchanged after dry-run)

### Integration Tests (`iroll/cmd/evolving_test.go`)

- End-to-end: build an iroll, run evolving with `--sql`, verify changes persisted
- End-to-end: `--file` flag
- End-to-end: `--dry-run` flag (no persistence)
- End-to-end: stdin input
- Error case: invalid SQL, missing target

## Non-Goals

- SQL syntax highlighting or validation beyond what SQLite provides
- Stored procedure support
- Interactive REPL mode
- Transaction control flags (BEGIN/COMMIT handled internally for dry-run only)
- Cross-iroll SQL execution (one database per invocation)
