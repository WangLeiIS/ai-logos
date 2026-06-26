---
name: logos-1
description: |
  Use the logos CLI tool to manage AI agent state, knowledge, iroll packages, and pages.
  Invoke this skill whenever you need to persist conversation context, load an agent's personality,
  create a new memory page, or interact with .iroll packages. This includes: initializing the logos system,
  building or loading iroll packages, creating/switching/deleting pages, reading page context to determine
  how to behave, per-key context edits (`page set`/`page unset`), setting page aliases (`page alias`),
  running raw SQL against a page's outer db (`page query`), querying page memories, recording autonomous
  loop work (`loop run`/`complete`/`abort`/`reflect`/`ps`/`history`), and querying registered books.
  Commands that mutate page/loop/dna/memory state emit a three-line JSON output protocol
  (`{"status":"ok"}` plus optional data plus optional `{"hints":[...]}`); parse accordingly. Use this skill
  for ANY task involving logos, iroll, page management, agent memory, knowledge, or persistent context.
---

# Logos — AI Agent State and Knowledge Management

Logos is a CLI tool for persistent agent state and knowledge. It stores personality, context, memories, and registered knowledge resources in `.iroll` packages. Each conversation session uses a "page" — a record that holds behavioral context and links to the iroll.

The core workflow: **load an iroll → create a page → read its context → follow those instructions → query and record structured state as needed.**

## Startup Sequence

Every conversation should begin with this sequence. It's fast (three commands) and ensures you're operating with the right context.

### Step 1: Check status

```bash
logos status
```

Returns how many iroll packages exist, how many pages are recorded, and which rolls are available. Use the result to decide what to do next.

- `iroll_count: 0` → need to build or load an iroll package
- `page_count: 0` → need to create a page before working

### Step 2: Ensure an iroll package exists

```bash
logos roll list
```

If empty, build or load one:

```bash
# Build from an Irollfile (defaults to ./Irollfile)
logos roll build -t my-agent

# Or load an existing .iroll file
logos roll load ./my-agent.iroll
```

If multiple iroll packages exist and the user hasn't specified which one to use, ask the user to choose before proceeding.

### Step 3: Create a page

```bash
logos page new <iroll-name> --cwd .
```

This creates a new page and automatically sets it as the active page for the current working directory. The new page inherits its initial context from the template page (page_id=0), which typically defines the agent's baseline behavior through keys like `system_prompt` (personality/role), `response_contract` (output format), and `dna` (decision genes) — not just `system_prompt`.

The returned JSON includes `page_id` — you can reference it later if needed.

### Step 4: Read context and follow it

```bash
logos page get --cwd .
```

The `context` field is a JSON string. Parse it and follow the instructions inside. This is your behavioral blueprint — it tells you how to respond, what persona to adopt, what rules to follow.

**The context is your directive. Read it once, then execute accordingly for the rest of the conversation.**

## During Conversation

### Query page memories

```bash
logos page query-memory --keyword "Python" --cwd .
logos page query-memory user-prefers-python --full --cwd .
```

Memory is isolated by page. Summary output omits `content`; add `--full` when the complete record is needed. Logos currently has no manual `add-memory` CLI; memory creation is reserved for future context compression and DB-level integrations.

### Update page context

When behavioral instructions change during a conversation:

```bash
logos page set --content '{"system_prompt":"新的指令"}' --cwd .
logos page set user_context.project blog --cwd .
```

The content must be valid JSON. The entire context is replaced, so include all fields.

### View all pages

```bash
logos page list --cwd .
```

### Switch to a different page

```bash
logos page switch <page-id>
```

### Delete a page

```bash
logos page delete <page-id>
```

### Query registered books

The calling agent extracts exact tags from the user's question, then asks Logos to retrieve original chunks. Logos performs deterministic retrieval; use the returned chunks to answer the user.

```bash
logos book list --cwd .
logos book inspect <book-id> --cwd .
logos book query --book <book-id> --tag <tag> --tag <tag> --cwd .
```

Use repeated `--book` flags for multi-book search. Do not pass the full natural-language question as a tag unless it is intentionally indexed as one exact tag.

Note: `book` commands still emit the OLD single-line JSON envelope (one JSON document per line), not the three-line `status`/data/`hints` protocol used by `page`/`loop`/`query-dna`/memory. Parse book output differently — there is no leading `{"status":"ok"}` line and no trailing `{"hints":[...]}` line.

### Choose and record autonomous loop work

Resolved page context includes `loop.focus` for the current page and `loop.available` seeds. Logos never executes loop work; you choose a seed, perform the work yourself, and use commands to record progress.

```bash
logos loop ps --cwd .
logos loop run <name> --plan '{"steps":["first step"]}' --cwd .
logos loop update --progress '{"completed":["first step"]}' --cwd .
logos loop complete --result '{"summary":"done"}' --cwd .
logos loop history <name> --cwd .
```

Each page has an independent active main run. Child runs use `--parent <main-run-id>` and are limited to one level. End children before completing or aborting the main run.

## Command Reference

