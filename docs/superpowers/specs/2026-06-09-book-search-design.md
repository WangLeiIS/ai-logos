# Book Search Design

## Goal

Add a versioned Book Bundle format and a CLI that returns relevant original book chunks for explicit agent-provided tags. Logos performs storage validation and deterministic retrieval only; the calling agent extracts tags and answers the user's question.

## Responsibilities

- `manifest.json` is the source of truth for book metadata.
- SQLite stores only registered book metadata for listing, filtering, and locating bundles.
- Parquet files stored under `Resources/books/` contain chunks and retrieval indexes.
- Logos validates bundles, performs exact tag matching, calculates scores, and returns original chunks.
- Logos does not tokenize questions, generate tags, or generate answers.

## Book Bundle v1

Each direct child directory of `Resources/books/` is a Book Bundle:

```text
Resources/books/<book-id>/
├── manifest.json
├── chunks.parquet
├── inverted_index.parquet
├── idf_stats.parquet
├── source.md
└── images/
```

`source.md` and `images/` are optional supporting resources. The manifest and three Parquet files are required.

### Manifest

```json
{
  "format": "logos-book",
  "format_version": 1,
  "book_id": "aluminum-welding",
  "title": "6061-T4 铝合金激光焊接接头组织与力学性能研究",
  "description": "研究激光焊接接头的微观组织与力学性能",
  "authors": ["王淼"],
  "language": "zh-CN",
  "tags": ["铝合金", "激光焊接"],
  "chunk_count": 6,
  "search_engine": "keyword-coverage-idf-v1"
}
```

Required validation:

- `format` equals `logos-book`.
- `format_version` equals `1`.
- `book_id` is a valid single directory name and equals the bundle directory name.
- `title` is not empty.
- `description`, `language`, authors, and tags may be empty.
- `chunk_count` is non-negative and equals the number of rows in `chunks.parquet`.
- `search_engine` equals `keyword-coverage-idf-v1`.

### chunks.parquet

One row represents one original chunk returned to agents.

| Column | Parquet type | Constraint |
|---|---|---|
| `chunk_id` | string | Required, non-empty, unique within the book |
| `book_id` | string | Required; must equal manifest `book_id` |
| `chunk_type` | string | Required; chunk classification |
| `content` | string | Required, non-empty |
| `questions` | list<string> | Required; questions answered by the chunk |
| `title_keywords` | list<string> | Required; title-level retrieval keywords |
| `content_keywords` | list<string> | Required; content-level retrieval keywords |
| `quote_keywords` | list<string> | Required; conceptual or quoted retrieval keywords |
| `seq_num` | int32 | Required, non-negative sequence number |
| `source_file` | string | Required; source metadata |
| `start_line` | int32 | Required, non-negative |
| `end_line` | int32 | Required, no less than `start_line` |
| `char_count` | int32 | Required, non-negative |
| `summary` | string | Required; short chunk summary |

### inverted_index.parquet

One row represents one exact keyword-to-chunk-field relationship.

| Column | Parquet type | Constraint |
|---|---|---|
| `id` | string | Required, non-empty, unique |
| `chunk_id` | string | Required; must exist in `chunks.parquet` |
| `keyword` | string | Required, non-empty; normalized at validation and query time |
| `field_type` | string | Required; one of `title`, `content`, and `quote` |

Duplicate normalized `(keyword, chunk_id, field_type)` rows are invalid. Keyword frequency does not affect scoring.

### idf_stats.parquet

One row represents one keyword's IDF statistics.

| Column | Parquet type | Constraint |
|---|---|---|
| `keyword` | string | Required, non-empty, unique after normalization |
| `df` | int32 | Required, non-negative, no greater than `chunk_count` |
| `idf` | double | Required, finite, non-negative |

Every keyword in the inverted index must have one IDF row. Extra IDF rows are allowed.

## Keyword Matching

Query tags are supplied by the calling agent. Matching is exact after normalization:

- Trim leading and trailing whitespace.
- Convert English letters to lowercase.
- Preserve Chinese text.
- Reject tags that become empty.
- Deduplicate query tags after normalization.

Logos normalizes both query tags and stored keywords using the same rules before matching. Logos does not perform tokenization, substring matching, fuzzy matching, or synonym expansion.

## SQLite Registration

The `book` table stores metadata and bundle locations only:

