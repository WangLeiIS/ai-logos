# Page Context CRUD & Data Access Layering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Promote `context` to a first-class page citizen with per-key CRUD (`page get/set/unset/alias` replacing `get-context`/`update-context`), and split the SQL escape hatches by data ownership (`roll evolving` → roll-level inner+template; new `page query` → page-level cwd outer).

**Architecture:** Two layers of change. (1) db layer: add JSON-path navigation + per-key context CRUD on top of the existing raw-context read-modify-write. (2) cmd layer: rewrite the context commands, add `page query`, and refactor `evolving` to open the roll-level template instead of the workspace-default live outer. The SQL execution engine (`db.ExecuteAll`) is reused by both escape hatches.

**Tech Stack:** Go 1.24, Cobra CLI, SQLite (go-sqlite3, CGO), encoding/json.

**Spec:** `docs/superpowers/specs/2026-06-26-page-context-and-query-design.md`

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `iroll/db/context_keys.go` | JSON-path navigation + per-key context CRUD (`Get/Set/UnsetContextKey`, `ErrContextKeyNotFound`) | Create |
| `iroll/db/context_keys_test.go` | Unit tests for navigation + CRUD | Create |
| `iroll/cmd/root.go` | Add `ErrCodeKeyNotFound` constant | Modify |
| `iroll/cmd/context.go` | Remove `get-context`/`update-context`; add `page get/set/unset/alias` + shared `pageTarget` flags + `pageBrief` helper | Rewrite |
| `iroll/cmd/sql_input.go` | Shared `resolveSQLInput` helper (flag/positional/file/stdin) | Create |
| `iroll/cmd/evolving.go` | Refactor `runEvolving` to open template outer; slim `resolveEvolvingSQL` to wrapper | Modify |
| `iroll/cmd/page_query.go` | New `page query` command (standalone cwd-outer open + ExecuteAll) | Create |
| `iroll/cmd/page_query_test.go` | Integration test for `page query` | Create |
| `iroll/cmd/loop_run.go`, `memory.go`, `query_dna.go`, `page.go` | Mechanical hint string updates | Modify |
| `skills/logos-1/skill.md`, `README.md`, `docs/rebot-roll.md`, `iroll/db/db.go` | Doc updates | Modify |

**Key design decisions (from spec + exploration):**
- `page get <path>` runs the **full** `ResolveContext` (resolves `@file`/`@sql` + injects `loop_focus`/`loop_available`) then navigates — so `loop_focus`, `user_context.project`, `system_prompt` are all reachable. (The actual injected keys are `loop_focus`/`loop_available`, flat top-level — see `db.go:270,282`.)
- `page set`/`unset` operate on **raw** context (read-modify-write); `@file`/`@sql` markers survive as nested maps.
- `page query` opens the cwd outer.db **standalone** (`db.Open`), no inner attach. This is simpler and Windows-robust than read-only attach, and enforces an even stricter boundary: `page query` sees only outer tables (`pages`/`memory`/`loop_runs`). Inner data (`dna`/loop seeds) stays reachable via `query-dna` / `evolving`. (Refines spec §2: ro-attach replaced by no-attach.)
- `evolving` opens `OpenOuter(<irollPath>/roll-outer.db, innerPath)` — template as main + inner attached, both read-write.

---

## Task 1: db layer — context key navigation + CRUD

**Files:**
- Create: `iroll/db/context_keys.go`
- Test: `iroll/db/context_keys_test.go`

- [ ] **Step 1: Write the failing tests**

Create `iroll/db/context_keys_test.go`:

