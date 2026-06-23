package e2e

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"logos/builder"
	"logos/db"
	"logos/e2e/testenv"
	"logos/store"
)

// writeMinimalSchema writes a schema.sql with CREATE TABLE metadata and pages
// into the given directory. This provides the minimum tables needed for
// Build to produce a working database.
func writeMinimalSchema(t *testing.T, dir string) {
	t.Helper()
	content := `CREATE TABLE IF NOT EXISTS metadata (
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    remark TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_id TEXT NOT NULL,
    cwd TEXT,
    context TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);`
	if err := os.WriteFile(filepath.Join(dir, "schema.sql"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestBuildWithInvalidSQLFails creates a Irollfile that MIGRATEs a nonexistent
// SQL file. Build should fail and no residue should be left in the store.
func TestBuildWithInvalidSQLFails(t *testing.T) {
	env := testenv.New(t)

	dir := t.TempDir()
	lfContent := "MIGRATE nonexistent.sql\n"
	lfPath := filepath.Join(dir, "Irollfile")
	if err := os.WriteFile(lfPath, []byte(lfContent), 0644); err != nil {
		t.Fatal(err)
	}

	lf, err := builder.ParseIrollfile(lfPath)
	if err != nil {
		t.Fatalf("ParseIrollfile returned error: %v", err)
	}

	_, err = builder.Build(lf, "bad-sql-test")
	if err == nil {
		t.Fatal("Build with nonexistent SQL file should have failed, but succeeded")
	}

	// Verify no residue in store: IrollPath should fail or stat should fail.
	_, pathErr := store.IrollPath("bad-sql-test")
	if pathErr == nil {
		// If IrollPath didn't error, the directory should not exist on disk.
		if _, statErr := os.Stat(filepath.Join(env.Store, "bad-sql-test")); statErr == nil {
			t.Fatal("store directory for 'bad-sql-test' should not exist after failed build")
		}
	}
}

// TestBuildWithInvalidBookFails creates a Irollfile that COPYs a book directory
// missing manifest.json. Build should fail and no residue should be left.
func TestBuildWithInvalidBookFails(t *testing.T) {
	env := testenv.New(t)

	dir := t.TempDir()
	writeMinimalSchema(t, dir)

	// Create a book directory without manifest.json
	bookDir := filepath.Join(dir, "Resources", "books", "bad-book")
	if err := os.MkdirAll(bookDir, 0755); err != nil {
		t.Fatal(err)
	}

	lfContent := "MIGRATE schema.sql\nCOPY Resources Resources\n"
	lfPath := filepath.Join(dir, "Irollfile")
	if err := os.WriteFile(lfPath, []byte(lfContent), 0644); err != nil {
		t.Fatal(err)
	}

	lf, err := builder.ParseIrollfile(lfPath)
	if err != nil {
		t.Fatalf("ParseIrollfile returned error: %v", err)
	}

	_, err = builder.Build(lf, "bad-book-test")
	if err == nil {
		t.Fatal("Build with invalid book (missing manifest) should have failed, but succeeded")
	}

	// Verify no residue in store.
	_, pathErr := store.IrollPath("bad-book-test")
	if pathErr == nil {
		if _, statErr := os.Stat(filepath.Join(env.Store, "bad-book-test")); statErr == nil {
			t.Fatal("store directory for 'bad-book-test' should not exist after failed build")
		}
	}
}

// TestBuildWithInvalidSkillFails creates a skill with a frontmatter that has
// a name but is missing the description field. Build should fail.
func TestBuildWithInvalidSkillFails(t *testing.T) {
	env := testenv.New(t)

	dir := t.TempDir()
	writeMinimalSchema(t, dir)

	// Create a skill directory with a skill.md that has name but no description.
	skillDir := filepath.Join(dir, "Resources", "skills", "broken-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: broken-skill\n---\nSome content without description.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte(skillMD), 0644); err != nil {
		t.Fatal(err)
	}

	lfContent := "MIGRATE schema.sql\nCOPY Resources Resources\n"
	lfPath := filepath.Join(dir, "Irollfile")
	if err := os.WriteFile(lfPath, []byte(lfContent), 0644); err != nil {
		t.Fatal(err)
	}

	lf, err := builder.ParseIrollfile(lfPath)
	if err != nil {
		t.Fatalf("ParseIrollfile returned error: %v", err)
	}

	_, err = builder.Build(lf, "bad-skill-test")
	if err == nil {
		t.Fatal("Build with invalid skill (missing description) should have failed, but succeeded")
	}

	// Verify no residue in store.
	_, pathErr := store.IrollPath("bad-skill-test")
	if pathErr == nil {
		if _, statErr := os.Stat(filepath.Join(env.Store, "bad-skill-test")); statErr == nil {
			t.Fatal("store directory for 'bad-skill-test' should not exist after failed build")
		}
	}
}

// TestDuplicateBuildRejected verifies that building the same tag name twice
// fails on the second attempt.
func TestDuplicateBuildRejected(t *testing.T) {
	env := testenv.New(t)

	// First build should succeed.
	_, err := env.Build("dup-test")
	if err != nil {
		t.Fatalf("first Build returned error: %v", err)
	}

	// Second build with the same name should fail.
	_, err = env.Build("dup-test")
	if err == nil {
		t.Fatal("second Build with same tag should have failed, but succeeded")
	}
}

// TestQueryNonexistentReturnsError verifies that GetSkill, GetBook,
// GetLoopSeedByName, and GetPageByPageID return errors for nonexistent
// entries, and QueryDna returns an empty slice.
func TestQueryNonexistentReturnsError(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("query-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("query-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	// GetSkill should error.
	_, err = db.GetSkill(conn, "no-such-skill")
	if err == nil {
		t.Error("GetSkill('no-such-skill') returned nil error, want error")
	}

	// GetBook should error.
	_, err = db.GetBook(conn, "no-such-book")
	if err == nil {
		t.Error("GetBook('no-such-book') returned nil error, want error")
	}

	// GetLoopSeedByName should error.
	_, err = db.GetLoopSeedByName(conn, "no-such-seed")
	if err == nil {
		t.Error("GetLoopSeedByName('no-such-seed') returned nil error, want error")
	}

	// GetPageByPageID should error.
	_, err = db.GetPageByPageID(conn, "no-such-page-id")
	if err == nil {
		t.Error("GetPageByPageID('no-such-page-id') returned nil error, want error")
	}

	// QueryDna should return an empty slice (not error).
	results, err := db.QueryDna(conn, "no-match-keyword", "")
	if err != nil {
		t.Fatalf("QueryDna('no-match-keyword','') returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("QueryDna('no-match-keyword','') returned %d results, want 0", len(results))
	}
}

// TestContextSQLInjectionSafe verifies that ResolveContext cannot be used to
// inject SQL that modifies data. We pass an @sql reference containing an
// INSERT statement and confirm the skill count does not change.
func TestContextSQLInjectionSafe(t *testing.T) {
	env := testenv.New(t)

	result, err := env.Build("inject-test")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("inject-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	// Count skills before.
	var countBefore int
	if err := conn.QueryRow("SELECT COUNT(*) FROM skill").Scan(&countBefore); err != nil {
		t.Fatalf("count skills before: %v", err)
	}

	// Try to inject via @sql — an INSERT statement.
	maliciousCtx := `{"injected":{"@sql":"INSERT INTO skill (name, description, path, weight, created_at, updated_at) VALUES ('hacked','pwned','x',0.5,'now','now')"}}`
	_, resolveErr := db.ResolveContext(maliciousCtx, result.Path, conn, "0")

	// ResolveContext may or may not return an error; either way the count
	// must not change.
	if resolveErr != nil {
		t.Logf("ResolveContext returned error (expected for non-SELECT): %v", resolveErr)
	}

	// Count skills after.
	var countAfter int
	if err := conn.QueryRow("SELECT COUNT(*) FROM skill").Scan(&countAfter); err != nil {
		t.Fatalf("count skills after: %v", err)
	}

	if countAfter != countBefore {
		t.Fatalf("skill count changed from %d to %d — SQL injection succeeded", countBefore, countAfter)
	}
}

// TestConcurrentPageOperations verifies that two goroutines can concurrently
// InsertPage on the same connection with different cwds without errors.
func TestConcurrentPageOperations(t *testing.T) {
	env := testenv.New(t)

	if _, err := env.Build("concurrent-page-test"); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	conn, err := env.DB("concurrent-page-test")
	if err != nil {
		t.Fatalf("env.DB returned error: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cwd := filepath.Join(t.TempDir(), "workspace", string(rune('A'+idx)))
			if _, insertErr := db.InsertPage(conn, cwd); insertErr != nil {
				errCh <- insertErr
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for e := range errCh {
		t.Errorf("concurrent InsertPage error: %v", e)
	}
}

// TestConcurrentBuildRejectsRace verifies that when two goroutines try to
// build with the same tag name concurrently, exactly one succeeds.
func TestConcurrentBuildRejectsRace(t *testing.T) {
	env := testenv.New(t)

	var wg sync.WaitGroup
	resultCh := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, buildErr := env.Build("race-test")
			resultCh <- buildErr
		}()
	}

	wg.Wait()
	close(resultCh)

	successCount := 0
	for buildErr := range resultCh {
		if buildErr == nil {
			successCount++
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful build, got %d", successCount)
	}
}