```sql
CREATE TABLE IF NOT EXISTS book (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    resource_path TEXT NOT NULL,
    format_version INTEGER NOT NULL,
    authors TEXT NOT NULL DEFAULT '[]',
    language TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

`authors` and `tags` are JSON arrays. `resource_path` is relative to the iroll root, for example `Resources/books/aluminum-welding`.

## Build-Time Discovery and Synchronization

After all Layerfile instructions have run and before the layer hash and history entry are generated:

1. Find direct child directories under `Resources/books/`.
2. Fully validate every discovered Book Bundle.
3. Ensure the `book` table exists.
4. Upsert metadata from each valid manifest by `book_id`.
5. Delete `book` rows whose bundles no longer exist.
6. Fail the entire build if any bundle is invalid.

The scan, validation, upserts, and deletions occur before the build is copied into the iroll store. Invalid books never enter a built iroll. No standalone `book sync` command is included in v1.

## Query CLI

Commands:

```bash
logos book list [name] [--cwd .]

logos book inspect <book-id> [name] [--cwd .]

logos book query \
  --book aluminum-welding \
  --book material-science \
  --tag 激光焊接 \
  --tag 接头强度 \
  --limit 10 \
  --per-book-limit 5 \
  --cwd .
```

`book query` requirements:

- At least one `--book` is required.
- At least one non-empty `--tag` is required.
- Repeated `--book` and `--tag` values are deduplicated.
- The current active iroll is used through `--cwd`.
- Every requested book must be registered and pass a fast query-time validation.
- Any missing or invalid requested book fails the entire query.
- Books are searched independently and may be searched concurrently.
- Each book contributes at most `--per-book-limit` results.
- Merged results are sorted by raw score descending and truncated to `--limit`.
- Stable ties are resolved by `book_id`, `position`, then `chunk_id`.

Fast query-time validation checks the manifest identity/version, required files, and expected Parquet schemas. Cross-file integrity is guaranteed by build-time validation and is not repeated for every query.

Parquet schema compatibility is based on required columns by name and compatible physical type. Files may contain additional columns, use logical annotations, and order columns differently.

## Scoring

For each candidate chunk:

```text
TitleCoverage   = unique query tags matching title   / total unique query tags
ContentCoverage = unique query tags matching content / total unique query tags
QuoteCoverage   = unique query tags matching quote   / total unique query tags
AvgIDF          = mean IDF of all unique query tags matching any field

Score = (
  TitleCoverage × 10 +
  ContentCoverage × 5 +
  QuoteCoverage × 1
) × AvgIDF
```

Rules:

- A tag may contribute to multiple coverage values when it matches multiple fields.
- A matched tag contributes to `AvgIDF` once, regardless of how many fields it matches.
- Keyword frequency does not affect scoring.
- Chunks with no matched tags are not returned.
- Scores from different books are merged without normalization in v1.

## Query Result

The CLI returns original chunk content and explainable scoring details:

```json
{
  "query_tags": ["激光焊接", "接头强度"],
  "books": ["aluminum-welding"],
  "results": [
    {
      "book_id": "aluminum-welding",
      "chunk_id": "aluminum-welding-004",
      "title": "铝合金激光焊接接头硬度呈 V 型分布，焊缝软化导致强度塑性降低。",
      "content": "原文片段……",
      "source_path": "output\\books\\aluminum-welding\\source.md",
      "position": 4,
      "score": 8.42,
      "title_coverage": 0.5,
      "content_coverage": 1.0,
      "quote_coverage": 0.0,
      "avg_idf": 0.81,
      "matched_keywords": ["激光焊接", "接头强度"]
    }
  ]
}
```

## Error Handling

Errors are explicit JSON errors following existing CLI behavior. The complete query fails when:

- No books or tags are supplied.
- A requested book is missing or unregistered.
- A bundle path escapes the iroll root.
- Manifest identity or format is invalid.
- A required Parquet file is missing or has an incompatible schema.
- A Parquet file cannot be read.

## Testing

Tests cover:

- Manifest parsing and validation.
- All required Parquet schemas and cross-file integrity.
- Build-time discovery, upsert, update, and deletion.
- Invalid bundles causing build failure.
- Exact normalized keyword matching.
- Coverage and IDF scoring, including multi-field matches and no frequency effects.
- Multi-book merge order, limits, and all-or-nothing failure.
- CLI argument validation and JSON output.