| Command | Purpose |
|---------|---------|
| `logos status` | System status (iroll count, page count) |
| `logos roll list` | List all iroll packages |
| `logos roll build -f <file> -t <name>` | Build iroll from Irollfile |
| `logos roll load <file>` | Load .iroll file into ~/.iroll/ |
| `logos roll rm <name>` | Remove an iroll package |
| `logos roll save <name> [-o path]` | Export iroll to .iroll file |
| `logos roll inspect <name>` | Show iroll details |
| `logos roll history <name>` | Show build history |
| `logos roll evolving [name] [--cwd .]` | Show evolving (uncommitted) changes for an iroll |
| `logos page new <name> [--cwd .]` | Create new page |
| `logos page list [name] [--cwd .]` | List pages |
| `logos page switch <page-id>` | Switch active page |
| `logos page delete <page-id>` | Delete a page |
| `logos page default set <name> [--cwd .]` | Set the cwd's default iroll |
| `logos page default show [--cwd .]` | Show the cwd's default iroll |
| `logos page default clear [--cwd .]` | Clear the cwd's default iroll |
| `logos page get [path] [--page <id>] [--alias <name>] [--roll <name>] [--cwd .]` | Get full context or a single resolved key |
| `logos page set <path> <value> [--page <id>] [--alias <name>] [--roll <name>] [--cwd .]` | Set a context key (json-or-text) |
| `logos page set --content '<json>' [--page <id>] [--alias <name>] [--roll <name>] [--cwd .]` | Replace the whole context |
| `logos page unset <path> [--page <id>] [--alias <name>] [--roll <name>] [--cwd .]` | Delete a context key |
| `logos page alias <name> [--page <id>] [--clear]` | Set/clear page alias |
| `logos page query [sql] [--sql <stmt>] [--file <p>] [--page <id>] [--alias <name>] [--dry-run] [--cwd .]` | Raw SQL on this page's outer db |
| `logos page query-memory [name] [--keyword <text>] [--min-importance <n>] [--since <ts>] [--before <ts>] [--limit <n>] [--full] [--cwd .]` | Query current-page memories |
| `logos page query-dna <name> [--type <type>] [--cwd .]` | Fuzzy search dna by name |
| `logos skill list [name] [--cwd .]` | List registered skills |
| `logos skill show <skill-id> [name] [--cwd .]` | Show registered skill metadata/content |
| `logos book list [name] [--cwd .]` | List registered books |
| `logos book inspect <book-id> [name] [--cwd .]` | Inspect registered book metadata |
| `logos book query --book <id>... --tag <tag>... [--limit 10] [--per-book-limit 5] [--cwd .]` | Retrieve original chunks by exact tags |
| `logos loop list [--archived] [--cwd .]` | List reusable loop seeds |
| `logos loop inspect <name> [--cwd .]` | Inspect a single loop seed |
| `logos loop run <name> [--parent <id>] [--plan <value>] [--cwd .]` | Record an autonomous run choice |
| `logos loop update [run-id] [--plan <value>] [--progress <value>] [--cwd .]` | Replace supplied active-run fields |
| `logos loop complete [run-id] --result <value> [--cwd .]` | Complete an active run (`--result` required) |
| `logos loop abort [run-id] --reason <value> [--cwd .]` | Abort an active run (`--reason` required) |
| `logos loop reflect <run-id> --content <value> [--replace] [--cwd .]` | Append/replace reflection on a run (positional `<run-id>` + `--content` required) |
| `logos loop ps [--cwd .]` | Show the current page's run tree (the successor to the deleted `current` subcommand) |
| `logos loop history <name> [--cwd .]` | Show past runs of a loop seed |
| `logos loop show [run-id] [--cwd .]` | Show one run in detail |

## Key Concepts

- **iroll** — a package containing an agent's complete state (two databases + resources), stored in `~/.iroll/<name>/`. The read-only `roll-inner.db` holds the blueprint (metadata/dna/loop seeds/skill/book/history + template rows); the `roll-outer.db` is the template, copied per working directory to `<cwd>/.iroll/<name>.db`. The outer db is opened as the main connection and the inner is ATTACHed AS `inner.` — so any `@sql` reading an inner table MUST prefix it with `inner.` (e.g. `SELECT value FROM inner.metadata WHERE ...`). A bare table name like `metadata` silently hits the outer db, which has no such table.
- **page** — a conversation session record with context (behavioral JSON) and a link to the memory store
- **context** — a JSON object stored in a page. Keys are free-form. Values support three types:
  - Plain string: `"system_prompt": "你是一个助手"`
  - File reference: `"greeting": {"@file": "Resources/greeting.txt"}` — reads file content from iroll package
  - SQL query: `"description": {"@sql": "SELECT value FROM inner.metadata WHERE key = 'description'"}` — queries `roll-inner.db` (note the `inner.` prefix; inner tables are ATTACHed AS `inner.`)
- **template page** — page_id=0 stores the default context; new pages inherit from it
- **active page** — each working directory tracks its own active page via system.db
- **dna** — decision-making Q&A pairs defining agent behavior. Context loads questions only (no answers); use `query-dna` to retrieve full records on demand
- **loop seed** — reusable behavioral intention the agent may choose; it has no global execution status
- **loop run** — page-scoped plan, progress, result, and reflection record. Logos records it but never executes the work
- **memory** — page-scoped Q&A-style experience records with importance and sleep-processing count
- **book** — a build-validated knowledge bundle under `Resources/books/`; queried using explicit exact tags

When `page get` or `page new` returns context, `@file` and `@sql` references are already resolved to actual values. When `page set` writes context, it stores raw JSON with markers — resolution happens at read time.
