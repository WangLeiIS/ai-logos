package e2e

import (
	"encoding/json"
	"testing"

	"logos/db"
	"logos/e2e/testenv"
	"logos/store"
)

// TestDNAInsertAndQuery inserts 3 DNA entries via raw SQL with different types
// and verifies QueryDna returns matches by keyword, by type, and returns all
// when no filters are provided.
func TestDNAInsertAndQuery(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("dna-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("dna-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	// Insert 3 DNA entries with different types via raw SQL.
	entries := []struct {
		name, dnaType, question, answer string
		weight                          float64
	}{
		{"greeting-style", "审美观", "How to greet users?", "Warm and concise", 0.7},
		{"error-handling", "认知观", "What to do on errors?", "Acknowledge and retry", 0.8},
		{"privacy-policy", "伦理观", "Should we log personal data?", "Never log PII", 0.9},
	}
	for _, e := range entries {
		_, err := conn.Exec(
			`INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
			e.name, e.dnaType, e.question, e.answer, e.weight,
		)
		if err != nil {
			t.Fatalf("insert DNA %q: %v", e.name, err)
		}
	}

	// QueryDna by keyword should return matching entries.
	results, err := db.QueryDna(conn, "greeting", "")
	if err != nil {
		t.Fatalf("QueryDna by keyword: %v", err)
	}
	if len(results) != 1 || results[0].Name != "greeting-style" {
		t.Fatalf("QueryDna('greeting','') = %d results, want 1 with name greeting-style", len(results))
	}

	// QueryDna by type should return only matching type entries.
	// init_data.sql seeds idea/emotion entries, so only our inserted
	// 'error-handling' entry should match '认知观'.
	results, err = db.QueryDna(conn, "", "认知观")
	if err != nil {
		t.Fatalf("QueryDna by type: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("QueryDna('','认知观') = %d results, want 1", len(results))
	}
	for _, d := range results {
		if d.Type != "认知观" {
			t.Fatalf("QueryDna type filter returned entry with type %q, want '认知观'", d.Type)
		}
	}

	// QueryDna with no filters returns all 3 new entries (plus the 4 from init_data).
	results, err = db.QueryDna(conn, "", "")
	if err != nil {
		t.Fatalf("QueryDna all: %v", err)
	}
	// init_data.sql inserts 4 + our 3 = 7 total.
	if len(results) != 7 {
		t.Fatalf("QueryDna('','') returned %d entries, want 7", len(results))
	}
}

// TestMemoryPageIsolation creates 2 pages, inserts memory for each, and verifies
// that querying each page only returns its own memories.
func TestMemoryPageIsolation(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("mem-iso-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("mem-iso-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	// Create two pages.
	page1, err := db.InsertPage(conn, "/project/alpha")
	if err != nil {
		t.Fatalf("InsertPage page1: %v", err)
	}
	page2, err := db.InsertPage(conn, "/project/beta")
	if err != nil {
		t.Fatalf("InsertPage page2: %v", err)
	}

	// Insert memory for each page.
	mem1, err := db.InsertMemory(conn, page1.PageID, "alpha-context", "What is alpha?", "Alpha project context", 0.8)
	if err != nil {
		t.Fatalf("InsertMemory page1: %v", err)
	}
	mem2, err := db.InsertMemory(conn, page2.PageID, "beta-context", "What is beta?", "Beta project context", 0.7)
	if err != nil {
		t.Fatalf("InsertMemory page2: %v", err)
	}

	// Query page1 should only return mem1 (plus base-layer memory from init_data for page '0').
	results1, err := db.QueryMemory(conn, page1.PageID, db.QueryMemoryParams{})
	if err != nil {
		t.Fatalf("QueryMemory page1: %v", err)
	}
	for _, m := range results1 {
		if m.PageID != page1.PageID {
			t.Fatalf("page1 query returned memory from page %q", m.PageID)
		}
	}
	foundMem1 := false
	for _, m := range results1 {
		if m.ID == mem1.ID {
			foundMem1 = true
		}
	}
	if !foundMem1 {
		t.Fatal("page1 query did not return the inserted memory")
	}

	// Query page2 should only return mem2.
	results2, err := db.QueryMemory(conn, page2.PageID, db.QueryMemoryParams{})
	if err != nil {
		t.Fatalf("QueryMemory page2: %v", err)
	}
	foundMem2 := false
	for _, m := range results2 {
		if m.ID == mem2.ID {
			foundMem2 = true
		}
		if m.PageID != page2.PageID {
			t.Fatalf("page2 query returned memory from page %q", m.PageID)
		}
	}
	if !foundMem2 {
		t.Fatal("page2 query did not return the inserted memory")
	}
}

// TestMemoryQueryFilters inserts 3 memories with different importance values
// and tests keyword filter, min-importance filter, and limit. Verifies ordering
// by importance DESC.
func TestMemoryQueryFilters(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("mem-filter-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("mem-filter-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	page, err := db.InsertPage(conn, "/project/filter")
	if err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Insert 3 memories with different importance.
	_, err = db.InsertMemory(conn, page.PageID, "high-priority", "What is critical?", "Critical info", 0.9)
	if err != nil {
		t.Fatalf("InsertMemory high: %v", err)
	}
	_, err = db.InsertMemory(conn, page.PageID, "mid-priority", "What is normal?", "Normal info", 0.5)
	if err != nil {
		t.Fatalf("InsertMemory mid: %v", err)
	}
	_, err = db.InsertMemory(conn, page.PageID, "low-priority", "What is low?", "Low info", 0.1)
	if err != nil {
		t.Fatalf("InsertMemory low: %v", err)
	}

	// Test keyword filter.
	results, err := db.QueryMemory(conn, page.PageID, db.QueryMemoryParams{Keyword: "high"})
	if err != nil {
		t.Fatalf("QueryMemory keyword: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("keyword 'high' returned %d, want 1", len(results))
	}
	if results[0].Name != "high-priority" {
		t.Fatalf("keyword result name = %q, want 'high-priority'", results[0].Name)
	}

	// Test min-importance filter.
	results, err = db.QueryMemory(conn, page.PageID, db.QueryMemoryParams{MinImportance: 0.6})
	if err != nil {
		t.Fatalf("QueryMemory minImportance: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("minImportance 0.6 returned %d, want 1", len(results))
	}
	if results[0].Importance < 0.6 {
		t.Fatalf("minImportance result has importance %f, want >= 0.6", results[0].Importance)
	}

	// Test limit.
	results, err = db.QueryMemory(conn, page.PageID, db.QueryMemoryParams{Limit: 2})
	if err != nil {
		t.Fatalf("QueryMemory limit: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("limit 2 returned %d, want 2", len(results))
	}

	// Verify ordering by importance DESC.
	for i := 1; i < len(results); i++ {
		if results[i].Importance > results[i-1].Importance {
			t.Fatalf("results[%d].Importance=%f > results[%d].Importance=%f, want DESC order",
				i, results[i].Importance, i-1, results[i-1].Importance)
		}
	}
}

// TestLoopSeedAndRunWithMemory creates a loop seed, starts a run, writes a
// memory during the run, completes the run, and verifies the history, the
// memory persists, and the seed snapshot in the run.
func TestLoopSeedAndRunWithMemory(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("loop-mem-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("loop-mem-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	// Insert a loop seed.
	seed, err := db.InsertLoopSeed(conn, "review-cycle", "normal", "Code review cycle", "Review all pending changes and report status", 0.8)
	if err != nil {
		t.Fatalf("InsertLoopSeed: %v", err)
	}
	if seed.Name != "review-cycle" {
		t.Fatalf("seed name = %q, want 'review-cycle'", seed.Name)
	}

	// Create a page for the run.
	page, err := db.InsertPage(conn, "/project/loop-test")
	if err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Start a loop run.
	plan := `{"steps":["review","report"]}`
	run, err := db.StartLoopRun(conn, page.PageID, "review-cycle", nil, plan)
	if err != nil {
		t.Fatalf("StartLoopRun: %v", err)
	}
	if run.Status != "active" {
		t.Fatalf("run status = %q, want 'active'", run.Status)
	}
	if run.SeedName != "review-cycle" {
		t.Fatalf("run seed_name = %q, want 'review-cycle'", run.SeedName)
	}
	if run.SeedDescribe != "Code review cycle" {
		t.Fatalf("run seed_describe = %q, want 'Code review cycle'", run.SeedDescribe)
	}

	// Write a memory during the run.
	mem, err := db.InsertMemory(conn, page.PageID, "review-finding", "What was found?", "Found 3 issues", 0.85)
	if err != nil {
		t.Fatalf("InsertMemory during run: %v", err)
	}

	// Complete the loop run.
	result := `{"issues":3,"status":"done"}`
	completed, err := db.CompleteLoopRun(conn, page.PageID, nil, result)
	if err != nil {
		t.Fatalf("CompleteLoopRun: %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("completed status = %q, want 'completed'", completed.Status)
	}

	// Verify history.
	history, err := db.ListLoopHistory(conn, "review-cycle", page.PageID, 10)
	if err != nil {
		t.Fatalf("ListLoopHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0].Status != "completed" {
		t.Fatalf("history[0] status = %q, want 'completed'", history[0].Status)
	}

	// Verify memory persists after run completion.
	memories, err := db.QueryMemory(conn, page.PageID, db.QueryMemoryParams{Keyword: "review-finding"})
	if err != nil {
		t.Fatalf("QueryMemory after run: %v", err)
	}
	if len(memories) != 1 || memories[0].ID != mem.ID {
		t.Fatalf("memory after run: got %d results, want 1 with ID %d", len(memories), mem.ID)
	}

	// Verify seed snapshot in run (seed_describe and seed_content are copied).
	if completed.SeedDescribe != seed.Describe {
		t.Fatalf("run seed_describe = %q, want %q", completed.SeedDescribe, seed.Describe)
	}
	if completed.SeedContent != seed.Content {
		t.Fatalf("run seed_content = %q, want %q", completed.SeedContent, seed.Content)
	}
}

// TestSkillDiscoveryAndQuery verifies that the skill table works in the empty
// state. Since base-agent does not COPY skills, ListSkills returns an empty
// slice and GetSkill returns an error.
func TestSkillDiscoveryAndQuery(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("skill-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("skill-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	// ListSkills returns nil or empty slice since no skills are registered.
	skills, err := db.ListSkills(conn)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("ListSkills returned %d skills, want 0", len(skills))
	}

	// GetSkill with any name should return error since table is empty.
	_, err = db.GetSkill(conn, "code-helper")
	if err == nil {
		t.Fatal("GetSkill('code-helper') returned nil error, want error")
	}

	_, err = db.GetSkill(conn, "nonexistent")
	if err == nil {
		t.Fatal("GetSkill('nonexistent') returned nil error, want error")
	}
}

// TestLoopAutoInjectionInContext inserts a seed, starts a run, then verifies
// that ListActiveRuns returns the active run and ListAvailableLoopSeeds
// returns available seeds, and that ResolveContext injects "loop_focus"
// and "loop_available" fields into the resolved JSON.
func TestLoopAutoInjectionInContext(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("ctx-inject-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("ctx-inject-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	// Insert a loop seed.
	_, err = db.InsertLoopSeed(conn, "daily-sync", "normal", "Synchronize state", "Sync all modules and report", 0.7)
	if err != nil {
		t.Fatalf("InsertLoopSeed: %v", err)
	}

	// Create a page.
	page, err := db.InsertPage(conn, "/project/ctx-test")
	if err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Start a run.
	_, err = db.StartLoopRun(conn, page.PageID, "daily-sync", nil, `{"action":"sync"}`)
	if err != nil {
		t.Fatalf("StartLoopRun: %v", err)
	}

	// ListActiveRuns should show the active main run.
	runs, err := db.ListActiveRuns(conn, page.PageID)
	if err != nil {
		t.Fatalf("ListActiveRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("ListActiveRuns returned no runs, want at least 1")
	}
	if runs[0].SeedName != "daily-sync" {
		t.Fatalf("runs[0].SeedName = %q, want 'daily-sync'", runs[0].SeedName)
	}
	if runs[0].Status != "active" {
		t.Fatalf("runs[0].Status = %q, want 'active'", runs[0].Status)
	}

	// ListAvailableLoopSeeds should contain the seeds (our seed + 2 from init_data).
	seeds, err := db.ListAvailableLoopSeeds(conn)
	if err != nil {
		t.Fatalf("ListAvailableLoopSeeds: %v", err)
	}
	if len(seeds) == 0 {
		t.Fatal("ListAvailableLoopSeeds returned no seeds, want at least one")
	}

	// ResolveContext should inject "loop_focus" and "loop_available" fields.
	irollPath, err := store.IrollPath("ctx-inject-test", "latest")
	if err != nil {
		t.Fatalf("IrollPath: %v", err)
	}

	resolved, err := db.ResolveContext(`{"test":true}`, irollPath, conn, page.PageID)
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(resolved), &parsed); err != nil {
		t.Fatalf("parse resolved context: %v", err)
	}

	// Verify "loop_focus" key exists.
	loopFocusRaw, ok := parsed["loop_focus"]
	if !ok {
		t.Fatal("resolved context missing 'loop_focus' key")
	}

	// Verify loop_focus contains the active run.
	var loopRuns []struct {
		SeedName string `json:"seed_name"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(loopFocusRaw, &loopRuns); err != nil {
		t.Fatalf("parse loop_focus: %v", err)
	}
	if len(loopRuns) == 0 {
		t.Fatal("loop_focus is empty, want at least one run")
	}
	if loopRuns[0].SeedName != "daily-sync" {
		t.Fatalf("loop_focus[0].seed_name = %q, want 'daily-sync'", loopRuns[0].SeedName)
	}
	if loopRuns[0].Status != "active" {
		t.Fatalf("loop_focus[0].status = %q, want 'active'", loopRuns[0].Status)
	}

	// Verify "loop_available" key exists.
	if _, ok := parsed["loop_available"]; !ok {
		t.Fatal("resolved context missing 'loop_available' key")
	}
}
