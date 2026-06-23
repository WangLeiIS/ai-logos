# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Logos is an AI agent state and knowledge management system. It provides a standardized `.iroll` package format (ZIP archive + SQLite database) for storing agent personality, memory, loop behaviors, knowledge, and resources.

**Core principle: The system doesn't integrate any agent capabilities—agents use us.**

## Build and Test Commands

### Logos CLI

```bash
# Build the CLI
cd iroll
go build -o ../logos .

# Run the CLI
./logos status

# Build an agent from Layerfile
./logos roll build -f examples/base-agent/Layerfile -t my-agent

# Create a page and start a conversation
./logos page new my-agent --cwd .

# Get agent context
./logos page get-context --cwd .
```

### irollhub (HTTP Service)

```bash
cd irollhub
go run -tags sqlite_fts5 . config.yaml
```

### Running Tests

```bash
# Test specific packages
cd iroll
go test ./db/...
go test ./book/...
go test ./builder/...
go test ./cmd/...

# Run all tests
go test ./...

# Run irollhub tests
cd ../irollhub
go test -tags sqlite_fts5 ./...
```

## High-Level Architecture

The project consists of two independent Go modules:

### 1. logos (iroll/) - CLI Tool

**Package Structure:**

- `builder/` - Layerfile layered build system (FROM/MIGRATE/COPY instructions)
- `book/` - Book Bundle validation and retrieval (Parquet-based knowledge chunks)
- `cmd/` - Cobra CLI command implementations
- `db/` - SQLite database operations (pages, memory, dna, loop, loop_runs)
- `store/` - Storage management (~/.iroll/ directory, ZIP extraction)
- `safepath/` - Path security validation (prevents directory traversal)

**Core Concepts:**

- **iroll Package**: ZIP archive containing `ai_roll.db` and `Resources/` directory. Loaded to `~/.iroll/<name>/`
- **Page**: Each conversation creates a page, inheriting from template page (page_id=0). Each working directory tracks its active page.
- **Context**: JSON-formatted behavioral instructions supporting three value types:
  - Pure strings: `"key": "value"`
  - File references: `"key": {"@file": "path"}` - reads from iroll package
  - SQL queries: `"key": {"@sql": "SELECT ..."}` - queries ai_roll.db
- **DNA**: Decision genes via Q&A pairs defining agent's decision mechanism across four dimensions (cognitive, ethical, aesthetic, ontological)
- **Loop**: Reusable behavior seeds agent can autonomously choose. `loop_runs` stores page-independent execution state. Each page has at most one active main run with optional one-level child runs.
- **Book Bundle**: Located at `Resources/books/<book-id>/` with manifest.json and three Parquet files. Logos validates and registers at build time; queries use exact tag retrieval with scoring.

**Database Structure (ai_roll.db per iroll):**
- metadata - key-value metadata
- dna - decision genes
- loop - reusable behavior seeds
- loop_runs - page-specific execution records
- pages - page contexts
- memory - page-isolated Q&A memory
- book - Book Bundle metadata
- history - build history

### 2. irollhub - HTTP Registry Service

Three-tier structure: Organization → Package → Version

Addressing format: `org/package:version` (e.g., `official/cat-agent:latest`)

Architecture: HTTP service with SQLite for metadata, MinIO for .iroll file storage. Pure API service, no frontend.

**Key APIs:**
- Authentication: GitHub/Google OAuth → API Key
- Organizations/Packages/Versions: CRUD operations
- Search: Full-text search across packages

Build and test irollhub with `-tags sqlite_fts5`; its search schema uses SQLite FTS5.

## Important Implementation Details

### Path Security

All iroll names and paths must pass `safepath` validation to prevent directory traversal attacks. Use `safepath.ValidateName()` for names and `safepath.Join()` for path construction.

### SQLite Concurrency

SQLite WAL mode with busy retries is used for concurrency. Pattern:
```go
delay := time.Millisecond
for attempt := 0; ; attempt++ {
    err := operationOnce()
    if !isSQLiteBusyOrLocked(err) || attempt == maxRetries {
        return err
    }
    time.Sleep(delay)
    delay *= 2
}
```

### Context Resolution

When reading context, `@file` and `@sql` references are resolved to actual values. When writing, raw markers are stored. The `loop` field is dynamically injected by `BuildLoopContext()` during `get-context` operations.

### Loop Run Lifecycle

- Status: `active → completed | aborted`
- Only one active main run per page
- Optional one-level child runs (no deeper nesting)
- Main run with active children cannot be completed
- Runs are immutable after completion; only reflection can be appended/replaced

### CGO Dependency

The project requires CGO (go-sqlite3). On Windows, install GCC (MinGW-w64 or TDM-GCC). irollhub also requires the `sqlite_fts5` Go build tag for its FTS5 search table.

## Two Independent Go Modules

The `iroll/` and `irollhub/` directories are separate Go modules with their own `go.mod` files. When working on one module, operate within its directory.

## Documentation Reference

Primary documentation: README.md and docs/rebot-roll.md

Note: docs/superpowers/ contains dated design/implementation records which may include historical terminology and plans that have been superseded by current designs.
