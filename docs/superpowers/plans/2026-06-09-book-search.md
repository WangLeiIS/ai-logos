# Book Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add build-validated Parquet Book Bundles, SQLite metadata registration, and explicit multi-book tag retrieval.

**Architecture:** Create a focused `book` package that owns bundle manifests, Parquet schemas, validation, tag normalization, and retrieval. Keep SQLite registration in `db/book.go`; call synchronization from the builder and expose read/query operations through a new top-level `logos book` command.

**Tech Stack:** Go 1.24, Cobra, SQLite, `github.com/parquet-go/parquet-go`

---

## File Structure

- Create `iroll/book/types.go`: public manifest, Parquet row, metadata, query, and result types.
- Create `iroll/book/manifest.go`: manifest loading and validation.
- Create `iroll/book/parquet.go`: typed Parquet readers and fast schema validation.
- Create `iroll/book/validate.go`: full bundle discovery and cross-file validation.
- Create `iroll/book/query.go`: tag normalization, scoring, one-book search, and multi-book merge.
- Create `iroll/book/testutil_test.go`: typed Parquet Book Bundle fixtures used by package tests.
- Create `iroll/db/book.go`: `book` table creation, synchronization, list, and inspect queries.
- Modify `iroll/builder/build.go`: validate and synchronize books before layer hashing.
- Create `iroll/cmd/book.go`: top-level `book list`, `book inspect`, and `book query`.
- Modify `iroll/skills/logos-1/skill.md` and `README.md`: document agent-facing query workflow.

## Task 1: Add Parquet Dependency and Core Types

**Files:**
- Modify: `iroll/go.mod`
- Modify: `iroll/go.sum`
- Create: `iroll/book/types.go`
- Create: `iroll/book/normalize_test.go`
- Create: `iroll/book/normalize.go`

- [ ] **Step 1: Add the Parquet dependency**

Run:

```powershell
go get github.com/parquet-go/parquet-go
```

Expected: `go.mod` and `go.sum` include `github.com/parquet-go/parquet-go`.

- [ ] **Step 2: Define the public types**

Create these core shapes in `book/types.go`:

```go
type Manifest struct {
	Format        string   `json:"format"`
	FormatVersion int      `json:"format_version"`
	BookID        string   `json:"book_id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Authors       []string `json:"authors"`
	Language      string   `json:"language"`
	Tags          []string `json:"tags"`
	ChunkCount    int64    `json:"chunk_count"`
	SearchEngine  string   `json:"search_engine"`
}

type ChunkRow struct {
	ChunkID    string `parquet:"chunk_id"`
	Title      string `parquet:"title"`
	Content    string `parquet:"content"`
	SourcePath string `parquet:"source_path"`
	Position   int64  `parquet:"position"`
}

type IndexRow struct {
	Keyword string   `parquet:"keyword"`
	ChunkID string   `parquet:"chunk_id"`
	Fields  []string `parquet:"fields,list"`
}