```go
package db

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNavigateGet(t *testing.T) {
	m := map[string]interface{}{
		"user_context": map[string]interface{}{"project": "blog"},
		"plain":        "hi",
	}
	if v, ok := navigateGet(m, "user_context.project"); !ok || v != "blog" {
		t.Fatalf("navigateGet nested = (%v, %v), want (blog, true)", v, ok)
	}
	if v, ok := navigateGet(m, "plain"); !ok || v != "hi" {
		t.Fatalf("navigateGet top = (%v, %v), want (hi, true)", v, ok)
	}
	if _, ok := navigateGet(m, "user_context.missing"); ok {
		t.Fatal("navigateGet missing key should return ok=false")
	}
	if _, ok := navigateGet(m, "plain.sub"); ok {
		t.Fatal("navigateGet into non-map should return ok=false")
	}
}

func TestNavigateSet(t *testing.T) {
	m := map[string]interface{}{}
	navigateSet(m, "user_context.project", "blog")
	if v, _ := navigateGet(m, "user_context.project"); v != "blog" {
		t.Fatalf("after set nested, got %v", v)
	}
	// overwrite existing
	navigateSet(m, "user_context.project", "blog-v2")
	if v, _ := navigateGet(m, "user_context.project"); v != "blog-v2" {
		t.Fatalf("overwrite failed, got %v", v)
	}
	// auto-create intermediate
	navigateSet(m, "a.b.c", 1)
	if v, _ := navigateGet(m, "a.b.c"); v != 1 {
		t.Fatalf("auto-create intermediate failed, got %v", v)
	}
}

func TestNavigateUnset(t *testing.T) {
	m := map[string]interface{}{
		"user_context": map[string]interface{}{"project": "blog", "todo": "x"},
	}
	if !navigateUnset(m, "user_context.todo") {
		t.Fatal("unset existing should return true")
	}
	if _, ok := navigateGet(m, "user_context.todo"); ok {
		t.Fatal("key still present after unset")
	}
	// sibling preserved
	if v, _ := navigateGet(m, "user_context.project"); v != "blog" {
		t.Fatalf("sibling lost after unset, got %v", v)
	}
	if navigateUnset(m, "user_context.missing") {
		t.Fatal("unset missing should return false")
	}
}

func TestParseJSONOrText(t *testing.T) {
	cases := []struct {
		in   string
		want interface{}
	}{
		{"blog", "blog"},
		{"true", true},
		{"42", float64(42)},
		{`["a","b"]`, []interface{}{"a", "b"}},
		{`{"k":"v"}`, map[string]interface{}{"k": "v"}},
		{"not json at all", "not json at all"},
	}
	for _, c := range cases {
		got, err := parseJSONOrText(c.in)
		if err != nil {
			t.Fatalf("parseJSONOrText(%q) err: %v", c.in, err)
		}
		if !deepEqual(got, c.want) {
			t.Fatalf("parseJSONOrText(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func deepEqual(a, b interface{}) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

// setupContextTestDB builds an outer+inner connection (schema only) and inserts
// one working page with a known raw context.
func setupContextTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	innerPath, outerPath := setupDualDB(t, dir)
	conn, err := OpenOuter(outerPath, innerPath)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })

	ctx := `{"system_prompt":"hi","user_context":{"project":"blog","todo":["deploy"]}}`
	if _, err := conn.Exec(
		`INSERT INTO pages (page_id, cwd, context, created_at, updated_at) VALUES ('p1', '', ?, datetime('now'), datetime('now'))`,
		ctx,
	); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestSetContextKey(t *testing.T) {
	conn := setupContextTestDB(t)

	if err := SetContextKey(conn, "p1", "user_context.project", "blog-v2"); err != nil {
		t.Fatalf("SetContextKey: %v", err)
	}
	p, err := GetPageByPageID(conn, "p1")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(p.Context), &got); err != nil {
		t.Fatal(err)
	}
	if v, _ := navigateGet(got, "user_context.project"); v != "blog-v2" {
		t.Fatalf("project not updated, got %v", v)
	}
	// siblings preserved
	if v, _ := navigateGet(got, "system_prompt"); v != "hi" {
		t.Fatalf("system_prompt lost, got %v", v)
	}
	if _, ok := navigateGet(got, "user_context.todo"); !ok {
		t.Fatal("user_context.todo lost")
	}
}

func TestSetContextKeyNewNested(t *testing.T) {
	conn := setupContextTestDB(t)
	if err := SetContextKey(conn, "p1", "user_context.stats.count", "3"); err != nil {
		t.Fatalf("SetContextKey new nested: %v", err)
	}
	p, _ := GetPageByPageID(conn, "p1")
	var got map[string]interface{}
	json.Unmarshal([]byte(p.Context), &got)
	if v, _ := navigateGet(got, "user_context.stats.count"); v != float64(3) {
		t.Fatalf("new nested not created/parsed, got %v", v)
	}
}

func TestSetContextKeyStoresMarkerRaw(t *testing.T) {
	conn := setupContextTestDB(t)
	marker := `{"@sql":"SELECT value FROM inner.metadata WHERE key='name'"}`
	if err := SetContextKey(conn, "p1", "name", marker); err != nil {
		t.Fatalf("SetContextKey marker: %v", err)
	}
	p, _ := GetPageByPageID(conn, "p1")
	var got map[string]interface{}
	json.Unmarshal([]byte(p.Context), &got)
	val, _ := navigateGet(got, "name")
	obj, ok := val.(map[string]interface{})
	if !ok {
		t.Fatalf("marker not stored as object, got %#v", val)
	}
	if obj["@sql"] == nil {
		t.Fatalf("marker @sql lost, got %#v", obj)
	}
}

func TestUnsetContextKey(t *testing.T) {
	conn := setupContextTestDB(t)
	if err := UnsetContextKey(conn, "p1", "user_context.todo"); err != nil {
		t.Fatalf("UnsetContextKey: %v", err)
	}
	p, _ := GetPageByPageID(conn, "p1")
	var got map[string]interface{}
	json.Unmarshal([]byte(p.Context), &got)
	if _, ok := navigateGet(got, "user_context.todo"); ok {
		t.Fatal("key still present after unset")
	}
}

func TestUnsetContextKeyMissing(t *testing.T) {
	conn := setupContextTestDB(t)
	err := UnsetContextKey(conn, "p1", "does.not.exist")
	if !errors.Is(err, ErrContextKeyNotFound) {
		t.Fatalf("expected ErrContextKeyNotFound, got %v", err)
	}
}

func TestGetContextKey(t *testing.T) {
	conn := setupContextTestDB(t)
	// irollPath "" is fine: no @file in this context.
	v, err := GetContextKey(conn, "p1", "user_context.project", "")
	if err != nil {
		t.Fatalf("GetContextKey: %v", err)
	}
	if v != "blog" {
		t.Fatalf("GetContextKey = %v, want blog", v)
	}
}

func TestGetContextKeyMissing(t *testing.T) {
	conn := setupContextTestDB(t)
	_, err := GetContextKey(conn, "p1", "nope.nada", "")
	if !errors.Is(err, ErrContextKeyNotFound) {
		t.Fatalf("expected ErrContextKeyNotFound, got %v", err)
	}
}
```

Note: `setupDualDB` already exists in `loop_seed_test.go` (same package). Add `"database/sql"` to imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd iroll && go test ./db/ -run 'TestNavigate|TestParseJSONOrText|TestSetContextKey|TestUnsetContextKey|TestGetContextKey' -v`
Expected: FAIL / build error — `navigateGet`, `parseJSONOrText`, `SetContextKey`, etc. undefined.

- [ ] **Step 3: Write the implementation**

Create `iroll/db/context_keys.go`:

```go
package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrContextKeyNotFound is returned when a context path does not exist.
var ErrContextKeyNotFound = errors.New("context key not found")

// parseJSONOrText parses s as JSON if it is valid; otherwise returns s as a plain string.
func parseJSONOrText(s string) (interface{}, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v, nil
	}
	return s, nil
}

// navigateGet walks a dot-separated path through nested map[string]interface{} values.
// Returns (value, true) if the full path exists, (nil, false) otherwise.
func navigateGet(m map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var cur interface{} = m
	for _, part := range parts {
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		val, exists := obj[part]
		if !exists {
			return nil, false
		}
		cur = val
	}
	return cur, true
}

// navigateSet walks a dot-separated path, creating intermediate maps as needed,
// and sets the leaf to value.
func navigateSet(m map[string]interface{}, path string, value interface{}) {
	parts := strings.Split(path, ".")
	cur := m
	for i, part := range parts {
		if i == len(parts)-1 {
			cur[part] = value
			return
		}
		next, ok := cur[part].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			cur[part] = next
		}
		cur = next
	}
}

