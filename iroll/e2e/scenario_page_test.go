package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"logos/db"
	"logos/e2e/testenv"
	"logos/store"
)

// TestPageNewInheritsTemplate verifies that a newly inserted page inherits the
// template page's context (page_id='0'). We first update the template to contain
// a known system_prompt and greeting @file reference, then insert a new page and
// confirm it inherits that structure.
func TestPageNewInheritsTemplate(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("page-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("page-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	// Update the template page (page_id='0') with known context
	templateCtx := `{"system_prompt":"你是一个AI助手","greeting":{"@file":"Resources/greeting.txt"}}`
	if _, err := db.UpdatePageContext(conn, "0", templateCtx); err != nil {
		t.Fatalf("UpdatePageContext template: %v", err)
	}

	// Insert a new page — it should inherit the template context
	cwd := filepath.Join(t.TempDir(), "ws")
	page, err := db.InsertPage(conn, cwd)
	if err != nil {
		t.Fatalf("InsertPage returned error: %v", err)
	}

	// Verify the new page's context matches the template
	var ctx map[string]interface{}
	if err := json.Unmarshal([]byte(page.Context), &ctx); err != nil {
		t.Fatalf("parse new page context: %v", err)
	}
	if sp, ok := ctx["system_prompt"].(string); !ok || sp != "你是一个AI助手" {
		t.Fatalf("new page system_prompt = %v, want %q", ctx["system_prompt"], "你是一个AI助手")
	}
	if greeting, ok := ctx["greeting"].(map[string]interface{}); !ok {
		t.Fatalf("new page greeting = %v, want object with @file", ctx["greeting"])
	} else if ref, ok := greeting["@file"].(string); !ok || ref != "Resources/greeting.txt" {
		t.Fatalf("new page greeting @file = %v, want %q", greeting["@file"], "Resources/greeting.txt")
	}
}

// TestPageCurrentResolvesActive verifies that after inserting a page and indexing
// it, GetActive returns the correct iroll name and page ID for the cwd.
func TestPageCurrentResolvesActive(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("page-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	cwd := filepath.Join(t.TempDir(), "ws")
	page, err := env.CreatePage("page-test", "latest", "", cwd)
	if err != nil {
		t.Fatalf("CreatePage returned error: %v", err)
	}

	name, _, pageID, err := store.GetActive(cwd)
	if err != nil {
		t.Fatalf("GetActive returned error: %v", err)
	}
	if name != "page-test" {
		t.Fatalf("GetActive name = %q, want %q", name, "page-test")
	}
	if pageID != page.PageID {
		t.Fatalf("GetActive pageID = %q, want %q", pageID, page.PageID)
	}
}

// TestPageListShowsAllPages verifies that inserting two pages under the same cwd
// causes ListPagesByCwd to return at least those two entries.
func TestPageListShowsAllPages(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("page-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("page-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	cwd := filepath.Join(t.TempDir(), "ws")
	if _, err := db.InsertPage(conn, cwd); err != nil {
		t.Fatalf("InsertPage 1 returned error: %v", err)
	}
	if _, err := db.InsertPage(conn, cwd); err != nil {
		t.Fatalf("InsertPage 2 returned error: %v", err)
	}

	pages, err := db.ListPagesByCwd(conn, cwd)
	if err != nil {
		t.Fatalf("ListPagesByCwd returned error: %v", err)
	}
	if len(pages) < 2 {
		t.Fatalf("ListPagesByCwd returned %d pages, want at least 2", len(pages))
	}
}

// TestPageSwitchChangesActive verifies that inserting two pages, making the first
// active, then switching to the second causes GetActive to return the second page.
func TestPageSwitchChangesActive(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("page-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("page-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	cwd := filepath.Join(t.TempDir(), "ws")

	// Create page1 and index it (makes it active)
	page1, err := db.InsertPage(conn, cwd)
	if err != nil {
		t.Fatalf("InsertPage 1 returned error: %v", err)
	}
	if err := store.IndexPage("page-test", "latest", page1.PageID, cwd); err != nil {
		t.Fatalf("IndexPage 1 returned error: %v", err)
	}

	// Verify page1 is active
	name, _, activeID, err := store.GetActive(cwd)
	if err != nil {
		t.Fatalf("GetActive after page1 returned error: %v", err)
	}
	if activeID != page1.PageID {
		t.Fatalf("active page after page1 = %q, want %q", activeID, page1.PageID)
	}

	// Create page2 and index it
	page2, err := db.InsertPage(conn, cwd)
	if err != nil {
		t.Fatalf("InsertPage 2 returned error: %v", err)
	}
	if err := store.IndexPage("page-test", "latest", page2.PageID, cwd); err != nil {
		t.Fatalf("IndexPage 2 returned error: %v", err)
	}

	// Switch to page1 (since page2 was made active by IndexPage above)
	if _, _, err := store.SwitchPage(page1.PageID); err != nil {
		t.Fatalf("SwitchPage to page1 returned error: %v", err)
	}

	// Now switch back to page2
	if _, _, err := store.SwitchPage(page2.PageID); err != nil {
		t.Fatalf("SwitchPage to page2 returned error: %v", err)
	}

	// Verify page2 is now active
	name, _, activeID, err = store.GetActive(cwd)
	if err != nil {
		t.Fatalf("GetActive after switch returned error: %v", err)
	}
	if name != "page-test" {
		t.Fatalf("active name = %q, want %q", name, "page-test")
	}
	if activeID != page2.PageID {
		t.Fatalf("active page after switch = %q, want %q", activeID, page2.PageID)
	}
}

// TestContextUpdateAndResolve verifies that a context with mixed content types —
// a plain string, an @file reference, and an @sql reference — resolves correctly.
// The @file reference should become the file's content, and @sql should become
// query results.
func TestContextUpdateAndResolve(t *testing.T) {
	env := testenv.New(t)

	result, err := env.Build("page-test")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("page-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	cwd := filepath.Join(t.TempDir(), "ws")
	page, err := db.InsertPage(conn, cwd)
	if err != nil {
		t.Fatalf("InsertPage returned error: %v", err)
	}

	// Create context with mixed content
	rawCtx := `{"system_prompt":"hello","greeting":{"@file":"Resources/greeting.txt"},"name_val":{"@sql":"SELECT value FROM metadata WHERE key = 'name'"}}`
	updated, err := db.UpdatePageContext(conn, page.PageID, rawCtx)
	if err != nil {
		t.Fatalf("UpdatePageContext returned error: %v", err)
	}

	// Resolve the context
	resolved, err := db.ResolveContext(updated.Context, result.Path, conn, page.PageID)
	if err != nil {
		t.Fatalf("ResolveContext returned error: %v", err)
	}

	var ctx map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &ctx); err != nil {
		t.Fatalf("parse resolved context: %v", err)
	}

	// Plain string should pass through
	if sp, _ := ctx["system_prompt"].(string); sp != "hello" {
		t.Fatalf("system_prompt = %q, want %q", sp, "hello")
	}

	// @file should resolve to greeting.txt content ("Hello from test-agent\n")
	if greeting, _ := ctx["greeting"].(string); !strings.Contains(greeting, "Hello from test-agent") {
		t.Fatalf("greeting = %q, want to contain %q", greeting, "Hello from test-agent")
	}

	// @sql should resolve to the metadata name value
	if nameVal, _ := ctx["name_val"].(string); nameVal != "base-cat" {
		t.Fatalf("name_val = %q, want %q", nameVal, "base-cat")
	}
}

// TestContextFileRefResolvesContent verifies that writing a known file into the
// iroll's Resources directory and referencing it via @file resolves to that
// file's content.
func TestContextFileRefResolvesContent(t *testing.T) {
	env := testenv.New(t)

	result, err := env.Build("page-test")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("page-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	// Write a known file to Resources/ in the iroll
	knownContent := "known file content for test"
	resourcesDir := filepath.Join(result.Path, "Resources")
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		t.Fatalf("MkdirAll Resources: %v", err)
	}
	testFile := filepath.Join(resourcesDir, "test-file.txt")
	if err := os.WriteFile(testFile, []byte(knownContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cwd := filepath.Join(t.TempDir(), "ws")
	page, err := db.InsertPage(conn, cwd)
	if err != nil {
		t.Fatalf("InsertPage returned error: %v", err)
	}

	// Context referencing the test file
	rawCtx := `{"content":{"@file":"Resources/test-file.txt"}}`
	updated, err := db.UpdatePageContext(conn, page.PageID, rawCtx)
	if err != nil {
		t.Fatalf("UpdatePageContext returned error: %v", err)
	}

	resolved, err := db.ResolveContext(updated.Context, result.Path, conn, page.PageID)
	if err != nil {
		t.Fatalf("ResolveContext returned error: %v", err)
	}

	var ctx map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &ctx); err != nil {
		t.Fatalf("parse resolved context: %v", err)
	}

	if got, _ := ctx["content"].(string); got != knownContent {
		t.Fatalf("resolved content = %q, want %q", got, knownContent)
	}
}

// TestContextSQLRefResolvesQuery verifies that an @sql reference in the context
// resolves to query results from the iroll's database.
func TestContextSQLRefResolvesQuery(t *testing.T) {
	env := testenv.New(t)

	result, err := env.Build("page-test")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("page-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	cwd := filepath.Join(t.TempDir(), "ws")
	page, err := db.InsertPage(conn, cwd)
	if err != nil {
		t.Fatalf("InsertPage returned error: %v", err)
	}

	// Context with @sql querying metadata
	rawCtx := `{"meta":{"@sql":"SELECT key, value FROM metadata ORDER BY key"}}`
	updated, err := db.UpdatePageContext(conn, page.PageID, rawCtx)
	if err != nil {
		t.Fatalf("UpdatePageContext returned error: %v", err)
	}

	resolved, err := db.ResolveContext(updated.Context, result.Path, conn, page.PageID)
	if err != nil {
		t.Fatalf("ResolveContext returned error: %v", err)
	}

	var ctx map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &ctx); err != nil {
		t.Fatalf("parse resolved context: %v", err)
	}

	meta, ok := ctx["meta"].([]interface{})
	if !ok {
		t.Fatalf("meta = %T %v, want array", ctx["meta"], ctx["meta"])
	}
	if len(meta) == 0 {
		t.Fatal("meta array is empty, want at least one row from metadata")
	}

	// Each row should be a map with "key" and "value" fields
	firstRow, ok := meta[0].(map[string]interface{})
	if !ok {
		t.Fatalf("meta[0] = %T %v, want map", meta[0], meta[0])
	}
	if _, hasKey := firstRow["key"]; !hasKey {
		t.Fatalf("meta[0] missing 'key' field: %v", firstRow)
	}
	if _, hasVal := firstRow["value"]; !hasVal {
		t.Fatalf("meta[0] missing 'value' field: %v", firstRow)
	}
}

// TestPageDeleteCleansUp verifies that deleting a page removes it from both the
// iroll's database and the store index (page_index and active_page).
func TestPageDeleteCleansUp(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("page-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("page-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	cwd := filepath.Join(t.TempDir(), "ws")
	page, err := db.InsertPage(conn, cwd)
	if err != nil {
		t.Fatalf("InsertPage returned error: %v", err)
	}
	if err := store.IndexPage("page-test", "latest", page.PageID, cwd); err != nil {
		t.Fatalf("IndexPage returned error: %v", err)
	}

	// Verify page exists before deletion
	if _, err := db.GetPageByPageID(conn, page.PageID); err != nil {
		t.Fatalf("GetPageByPageID before delete: %v", err)
	}

	// Verify active page is set
	name, _, activeID, err := store.GetActive(cwd)
	if err != nil {
		t.Fatalf("GetActive before delete: %v", err)
	}
	if name != "page-test" || activeID != page.PageID {
		t.Fatalf("active before delete = %q %q, want %q %q", name, activeID, "page-test", page.PageID)
	}

	// Delete the page (via store, which removes from both DB and index)
	if err := store.DeletePage(page.PageID); err != nil {
		t.Fatalf("DeletePage returned error: %v", err)
	}

	// Verify page is gone from DB
	if _, err := db.GetPageByPageID(conn, page.PageID); err == nil {
		t.Fatal("page still exists in DB after deletion")
	}

	// Verify active page is cleared
	if _, _, _, err := store.GetActive(cwd); err == nil {
		t.Fatal("active page still exists after deletion")
	}
}