type IDFRow struct {
	Keyword           string  `parquet:"keyword"`
	IDF               float64 `parquet:"idf"`
	DocumentFrequency int64   `parquet:"document_frequency"`
}
```

Also define `Bundle`, `Book`, `Query`, `Result`, and `QueryResponse` matching the approved design.

- [ ] **Step 3: Write failing normalization tests**

Cover:

```go
func TestNormalizeTagsTrimsLowercasesDeduplicates(t *testing.T)
func TestNormalizeTagsRejectsEmptyResult(t *testing.T)
```

Example expectation:

```go
got, err := NormalizeTags([]string{" 激光焊接 ", "LASER", "laser"})
want := []string{"激光焊接", "laser"}
```

- [ ] **Step 4: Verify the tests fail**

Run:

```powershell
go test ./book -run Normalize -v
```

Expected: FAIL because `NormalizeTags` does not exist.

- [ ] **Step 5: Implement normalization**

Implement:

```go
func NormalizeKeyword(value string) string
func NormalizeTags(values []string) ([]string, error)
```

Use `strings.TrimSpace` and `strings.ToLower`, preserve first-seen ordering, and reject an empty normalized result.

- [ ] **Step 6: Verify the tests pass**

Run:

```powershell
go test ./book -run Normalize -v
```

Expected: PASS.

## Task 2: Manifest Loading and Validation

**Files:**
- Create: `iroll/book/manifest.go`
- Create: `iroll/book/manifest_test.go`

- [ ] **Step 1: Write failing manifest tests**

Cover:

```go
func TestLoadManifestAcceptsValidV1(t *testing.T)
func TestLoadManifestRejectsMismatchedDirectoryID(t *testing.T)
func TestLoadManifestRejectsUnsupportedFormat(t *testing.T)
func TestLoadManifestRejectsUnsafeBookID(t *testing.T)
func TestLoadManifestRejectsEmptyTitle(t *testing.T)
```

- [ ] **Step 2: Verify the tests fail**

Run:

```powershell
go test ./book -run Manifest -v
```

Expected: FAIL because manifest loading is not implemented.

- [ ] **Step 3: Implement manifest loading**

Expose:

```go
func LoadManifest(bundleDir string) (*Manifest, error)
func ValidateManifest(manifest *Manifest, directoryName string) error
```

Read `manifest.json` through `safepath.Join`, reject unknown format/version/search engine, validate the ID with `safepath.ValidateName`, and require the directory name to equal `book_id`.

- [ ] **Step 4: Verify the tests pass**

Run:

```powershell
go test ./book -run Manifest -v
```

Expected: PASS.

## Task 3: Typed Parquet Readers and Bundle Fixtures

**Files:**
- Create: `iroll/book/parquet.go`
- Create: `iroll/book/parquet_test.go`
- Create: `iroll/book/testutil_test.go`

- [ ] **Step 1: Add a test fixture writer**

Use `parquet.WriteFile` or `parquet.NewGenericWriter[T]` in `testutil_test.go` to create:

```go
func writeBundleFixture(t *testing.T, root string, manifest Manifest, chunks []ChunkRow, index []IndexRow, idf []IDFRow) string
```

- [ ] **Step 2: Write failing typed-reader tests**

Cover successful reads and missing/incompatible files:

```go
func TestReadChunks(t *testing.T)
func TestReadIndex(t *testing.T)
func TestReadIDF(t *testing.T)
func TestFastValidateRejectsMissingParquet(t *testing.T)
func TestFastValidateRejectsWrongSchema(t *testing.T)
```

- [ ] **Step 3: Verify the tests fail**

Run:

```powershell
go test ./book -run "Read|FastValidate" -v
```

Expected: FAIL because Parquet readers do not exist.

- [ ] **Step 4: Implement typed readers and fast validation**

Expose:

```go
func ReadChunks(bundleDir string) ([]ChunkRow, error)
func ReadIndex(bundleDir string) ([]IndexRow, error)
func ReadIDF(bundleDir string) ([]IDFRow, error)
func FastValidate(bundleDir string) (*Manifest, error)
```

Open all paths through `safepath.Join`. Fast validation loads the manifest, confirms required files are readable, and verifies schemas by opening typed readers.

- [ ] **Step 5: Verify the tests pass**

Run:

```powershell
go test ./book -run "Read|FastValidate" -v
```

Expected: PASS.

## Task 4: Full Bundle Validation and Discovery

**Files:**
- Create: `iroll/book/validate.go`
- Create: `iroll/book/validate_test.go`

- [ ] **Step 1: Write failing full-validation tests**

Cover:

```go
func TestValidateBundleAcceptsValidBundle(t *testing.T)
func TestValidateBundleRejectsChunkCountMismatch(t *testing.T)
func TestValidateBundleRejectsDuplicateChunkID(t *testing.T)
func TestValidateBundleRejectsUnsafeSourcePath(t *testing.T)
func TestValidateBundleRejectsUnknownIndexChunk(t *testing.T)
func TestValidateBundleRejectsDuplicateIndexPair(t *testing.T)
func TestValidateBundleRejectsInvalidField(t *testing.T)
func TestValidateBundleRejectsMissingIDF(t *testing.T)
func TestValidateBundleRejectsInvalidIDF(t *testing.T)
func TestDiscoverRejectsInvalidChildBundle(t *testing.T)
```

- [ ] **Step 2: Verify the tests fail**

Run:

```powershell
go test ./book -run "ValidateBundle|Discover" -v
```

Expected: FAIL because full validation and discovery do not exist.

- [ ] **Step 3: Implement full validation and discovery**

Expose:

```go
func ValidateBundle(bundleDir string) (*Bundle, error)
func Discover(rollRoot string) ([]Bundle, error)
```

`ValidateBundle` performs all manifest, schema, row, and cross-file checks from the spec. `Discover` scans only direct child directories under `Resources/books`, sorts them by directory name, and fails on the first invalid bundle.

- [ ] **Step 4: Verify the tests pass**

Run:

```powershell
go test ./book -run "ValidateBundle|Discover" -v
```

Expected: PASS.

## Task 5: SQLite Book Metadata Synchronization

**Files:**
- Create: `iroll/db/book.go`
- Create: `iroll/db/book_test.go`

- [ ] **Step 1: Write failing database tests**

Cover:

```go
func TestSyncBooksCreatesAndInserts(t *testing.T)
func TestSyncBooksUpdatesManifestMetadata(t *testing.T)
func TestSyncBooksDeletesMissingBooks(t *testing.T)
func TestListBooksReturnsStableOrder(t *testing.T)
func TestGetBookReturnsRegisteredBook(t *testing.T)
```

- [ ] **Step 2: Verify the tests fail**

Run:

```powershell
go test ./db -run Book -v
```

Expected: FAIL because book registration functions do not exist.

- [ ] **Step 3: Implement transactional metadata synchronization**

Expose:

```go
func EnsureBookTable(conn *sql.DB) error
func SyncBooks(conn *sql.DB, bundles []book.Bundle) error
func ListBooks(conn *sql.DB) ([]book.Book, error)
func GetBook(conn *sql.DB, bookID string) (*book.Book, error)
```

Serialize authors and tags as JSON arrays. Execute table creation, upserts, and deletion of absent books in one transaction. Preserve `created_at` during updates and refresh `updated_at`.

- [ ] **Step 4: Verify the tests pass**

Run:

```powershell
go test ./db -run Book -v
```

Expected: PASS.

## Task 6: Integrate Book Validation Into Builds

**Files:**
- Modify: `iroll/builder/build.go`
- Modify: `iroll/builder/build_test.go`

- [ ] **Step 1: Write failing builder integration tests**

Cover:

```go
func TestBuildRegistersValidBooks(t *testing.T)
func TestBuildFailsForInvalidBookBundle(t *testing.T)
func TestBuildRemovesInheritedBookRegistrationWhenResourceIsRemoved(t *testing.T)
```

- [ ] **Step 2: Verify the tests fail**

Run:

```powershell
go test ./builder -run Book -v
```

Expected: FAIL because builds do not discover or synchronize books.

- [ ] **Step 3: Integrate discovery and synchronization**

After Layerfile instructions and database creation, before layer hashing:

```go
bundles, err := book.Discover(tmpDir)
if err != nil {
	return nil, fmt.Errorf("validate books: %w", err)
}
if err := db.SyncBooks(conn, bundles); err != nil {
	return nil, fmt.Errorf("sync books: %w", err)
}
```

Open the database once for book synchronization and history insertion. Ensure validation failures happen before copying the build into `~/.iroll`.

- [ ] **Step 4: Verify the tests pass**

Run:

```powershell
go test ./builder -run Book -v
```

Expected: PASS.

## Task 7: Query Scoring and Multi-Book Merge

**Files:**
- Create: `iroll/book/query.go`
- Create: `iroll/book/query_test.go`

- [ ] **Step 1: Write failing scoring tests**

Cover:

```go
func TestQueryScoresCoverageTimesAverageIDF(t *testing.T)
func TestQueryCountsOneTagAcrossMultipleFields(t *testing.T)
func TestQueryDoesNotCountKeywordFrequency(t *testing.T)
func TestQueryRequiresExactNormalizedMatch(t *testing.T)
func TestQueryOmitsChunksWithoutMatches(t *testing.T)
func TestQueryAppliesPerBookAndGlobalLimits(t *testing.T)
func TestQueryUsesStableTieBreakOrder(t *testing.T)
func TestQueryFailsEntirelyWhenOneBookIsInvalid(t *testing.T)
```

Use an explicit scoring assertion:

```go
want := (0.5*10 + 1.0*5 + 0.5*1) * ((2.0 + 4.0) / 2.0)
```

- [ ] **Step 2: Verify the tests fail**

Run:

```powershell
go test ./book -run Query -v
```

Expected: FAIL because query functions do not exist.

- [ ] **Step 3: Implement one-book query**

Implement an internal:

```go
func queryBundle(bundleDir string, bookID string, tags []string, limit int) ([]Result, error)
```

Read the index and IDF rows, build candidate chunk score state using boolean sets per field, read only returned chunk records into final results, and sort by score descending then position and chunk ID.

- [ ] **Step 4: Implement multi-book query**

Expose:

```go
func QueryBooks(rollRoot string, registered []Book, query Query) (*QueryResponse, error)
```

Validate every requested book before starting searches. Search valid books concurrently with `sync.WaitGroup` or `errgroup`, merge results, apply stable global ordering, and truncate to the global limit.

- [ ] **Step 5: Verify the tests pass**

Run:

```powershell
go test ./book -run Query -v
```

Expected: PASS.

## Task 8: Book CLI

**Files:**
- Create: `iroll/cmd/book.go`
- Create: `iroll/cmd/book_test.go`
- Modify: `iroll/cmd/root.go` only if a reusable active-iroll resolver is extracted.

- [ ] **Step 1: Write failing command tests**

Test command handlers through functions that return errors rather than invoking `outputError`:

```go
func TestBookListUsesActiveIroll(t *testing.T)
func TestBookInspectRejectsUnknownBook(t *testing.T)
func TestBookQueryRequiresBookAndTag(t *testing.T)
func TestBookQueryReturnsScoringDetails(t *testing.T)
```

- [ ] **Step 2: Verify the tests fail**

Run:

```powershell
go test ./cmd -run Book -v
```

Expected: FAIL because the book command does not exist.

- [ ] **Step 3: Implement top-level commands**

Register:

```text
logos book list [name] [--cwd .]
logos book inspect <book-id> [name] [--cwd .]
logos book query --book <id>... --tag <tag>... [--limit 10] [--per-book-limit 5] [--cwd .]
```

Use repeated Cobra string-array flags for books and tags. Resolve the active iroll when no explicit roll name is supplied. Validate positive limits and return JSON matching the approved result schema.

- [ ] **Step 4: Verify the tests pass**

Run:

```powershell
go test ./cmd -run Book -v
```

Expected: PASS.

## Task 9: Example Bundle and Documentation

**Files:**
- Modify: `examples/base-agent/Layerfile`
- Modify: `examples/base-agent/init_schema.sql`
- Create: `examples/base-agent/books/<book-id>/manifest.json`
- Modify or regenerate: the three example Parquet files to conform to Book Bundle v1
- Modify: `README.md`
- Modify: `skills/logos-1/skill.md`

- [ ] **Step 1: Add the example book table and bundle copy**

Add `book` table SQL to the base schema and copy the books directory:

```dockerfile
COPY books Resources/books
```

- [ ] **Step 2: Convert the existing example book**

Add a v1 manifest and regenerate `chunks.parquet`, `inverted_index.parquet`, and `idf_stats.parquet` with the approved schemas. Keep original Markdown and images as supporting resources.

- [ ] **Step 3: Document agent usage**

Document:

```bash
logos book list --cwd .
logos book query --book aluminum-welding --tag 激光焊接 --tag 接头强度 --cwd .
```

State explicitly that the agent extracts tags and Logos only returns original chunks.

## Task 10: Full Verification

**Files:**
- Modify only files required by failing verification.

- [ ] **Step 1: Format**

Run:

```powershell
gofmt -w book db/book.go builder/build.go cmd/book.go
```

- [ ] **Step 2: Run the complete tests**

Run:

```powershell
go test ./...
```

Expected: all packages PASS.

- [ ] **Step 3: Run static checks and build**

Run:

```powershell
go vet ./...
go build ./...
```

Expected: both commands exit successfully.

- [ ] **Step 4: Exercise the real CLI**

Build an example roll under an isolated home and run:

```powershell
logos roll build -f ../examples/base-agent/Layerfile -t book-test
logos page new book-test --cwd .
logos book list --cwd .
logos book inspect aluminum-welding --cwd .
logos book query --book aluminum-welding --tag 激光焊接 --tag 接头强度 --cwd .
```

Expected: the roll builds, the book is registered, and query results contain original chunks with scoring details.

- [ ] **Step 5: Review the diff**

Confirm no unrelated user changes were modified, especially `docs/rebot-roll.md` and existing untracked book resources outside the intentional v1 conversion.