// navigateUnset removes the leaf at path. Returns true if it existed.
func navigateUnset(m map[string]interface{}, path string) bool {
	parts := strings.Split(path, ".")
	cur := m
	for i, part := range parts {
		if i == len(parts)-1 {
			if _, exists := cur[part]; !exists {
				return false
			}
			delete(cur, part)
			return true
		}
		next, ok := cur[part].(map[string]interface{})
		if !ok {
			return false
		}
		cur = next
	}
	return false
}

// GetContextKey resolves the page's full context (ResolveContext, including @file/@sql
// resolution and loop injection) then navigates to path. irollPath is the iroll package
// root directory, used only to resolve @file markers.
func GetContextKey(db *sql.DB, pageID, path, irollPath string) (interface{}, error) {
	p, err := GetPageByPageID(db, pageID)
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveContext(p.Context, irollPath, db, pageID)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &m); err != nil {
		return nil, fmt.Errorf("parse resolved context: %w", err)
	}
	val, found := navigateGet(m, path)
	if !found {
		return nil, fmt.Errorf("context key %q: %w", path, ErrContextKeyNotFound)
	}
	return val, nil
}

// SetContextKey parses rawValue (json-or-text), then reads-modifies-writes the page's
// raw context, setting the leaf at path. @file/@sql markers on other keys are preserved.
func SetContextKey(db *sql.DB, pageID, path, rawValue string) error {
	p, err := GetPageByPageID(db, pageID)
	if err != nil {
		return err
	}
	m := map[string]interface{}{}
	if strings.TrimSpace(p.Context) != "" && p.Context != "null" {
		if err := json.Unmarshal([]byte(p.Context), &m); err != nil {
			return fmt.Errorf("parse page context as JSON: %w", err)
		}
	}
	value, err := parseJSONOrText(rawValue)
	if err != nil {
		return err
	}
	navigateSet(m, path, value)
	out, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}
	_, err = db.Exec("UPDATE pages SET context = ?, updated_at = ? WHERE page_id = ?", string(out), nowISO(), pageID)
	return err
}

// UnsetContextKey reads-modifies-writes the page's raw context, removing the leaf at path.
func UnsetContextKey(db *sql.DB, pageID, path string) error {
	p, err := GetPageByPageID(db, pageID)
	if err != nil {
		return err
	}
	m := map[string]interface{}{}
	if strings.TrimSpace(p.Context) != "" && p.Context != "null" {
		if err := json.Unmarshal([]byte(p.Context), &m); err != nil {
			return fmt.Errorf("parse page context as JSON: %w", err)
		}
	}
	if !navigateUnset(m, path) {
		return fmt.Errorf("context key %q: %w", path, ErrContextKeyNotFound)
	}
	out, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}
	_, err = db.Exec("UPDATE pages SET context = ?, updated_at = ? WHERE page_id = ?", string(out), nowISO(), pageID)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd iroll && go test ./db/ -run 'TestNavigate|TestParseJSONOrText|TestSetContextKey|TestUnsetContextKey|TestGetContextKey' -v`
Expected: PASS (all listed tests).

- [ ] **Step 5: Run full db package tests + commit**

Run: `cd iroll && go test ./db/...`
Expected: PASS (no regressions).

```bash
git add iroll/db/context_keys.go iroll/db/context_keys_test.go
git commit -m "feat(db): add per-key context CRUD with JSON-path navigation"
```

---

## Task 2: cmd — ErrCodeKeyNotFound + rewrite context.go

**Files:**
- Modify: `iroll/cmd/root.go` (add constant)
- Rewrite: `iroll/cmd/context.go`

- [ ] **Step 1: Add ErrCodeKeyNotFound to root.go**

In `iroll/cmd/root.go`, extend the error code const block (currently lines 22-30). Replace:

```go
const (
	ErrCodeInvalidTag    = "invalid_tag"
	ErrCodeIrollNotFound = "iroll_not_found"
	ErrCodeNoDefaultPage = "no_default_page"
	ErrCodePageNotFound  = "page_not_found"
	ErrCodeDBOpen        = "db_open_failed"
	ErrCodeInternal      = "internal"
	ErrCodeNoActivePage  = "no_active_page"
)
```

with:

```go
const (
	ErrCodeInvalidTag    = "invalid_tag"
	ErrCodeIrollNotFound = "iroll_not_found"
	ErrCodeNoDefaultPage = "no_default_page"
	ErrCodePageNotFound  = "page_not_found"
	ErrCodeDBOpen        = "db_open_failed"
	ErrCodeInternal      = "internal"
	ErrCodeNoActivePage  = "no_active_page"
	ErrCodeKeyNotFound   = "key_not_found"
)
```

- [ ] **Step 2: Rewrite context.go**

Replace the entire contents of `iroll/cmd/context.go` with:

```go
package cmd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"logos/builder"
	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

// pageTarget holds the shared page-targeting flags used by the context commands.
type pageTarget struct {
	page, alias, roll, cwd string
}

func (t *pageTarget) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&t.page, "page", "", "Page ID")
	cmd.Flags().StringVar(&t.alias, "alias", "", "Page alias")
	cmd.Flags().StringVar(&t.roll, "roll", "", "iroll name (uses default page)")
	cmd.Flags().StringVar(&t.cwd, "cwd", ".", "Working directory")
}

var (
	pageGetTarget   pageTarget
	pageSetTarget   pageTarget
	pageUnsetTarget pageTarget
	pageAliasTarget pageTarget
)

var pageSetContent string
var pageAliasClear bool

// pageBrief converts a full Page into a lightweight PageBrief (no context),
// encouraging callers to use `page get` when they need the context body.
func pageBrief(p *db.Page) db.PageBrief {
	return db.PageBrief{
		PageID:    p.PageID,
		Cwd:       p.Cwd,
		Alias:     p.Alias,
		CreatedAt: p.CreatedAt,
	}
}

