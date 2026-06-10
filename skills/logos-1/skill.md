---
name: logos-1
description: |
  Use the logos CLI tool to manage AI agent state, knowledge, iroll packages, and pages.
  Invoke this skill whenever you need to persist conversation context, load an agent's personality,
  create a new memory page, or interact with .iroll packages. This includes: initializing the logos system,
  building or loading iroll packages, creating/switching/deleting pages, reading page context to determine
  how to behave, updating context, saving memories, and querying registered books. Use this skill for
  ANY task involving logos, iroll, page management, agent memory, knowledge, or persistent context.
---

# Logos — AI Agent State and Knowledge Management

Logos is a CLI tool for persistent agent state and knowledge. It stores personality, context, memories, and registered knowledge resources in `.iroll` packages. Each conversation session uses a "page" — a record that holds behavioral context and links to the iroll.

The core workflow: **load an iroll → create a page → read its context → follow those instructions → save memories as you go.**

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
# Build from a Layerfile
logos roll build -f Layerfile -t my-agent

# Or load an existing .iroll file
logos roll load ./my-agent.iroll
```

If multiple iroll packages exist and the user hasn't specified which one to use, ask the user to choose before proceeding.

### Step 3: Create a page

```bash
logos page new <iroll-name> --cwd .
```

This creates a new page and automatically sets it as the active page for the current working directory. The new page inherits its initial context from the template page (page_id=0), which typically contains a `system_prompt` defining the agent's personality and behavior.

The returned JSON includes `page_id` — you can reference it later if needed.

### Step 4: Read context and follow it

```bash
logos page get-context --cwd .
```

The `context` field is a JSON string. Parse it and follow the instructions inside. This is your behavioral blueprint — it tells you how to respond, what persona to adopt, what rules to follow.

**The context is your directive. Read it once, then execute accordingly for the rest of the conversation.**

## During Conversation

### Save important information as memories

When the user shares preferences, facts, or instructions worth remembering:

```bash
logos page add-memory --content "用户喜欢简洁的回复风格" --importance 0.8 --cwd .
```

Importance ranges from 0.0 to 1.0. Use higher values (0.7-1.0) for critical information, lower values (0.1-0.5) for minor details.

### Update page context

When behavioral instructions change during a conversation:

```bash
logos page update-context --content '{"system_prompt":"新的指令"}' --cwd .
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

### Choose and record autonomous loop work

Resolved page context includes `loop.focus` for the current page and `loop.available` seeds. Logos never executes loop work; you choose a seed, perform the work yourself, and use commands to record progress.

```bash
logos loop current --cwd .
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
| `logos roll build -f <file> -t <name>` | Build iroll from Layerfile |
| `logos roll load <file>` | Load .iroll file into ~/.iroll/ |
| `logos roll rm <name>` | Remove an iroll package |
| `logos roll save <name> [-o path]` | Export iroll to .iroll file |
| `logos roll inspect <name>` | Show iroll details |
| `logos roll history <name>` | Show build history |
| `logos page new <name> [--cwd .]` | Create new page |
| `logos page current [--cwd .]` | Show active page |
| `logos page list [name] [--cwd .]` | List pages |
| `logos page switch <page-id>` | Switch active page |
| `logos page delete <page-id>` | Delete a page |
| `logos page get-context [name] [--page <id>] [--cwd .]` | Get page context |
| `logos page update-context [name] --content <json> [--page <id>] [--cwd .]` | Update page context |
| `logos page add-memory [name] --content <text> [--importance 0.5] [--cwd .]` | Add a memory |
| `logos page query-dna <name> [--type <type>] [--cwd .]` | Fuzzy search dna by name |
| `logos book list [name] [--cwd .]` | List registered books |
| `logos book inspect <book-id> [name] [--cwd .]` | Inspect registered book metadata |
| `logos book query --book <id>... --tag <tag>... [--limit 10] [--per-book-limit 5] [--cwd .]` | Retrieve original chunks by exact tags |
| `logos loop list [--archived] [--cwd .]` | List reusable loop seeds |
| `logos loop run <name> [--parent <id>] [--plan <value>] [--cwd .]` | Record an autonomous run choice |
| `logos loop update [run-id] [--plan <value>] [--progress <value>] [--cwd .]` | Replace supplied active-run fields |
| `logos loop complete|abort|reflect|current|history|show ...` | Manage run lifecycle and life records |

## Key Concepts

- **iroll** — a package containing an agent's complete state (database + resources), stored in `~/.iroll/<name>/`
- **page** — a conversation session record with context (behavioral JSON) and a link to the memory store
- **context** — a JSON object stored in a page. Keys are free-form. Values support three types:
  - Plain string: `"system_prompt": "你是一个助手"`
  - File reference: `"greeting": {"@file": "Resources/greeting.txt"}` — reads file content from iroll package
  - SQL query: `"description": {"@sql": "SELECT value FROM metadata WHERE key = 'description'"}` — queries ai_roll.db
- **template page** — page_id=0 stores the default context; new pages inherit from it
- **active page** — each working directory tracks its own active page via system.db
- **dna** — decision-making Q&A pairs defining agent behavior. Context loads questions only (no answers); use `query-dna` to retrieve full records on demand
- **loop seed** — reusable behavioral intention the agent may choose; it has no global execution status
- **loop run** — page-scoped plan, progress, result, and reflection record. Logos records it but never executes the work
- **memory** — timestamped records with importance scores, stored per iroll
- **book** — a build-validated knowledge bundle under `Resources/books/`; queried using explicit exact tags

When `get-context` or `page new` returns context, `@file` and `@sql` references are already resolved to actual values. When `update-context` writes context, it stores raw JSON with markers — resolution happens at read time.