var pageGetCmd = &cobra.Command{
	Use:   "get [path]",
	Short: "Get page context (full, or a single resolved key by dot-path)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageGetTarget.cwd)
		name, version, pageID, conn := resolvePageContext(nil, pageGetTarget.page, pageGetTarget.alias, pageGetTarget.roll, cwd)
		defer conn.Close()

		if len(args) == 0 {
			p, err := db.GetPageByPageID(conn, pageID)
			if err != nil {
				outputFail(ErrCodePageNotFound, err.Error(), nil)
			}
			resolved, err := db.ResolveContext(p.Context, checkedIrollPath(name, version), conn, pageID)
			if err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			outputOK(json.RawMessage(resolved), contextFollowupHints(p))
			return
		}

		val, err := db.GetContextKey(conn, pageID, args[0], checkedIrollPath(name, version))
		if err != nil {
			if errors.Is(err, db.ErrContextKeyNotFound) {
				outputFail(ErrCodeKeyNotFound, err.Error(), nil)
			}
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(val, nil)
	},
}

var pageSetCmd = &cobra.Command{
	Use:   "set [path] [value]",
	Short: "Set a context key (json-or-text value), or replace the whole context with --content",
	Args:  cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageSetTarget.cwd)
		_, _, pageID, conn := resolvePageContext(nil, pageSetTarget.page, pageSetTarget.alias, pageSetTarget.roll, cwd)
		defer conn.Close()

		if cmd.Flags().Changed("content") {
			if len(args) > 0 {
				outputFail(ErrCodeInternal, "--content cannot be combined with path/value arguments", nil)
			}
			p, err := db.UpdatePageContext(conn, pageID, pageSetContent)
			if err != nil {
				outputFail(ErrCodeInternal, err.Error(), nil)
			}
			outputOK(pageBrief(p), contextFollowupHints(p))
			return
		}

		if len(args) != 2 {
			outputFail(ErrCodeInternal, "usage: logos page set <path> <value>  (or: logos page set --content '<json>')", nil)
		}
		if err := db.SetContextKey(conn, pageID, args[0], args[1]); err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(pageBrief(p), contextFollowupHints(p))
	},
}

var pageUnsetCmd = &cobra.Command{
	Use:   "unset <path>",
	Short: "Delete a context key by dot-path",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageUnsetTarget.cwd)
		_, _, pageID, conn := resolvePageContext(nil, pageUnsetTarget.page, pageUnsetTarget.alias, pageUnsetTarget.roll, cwd)
		defer conn.Close()

		if err := db.UnsetContextKey(conn, pageID, args[0]); err != nil {
			if errors.Is(err, db.ErrContextKeyNotFound) {
				outputFail(ErrCodeKeyNotFound, err.Error(), nil)
			}
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(pageBrief(p), contextFollowupHints(p))
	},
}

var pageAliasCmd = &cobra.Command{
	Use:   "alias [name]",
	Short: "Set or clear the page alias",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageAliasTarget.cwd)
		_, _, pageID, conn := resolvePageContext(nil, pageAliasTarget.page, pageAliasTarget.alias, pageAliasTarget.roll, cwd)
		defer conn.Close()

		var alias string
		if pageAliasClear {
			alias = ""
		} else {
			if len(args) != 1 {
				outputFail(ErrCodeInternal, "usage: logos page alias <name>  (or: logos page alias --clear)", nil)
			}
			alias = args[0]
		}
		if err := store.SetPageAlias(pageID, alias); err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		if err := db.UpdatePageAlias(conn, pageID, alias); err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		p, err := db.GetPageByPageID(conn, pageID)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(pageBrief(p), contextFollowupHints(p))
	},
}

// contextFollowupHints suggests common next steps after a context read/write.
func contextFollowupHints(p *db.Page) []Hint {
	hints := []Hint{
		{Action: "Read the full resolved context", Cmd: fmt.Sprintf("logos page get --page %s", p.PageID)},
		{Action: "Update a single context key", Cmd: "logos page set <path> <value>"},
	}
	if p.Alias == "" {
		hints = append(hints, Hint{
			Action: "Set an alias to reference this page by name",
			Cmd:    fmt.Sprintf("logos page alias <name> --page %s", p.PageID),
		})
	}
	return hints
}

// resolvePageContext resolves args/flags into a db connection with attached inner db.
// Priority: --page > --alias > --roll > positional arg > current cwd.
// Returns (name, version, pageID, conn).
func resolvePageContext(args []string, flagPage, flagAlias, flagRoll, cwd string) (string, string, string, *sql.DB) {
	// 1. --page: look up by page_id
	if flagPage != "" {
		name, version, outerPath, err := store.LookupPageByID(flagPage)
		if err != nil {
			outputFail(ErrCodePageNotFound, fmt.Sprintf("page %s not found: %v", flagPage, err), nil)
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		return name, version, flagPage, conn
	}

	// 2. --alias: look up by alias
	if flagAlias != "" {
		name, version, pageID, outerPath, err := store.LookupPageByAlias(flagAlias)
		if err != nil {
			outputFail(ErrCodePageNotFound, fmt.Sprintf("alias %s not found: %v", flagAlias, err), nil)
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		return name, version, pageID, conn
	}

	// 3. --roll: use default page for the named iroll
	if flagRoll != "" {
		pageID, err := store.GetDefaultPage(flagRoll)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		if pageID == "" {
			outputFail(ErrCodeNoDefaultPage, fmt.Sprintf("no default page for iroll '%s'", flagRoll), []Hint{
				{Action: "Create a new page for this iroll", Cmd: fmt.Sprintf("logos page new %s", flagRoll)},
				{Action: "List all pages to find one to set as default", Cmd: "logos page list -a"},
			})
		}
		_, version, outerPath, err := store.LookupPageByID(pageID)
		if err != nil {
			outputFail(ErrCodePageNotFound, fmt.Sprintf("default page %s gone: %v", pageID, err), nil)
		}
		innerPath := checkedInnerPath(flagRoll, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		return flagRoll, version, pageID, conn
	}

	// 4. Positional arg (iroll name): use default page for that iroll
	if len(args) > 0 {
		name, version, err := builder.ParseTag(args[0])
		if err != nil {
			outputFail(ErrCodeInvalidTag, fmt.Sprintf("invalid tag: %v", err), []Hint{
				{Action: "List all available iroll packages", Cmd: "logos status"},
			})
		}
		pageID, err := store.GetDefaultPage(name)
		if err != nil {
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		if pageID == "" {
			outputFail(ErrCodeNoDefaultPage, fmt.Sprintf("no default page for iroll '%s'", name), []Hint{
				{Action: "Create a new page and auto-set it as default", Cmd: fmt.Sprintf("logos page new %s", name)},
				{Action: "List all pages to find one to set as default", Cmd: "logos page list -a"},
			})
		}
		_, _, outerPath, err := store.LookupPageByID(pageID)
		if err != nil {
			outputFail(ErrCodePageNotFound, fmt.Sprintf("default page %s gone: %v", pageID, err), nil)
		}
		innerPath := checkedInnerPath(name, version)
		conn, err := db.OpenOuter(outerPath, innerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		return name, version, pageID, conn
	}

	// 5. Fallback: current cwd active page
	conn, irollName, irollVersion, pageID := openOuterFromActive(cwd)
	return irollName, irollVersion, pageID, conn
}

func init() {
	pageGetTarget.bind(pageGetCmd)
	pageSetTarget.bind(pageSetCmd)
	pageSetCmd.Flags().StringVar(&pageSetContent, "content", "", "Replace the whole context with this JSON")
	pageUnsetTarget.bind(pageUnsetCmd)
	pageAliasTarget.bind(pageAliasCmd)
	pageAliasCmd.Flags().BoolVar(&pageAliasClear, "clear", false, "Clear the alias")

	pageCmd.AddCommand(pageGetCmd)
	pageCmd.AddCommand(pageSetCmd)
	pageCmd.AddCommand(pageUnsetCmd)
	pageCmd.AddCommand(pageAliasCmd)
}
```

**Note:** `resolvePageContext` is preserved verbatim (it is still used by `query-memory` and `query-dna` via their own flag vars). The new commands pass `nil` as `args` so the positional-iroll-name branch is skipped — targeting is via flags only, leaving positionals free for `path`/`value`. The removed helper `getContextHints` had **no callers outside `context.go`** (verified), so the rewrite is self-contained.

- [ ] **Step 3: Build to verify it compiles**

Run: `cd iroll && go build ./...`
Expected: zero output (success). `resolvePageContext` and `openOuterFromActive` remain available to `query-memory`/`query-dna`. (Hint strings in other files still say `get-context` until Task 5 — those are plain strings, not compile errors.)

- [ ] **Step 4: Commit**

```bash
git add iroll/cmd/root.go iroll/cmd/context.go
git commit -m "feat(cmd): replace get-context/update-context with page get/set/unset/alias

- page get [path]: full resolved context or single key (incl. injected loop_focus/loop_available)
- page set <path> <value> | --content: per-key (json-or-text) or whole-context replace
- page unset <path>: delete a key
- page alias <name> | --clear: alias moved out of context commands
- add ErrCodeKeyNotFound"
```

---

## Task 3: cmd — shared resolveSQLInput + refactor evolving to template target

**Files:**
- Create: `iroll/cmd/sql_input.go`
- Modify: `iroll/cmd/evolving.go`

- [ ] **Step 1: Create the shared SQL-input helper**

Create `iroll/cmd/sql_input.go`:

```go
package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// resolveSQLInput resolves SQL text from, in priority order:
// the --sql flag, positional args (optionally skipping the first when it is a tag),
// the --file flag, or piped stdin. Returns "" if no input is available.
// skipFirstPositional is true for commands whose first positional is a target tag
// (e.g. evolving's name:version), false otherwise.
func resolveSQLInput(sqlFlag, fileFlag string, args []string, skipFirstPositional bool) string {
	if sqlFlag != "" {
		return sqlFlag
	}
	start := 0
	if skipFirstPositional && len(args) > 0 {
		start = 1
	}
	if len(args) > start {
		return strings.Join(args[start:], " ")
	}
	if fileFlag != "" {
		data, err := os.ReadFile(fileFlag)
		if err != nil {
			outputFail(ErrCodeInternal, fmt.Sprintf("read file %q: %v", fileFlag, err), nil)
		}
		return string(data)
	}
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			outputFail(ErrCodeInternal, fmt.Sprintf("read stdin: %v", err), nil)
		}
		return string(data)
	}
	return ""
}
```

- [ ] **Step 2: Refactor evolving.go**

The current `evolving.go` opens the workspace-default live outer and copies the template if missing. Change it to open the **template** `roll-outer.db` directly (roll-level), and slim `resolveEvolvingSQL` to a wrapper around `resolveSQLInput`.

Replace the import block and the two functions `runEvolving` and `resolveEvolvingSQL` in `iroll/cmd/evolving.go`. The `resolveEvolvingTarget`, `isTagArg`, and `init()` functions stay unchanged.

New imports (remove `io` — no longer used directly; keep `os` for `os.Stat`/`os.Stdin` is no longer direct either since stdin moved to `resolveSQLInput`; verify):

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"logos/builder"
	"logos/db"
	"logos/safepath"
	"logos/store"

	"github.com/spf13/cobra"
)
```

Replace `runEvolving` with:

```go
func runEvolving(cmd *cobra.Command, args []string) {
	name, version := resolveEvolvingTarget(args)
	innerPath := checkedInnerPath(name, version)

	// evolving operates at the ROLL level: the template roll-outer.db + inner.db.
	// It never touches page-level (cwd) live outer databases.
	templateOuter := filepath.Join(checkedIrollPath(name, version), "roll-outer.db")
	if _, err := os.Stat(templateOuter); err != nil {
		outputFail(ErrCodeIrollNotFound, fmt.Sprintf("template outer db not found for %s:%s: %v", name, version, err), nil)
	}

	sql := resolveEvolvingSQL(args)
	if strings.TrimSpace(sql) == "" {
		outputFail(ErrCodeInternal, "no SQL provided (use --sql, positional args, --file, or stdin)", nil)
	}

	// Open the template as the main db with inner attached (both read-write).
	// Bare tables address the template outer; inner.* addresses the blueprint.
	conn, err := db.OpenOuter(templateOuter, innerPath)
	if err != nil {
		outputFail(ErrCodeDBOpen, err.Error(), nil)
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, sql, evolvingDryRun)
	if err != nil {
		if len(results) > 0 {
			outputOK(results, nil)
		}
		outputFail(ErrCodeInternal, err.Error(), nil)
	}
	outputOK(results, nil)
}
```

Replace `resolveEvolvingSQL` (the old multi-priority version) with the wrapper:

```go
// resolveEvolvingSQL resolves the SQL input for evolving. The first positional arg
// is a target tag (name:version), so it is skipped when extracting SQL from positionals.
func resolveEvolvingSQL(args []string) string {
	return resolveSQLInput(evolvingSQL, evolvingFile, args, isTagArg(args))
}
```

Delete the now-unused `resolveEvolvingSQL` helper body that read `--file`/stdin inline (it is replaced by `resolveSQLInput`). Keep `isTagArg` and `resolveEvolvingTarget` exactly as they are.

**Verify imports:** after this change, `evolving.go` no longer uses `io` (was `io.ReadAll`), but still uses `os` (`os.Stat`), `strings` (`strings.TrimSpace`, and `isTagArg` uses `strings.Contains`/`SplitN`), `path/filepath`, `safepath`, `builder`, `store`, `fmt`. The import block above matches. If `go build` reports an unused import, remove it; if it reports a missing one, add it.

- [ ] **Step 3: Build and run existing evolving tests**

Run: `cd iroll && go build ./... && go test ./cmd/ -run 'TestResolveEvolvingSQL|TestResolveEvolvingTarget' -v`
Expected: PASS — `resolveEvolvingSQL` behavior is unchanged (the wrapper preserves the old priority order: `--sql` > positional-skip-tag > `--file` > stdin).

- [ ] **Step 4: Commit**

```bash
git add iroll/cmd/sql_input.go iroll/cmd/evolving.go
git commit -m "refactor(cmd): evolving targets roll-level template; share resolveSQLInput

- runEvolving opens the template roll-outer.db + inner (rw) instead of the
  workspace-default live outer; never touches page-level cwd data
- extract resolveSQLInput (flag/positional/file/stdin) for reuse by page query
- resolveEvolvingSQL becomes a thin wrapper (behavior unchanged)"
```

---

## Task 4: cmd — page query command

**Files:**
- Create: `iroll/cmd/page_query.go`
- Test: `iroll/cmd/page_query_test.go`

- [ ] **Step 1: Write the failing test**

Create `iroll/cmd/page_query_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"logos/db"
	"logos/store"
)

// setupPageQueryTest builds a roll + one cwd page (with a seeded memory row) and
// returns the cwd. The cwd outer path is registered via IndexPage so GetActive returns it.
func setupPageQueryTest(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	cwd := filepath.Join(t.TempDir(), "workspace")
	rollName := "test-roll"
	rollRoot, err := store.IrollPath(rollName, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rollRoot, 0755); err != nil {
		t.Fatal(err)
	}

	// inner.db with schema (keeps the roll well-formed; not opened by page query).
	innerConn, err := db.Open(filepath.Join(rollRoot, "roll-inner.db"))
	if err != nil {
		t.Fatal(err)
	}
	innerSchema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_inner.sql"))
	if err != nil {
		innerConn.Close()
		t.Fatal(err)
	}
	if _, err := innerConn.Exec(string(innerSchema)); err != nil {
		innerConn.Close()
		t.Fatal(err)
	}
	if err := innerConn.Close(); err != nil {
		t.Fatal(err)
	}

	// cwd outer db: create dir, open, apply schema, seed a memory row.
	outerPath, err := store.CwdOuterDbPath(cwd, rollName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(outerPath), 0755); err != nil {
		t.Fatal(err)
	}
	outerConn, err := db.Open(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	outerSchema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_outer.sql"))
	if err != nil {
		outerConn.Close()
		t.Fatal(err)
	}
	if _, err := outerConn.Exec(string(outerSchema)); err != nil {
		outerConn.Close()
		t.Fatal(err)
	}
	if _, err := outerConn.Exec(`INSERT INTO memory (page_id, name, question, content, importance, sleep_count, created_at, updated_at)
		VALUES ('page-one','m1','q?','c',0.5,0,datetime('now'),datetime('now'))`); err != nil {
		outerConn.Close()
		t.Fatal(err)
	}
	if err := outerConn.Close(); err != nil {
		t.Fatal(err)
	}

	// Register the page with its REAL outer path so resolveActiveOuter can find it.
	if err := store.IndexPage(rollName, "latest", "page-one", cwd, outerPath, ""); err != nil {
		t.Fatal(err)
	}
	return cwd
}

func TestPageQuerySelectMemory(t *testing.T) {
	cwd := setupPageQueryTest(t)

	origCwd := pageQueryTarget.cwd
	origSQL := pageQuerySQL
	pageQueryTarget.cwd = cwd
	pageQuerySQL = "SELECT name FROM memory WHERE page_id='page-one'"
	defer func() {
		pageQueryTarget.cwd = origCwd
		pageQuerySQL = origSQL
	}()

	outerPath, pageID := resolveActiveOuter("", "", cwd)
	if pageID != "page-one" {
		t.Fatalf("pageID = %q, want page-one", pageID)
	}
	conn, err := db.Open(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, pageQuerySQL, false)
	if err != nil {
		t.Fatalf("ExecuteAll: %v", err)
	}
	if len(results) != 1 || results[0].Type != "rows" || results[0].Count != 1 {
		t.Fatalf("results = %#v", results)
	}
}

func TestPageQueryCannotMutateInner(t *testing.T) {
	cwd := setupPageQueryTest(t)

	pageQueryTarget.cwd = cwd
	defer func() { pageQueryTarget.cwd = "" }()

	outerPath, _ := resolveActiveOuter("", "", cwd)
	conn, err := db.Open(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// inner is not attached to the standalone outer connection, so inner.* is unreachable.
	_, err = db.ExecuteAll(conn, "SELECT count(*) FROM inner.dna", false)
	if err == nil {
		t.Fatal("expected error: inner.* must not be reachable from page query (standalone outer)")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd iroll && go test ./cmd/ -run 'TestPageQuery' -v`
Expected: FAIL / build error — `pageQueryTarget`, `pageQuerySQL`, `resolveActiveOuter` undefined.

- [ ] **Step 3: Write the implementation**

Create `iroll/cmd/page_query.go`:

```go
package cmd

import (
	"path/filepath"
	"strings"

	"logos/db"
	"logos/store"

	"github.com/spf13/cobra"
)

var (
	pageQuerySQL    string
	pageQueryFile   string
	pageQueryDryRun bool
)

// pageQueryTarget holds targeting flags for page query (no --roll: it is page-scoped).
var pageQueryTarget struct {
	page, alias, cwd string
}

// resolveActiveOuter resolves --page / --alias / --cwd to the cwd outer.db path + pageID.
// Used by `page query`, which opens that outer standalone (no inner attach).
func resolveActiveOuter(flagPage, flagAlias, cwd string) (outerPath, pageID string) {
	if flagPage != "" {
		_, _, op, err := store.LookupPageByID(flagPage)
		if err != nil {
			outputFail(ErrCodePageNotFound, flagPage+" not found: "+err.Error(), nil)
		}
		return op, flagPage
	}
	if flagAlias != "" {
		_, _, pid, op, err := store.LookupPageByAlias(flagAlias)
		if err != nil {
			outputFail(ErrCodePageNotFound, "alias "+flagAlias+" not found: "+err.Error(), nil)
		}
		return op, pid
	}
	_, _, pid, op, err := store.GetActive(cwd)
	if err != nil {
		outputFail(ErrCodeNoActivePage, err.Error(), []Hint{
			{Action: "Create a new page for this directory", Cmd: "logos page new <iroll-name>"},
			{Action: "List all pages", Cmd: "logos page list -a"},
		})
	}
	return op, pid
}

var pageQueryCmd = &cobra.Command{
	Use:   "query [sql]",
	Short: "Run raw SQL against this page's outer database (pages / memory / loop_runs)",
	Long: `Run raw SQL against the current page's outer database.
Target page is resolved from --page, --alias, or the active page of --cwd.

Only the outer tables (pages, memory, loop_runs) are visible; inner tables
(dna, loop seeds, skills, metadata) are NOT attached — use query-dna or
roll evolving for those.

SQL input priority: --sql flag, positional args, --file flag, stdin.`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := filepath.Abs(pageQueryTarget.cwd)
		outerPath, _ := resolveActiveOuter(pageQueryTarget.page, pageQueryTarget.alias, cwd)

		// Standalone outer connection (read-write). No inner attach by design.
		conn, err := db.Open(outerPath)
		if err != nil {
			outputFail(ErrCodeDBOpen, err.Error(), nil)
		}
		defer conn.Close()

		sqlText := resolveSQLInput(pageQuerySQL, pageQueryFile, args, false)
		if strings.TrimSpace(sqlText) == "" {
			outputFail(ErrCodeInternal, "no SQL provided (use --sql, positional args, --file, or stdin)", nil)
		}

		results, err := db.ExecuteAll(conn, sqlText, pageQueryDryRun)
		if err != nil {
			if len(results) > 0 {
				outputOK(results, nil)
			}
			outputFail(ErrCodeInternal, err.Error(), nil)
		}
		outputOK(results, nil)
	},
}

func init() {
	pageQueryCmd.Flags().StringVar(&pageQueryTarget.page, "page", "", "Page ID")
	pageQueryCmd.Flags().StringVar(&pageQueryTarget.alias, "alias", "", "Page alias")
	pageQueryCmd.Flags().StringVar(&pageQueryTarget.cwd, "cwd", ".", "Working directory")
	pageQueryCmd.Flags().StringVar(&pageQuerySQL, "sql", "", "SQL statement(s) to execute")
	pageQueryCmd.Flags().StringVar(&pageQueryFile, "file", "", "Path to SQL file")
	pageQueryCmd.Flags().BoolVar(&pageQueryDryRun, "dry-run", false, "Preview mode: execute in a transaction then rollback")

	pageCmd.AddCommand(pageQueryCmd)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd iroll && go test ./cmd/ -run 'TestPageQuery' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add iroll/cmd/page_query.go iroll/cmd/page_query_test.go
git commit -m "feat(cmd): add page query — raw SQL on the page's outer database

Opens the cwd outer.db standalone (read-write, no inner attach), enforcing
that page query can only see/mutate outer tables (pages/memory/loop_runs).
Reuses the evolving SQL engine (db.ExecuteAll) and shared resolveSQLInput."
```

---

## Task 5: mechanical — hint sweep + docs

**Files:**
- Modify: `iroll/cmd/loop_run.go`, `iroll/cmd/memory.go`, `iroll/cmd/query_dna.go`, `iroll/cmd/page.go`
- Modify: `iroll/db/db.go` (comment), `skills/logos-1/skill.md`, `README.md`, `docs/rebot-roll.md`

- [ ] **Step 1: Update hint strings in cmd files**

These are string-only edits (no logic). For each, replace the `Cmd:` value:

**`iroll/cmd/loop_run.go`** (2 sites, lines ~218 and ~229):
- `"logos page get-context"` → `"logos page get"`

**`iroll/cmd/memory.go`** (2 sites, lines ~50 and ~68):
- `"logos page get-context"` → `"logos page get"`

**`iroll/cmd/query_dna.go`** (line ~39):
- `fmt.Sprintf("logos page update-context --page <page-id> --content '{\"dna_answer\":\"%s\"}'", results[0].Answer)` → `fmt.Sprintf("logos page set --page <page-id> dna_answer '{\"answer\":\"%s\"}'", results[0].Answer)`

  (line ~44):
- `"logos page get-context"` → `"logos page get"`

**`iroll/cmd/page.go`** (7 sites):
- line ~48: `fmt.Sprintf("logos page get-context --page %s", pid)` → `fmt.Sprintf("logos page get --page %s", pid)`
- line ~99: `fmt.Sprintf("logos page get-context --page %s", briefs[0].PageID)` → `fmt.Sprintf("logos page get --page %s", briefs[0].PageID)`
- line ~170: `fmt.Sprintf("logos page update-context --page %s --set-alias <name>", p.PageID)` → `fmt.Sprintf("logos page alias <name> --page %s", p.PageID)`
- line ~171: `fmt.Sprintf("logos page get-context --page %s", p.PageID)` → `fmt.Sprintf("logos page get --page %s", p.PageID)`
- line ~193: `fmt.Sprintf("logos page get-context --page %s", pageID)` → `fmt.Sprintf("logos page get --page %s", pageID)`
- line ~240: `fmt.Sprintf("logos page get-context --page %s", pageID)` → `fmt.Sprintf("logos page get --page %s", pageID)`
- line ~278: `fmt.Sprintf("logos page get-context --page %s", pageID)` → `fmt.Sprintf("logos page get --page %s", pageID)`

**`iroll/db/db.go`** (line ~33, comment on `PageBrief`):
- `// used for structured CLI output to encourage agent to call get-context.` → `// used for structured CLI output to encourage agent to call page get.`

- [ ] **Step 2: Verify no stale references remain**

Run: `cd iroll && grep -rn "get-context\|update-context" --include="*.go" .`
Expected: no matches (all renamed). Also run from repo root: `grep -rn "get-context\|update-context" --include="*.go" .` — expect none.

- [ ] **Step 3: Update skill.md**

In `skills/logos-1/skill.md`:
- "Step 4: Read context" section: replace `logos page get-context --cwd .` with `logos page get --cwd .`, and update prose ("get-context" → "page get").
- "During Conversation → Update page context": replace `logos page update-context --content '{...}' --cwd .` with `logos page set --content '{...}' --cwd .` (whole replace) and add a one-line example for per-key: `logos page set user_context.project blog --cwd .`.
- Command Reference table: replace the `page get-context` and `page update-context` rows with:

```
| `logos page get [path] [--page <id>] [--alias <name>] [--cwd .]` | Get full context or a single resolved key |
| `logos page set <path> <value> [--page <id>] [--cwd .]` | Set a context key (json-or-text) |
| `logos page set --content '<json>' [--page <id>] [--cwd .]` | Replace the whole context |
| `logos page unset <path> [--page <id>] [--cwd .]` | Delete a context key |
| `logos page alias <name> [--page <id>]` | Set/clear page alias |
| `logos page query [sql] [--sql <stmt>] [--file <p>] [--cwd .]` | Raw SQL on this page's outer db |
```

- [ ] **Step 4: Update README.md and docs/rebot-roll.md**

In `README.md` (页面管理 section): replace `page get-context` / `page update-context` lines with the new `page get` / `page set` / `page unset` / `page alias` / `page query` lines. Remove the `page current` line if still present (it was deleted earlier).

In `docs/rebot-roll.md` §5.3: same replacement in the command table; add `page query` row under a new "数据查询" note or alongside query-memory/query-dna.

- [ ] **Step 5: Build + run full test suite**

Run: `cd iroll && go build ./... && go test ./...`
Expected: build success, all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add iroll/cmd/loop_run.go iroll/cmd/memory.go iroll/cmd/query_dna.go iroll/cmd/page.go iroll/db/db.go skills/logos-1/skill.md README.md docs/rebot-roll.md
git commit -m "docs: rename hints and docs from get-context/update-context to page get/set/unset/alias/query"
```

---

## Task 6: final verification

- [ ] **Step 1: Manual smoke test (build the binary)**

Run:
```bash
cd iroll
go build -ldflags "-X logos/cmd.Version=0.3.0" -o ../logos .
cd ..
./logos roll build -f examples/base-agent/Irollfile -t smoke-agent
./logos page new smoke-agent --cwd .
./logos page set user_context.project blog --cwd .
./logos page get user_context.project --cwd .
./logos page get --cwd . | head -5
./logos page unset user_context.project --cwd .
./logos page alias demo --cwd .
./logos page query "SELECT count(*) FROM memory" --cwd .
```
Expected: `page set` then `page get user_context.project` echoes `blog`; `page query` returns a count (likely 0 or the template's base memory). No errors.

- [ ] **Step 2: Verify evolving targets the template**

Run:
```bash
./logos roll evolving smoke-agent:latest --sql "SELECT count(*) FROM inner.dna"
```
Expected: returns the dna row count from the inner blueprint (e.g. 4 for base-agent). This confirms evolving reads inner via the template open.

- [ ] **Step 3: Full test suite + go vet**

Run: `cd iroll && go vet ./... && go test ./...`
Expected: vet clean, all tests PASS.

- [ ] **Step 4: Commit any remaining changes (if smoke test surfaced fixes)**

If no changes, skip. Otherwise commit with a clear message.

---

## Self-Review

**Spec coverage:**
- §3.1 commands (get/set/unset/alias) → Task 2 ✓
- §3.2 path syntax (dot, auto-create, key_not_found) → Task 1 (navigateSet auto-create, ErrContextKeyNotFound) ✓
- §3.3 json-or-text + raw marker storage → Task 1 (parseJSONOrText, TestSetContextKeyStoresMarkerRaw) ✓
- §3.4 behaviors (no-path full / path single) → Task 2 ✓
- §3.5 ErrCodeKeyNotFound → Task 2 ✓
- §4.1 evolving → template+inner rw → Task 3 ✓
- §4.2 page query (cwd outer, free SQL, reuses ExecuteAll) → Task 4 ✓
- §2 boundary (page query cannot mutate inner) → Task 4 (standalone open; TestPageQueryCannotMutateInner proves inner.* unreachable) ✓ (refined: no-attach instead of ro-attach)
- §6 migration (remove old cmds, hints, skill.md, README) → Tasks 2 + 5 ✓

**Placeholder scan:** none — all code blocks are complete.

**Type consistency:** `pageTarget.bind` (Task 2) used by get/set/unset/alias; `pageQueryTarget` (Task 4) is a separate anonymous struct with page/alias/cwd only; `resolveSQLInput` (Task 3) signature `(sqlFlag, fileFlag string, args []string, skipFirstPositional bool) string` matches both callers; `ErrContextKeyNotFound` (Task 1) matched by `errors.Is` in Task 2; `pageBrief` (Task 2) returns `db.PageBrief`.

**One refinement flagged vs spec:** §2 said "inner attached read-only" for `page query`. The plan opens the cwd outer **standalone** (no inner attach) — simpler, Windows-robust, and enforces an even stricter boundary (inner unreachable, not just read-only). This satisfies the spec's intent ("page query only touches outer") and the user's core requirement ("page query is CRUD on outer"). Flag to user before execution.
