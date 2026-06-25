package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"logos/db"
	"logos/store"
)

func setupEvolvingTest(t *testing.T) (string, string) {
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

	innerPath := filepath.Join(rollRoot, "roll-inner.db")
	conn, err := db.Open(innerPath)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join("..", "..", "examples", "base-agent", "init_inner.sql"))
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if _, err := conn.Exec(string(schema)); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	conn.Close()

	if err := store.IndexPage(rollName, "latest", "page-one", cwd, ""); err != nil {
		t.Fatal(err)
	}
	return cwd, rollRoot
}

func TestResolveEvolvingTargetWithExplicitTag(t *testing.T) {
	name, version := resolveEvolvingTarget([]string{"my-agent:v0.1.0"})
	if name != "my-agent" || version != "v0.1.0" {
		t.Fatalf("resolveEvolvingTarget = (%q, %q), want (my-agent, v0.1.0)", name, version)
	}
}

func TestResolveEvolvingTargetWithDefaultVersion(t *testing.T) {
	name, version := resolveEvolvingTarget([]string{"my-agent"})
	if name != "my-agent" || version != "latest" {
		t.Fatalf("resolveEvolvingTarget = (%q, %q), want (my-agent, latest)", name, version)
	}
}

func TestResolveEvolvingTargetAutoDetect(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	evolvingCwd = cwd
	defer func() { evolvingCwd = origCwd }()

	name, version := resolveEvolvingTarget(nil)
	if name != "test-roll" || version != "latest" {
		t.Fatalf("resolveEvolvingTarget = (%q, %q), want (test-roll, latest)", name, version)
	}
}

func TestResolveEvolvingSQLFromPositionalArgs(t *testing.T) {
	origSQL := evolvingSQL
	origFile := evolvingFile
	evolvingSQL = ""
	evolvingFile = ""
	defer func() {
		evolvingSQL = origSQL
		evolvingFile = origFile
	}()

	sql := resolveEvolvingSQL([]string{"SELECT 1"})
	if sql != "SELECT 1" {
		t.Fatalf("resolveEvolvingSQL = %q, want 'SELECT 1'", sql)
	}

	sql = resolveEvolvingSQL([]string{"my-agent", "SELECT 1"})
	if sql != "SELECT 1" {
		t.Fatalf("resolveEvolvingSQL = %q, want 'SELECT 1'", sql)
	}
}

func TestResolveEvolvingSQLFromFlag(t *testing.T) {
	origSQL := evolvingSQL
	origFile := evolvingFile
	evolvingSQL = "UPDATE dna SET weight=0.5"
	evolvingFile = ""
	defer func() {
		evolvingSQL = origSQL
		evolvingFile = origFile
	}()

	sql := resolveEvolvingSQL([]string{"my-agent"})
	if sql != "UPDATE dna SET weight=0.5" {
		t.Fatalf("resolveEvolvingSQL = %q, want flag SQL", sql)
	}
}

func TestResolveEvolvingSQLFromFile(t *testing.T) {
	origSQL := evolvingSQL
	origFile := evolvingFile
	evolvingSQL = ""
	evolvingFile = filepath.Join(t.TempDir(), "test.sql")
	defer func() {
		evolvingSQL = origSQL
		evolvingFile = origFile
	}()

	if err := os.WriteFile(evolvingFile, []byte("SELECT 2"), 0644); err != nil {
		t.Fatal(err)
	}

	sql := resolveEvolvingSQL(nil)
	if sql != "SELECT 2" {
		t.Fatalf("resolveEvolvingSQL = %q, want 'SELECT 2'", sql)
	}
}

func TestEvolvingEndToEndInsertAndSelect(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	origSQL := evolvingSQL
	origDryRun := evolvingDryRun
	evolvingCwd = cwd
	evolvingSQL = "INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) " +
		"VALUES ('test-e2e', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now')); " +
		"SELECT name, weight FROM dna WHERE name='test-e2e'"
	evolvingDryRun = false
	defer func() {
		evolvingCwd = origCwd
		evolvingSQL = origSQL
		evolvingDryRun = origDryRun
	}()

	name, version := resolveEvolvingTarget(nil)
	dbPath, err := store.DbPath(name, version)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, evolvingSQL, false)
	if err != nil {
		t.Fatalf("ExecuteAll: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Type != "affected" || results[0].AffectedRows != 1 {
		t.Fatalf("results[0] = %#v", results[0])
	}
	if results[1].Type != "rows" || results[1].Count != 1 {
		t.Fatalf("results[1] = %#v", results[1])
	}
}

func TestEvolvingEndToEndDryRun(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	origSQL := evolvingSQL
	origDryRun := evolvingDryRun
	evolvingCwd = cwd
	evolvingSQL = "INSERT INTO dna (name, type, question, answer, weight, created_at, updated_at) " +
		"VALUES ('test-dry', 'idea', 'Q?', 'A.', 0.5, datetime('now'), datetime('now'))"
	evolvingDryRun = true
	defer func() {
		evolvingCwd = origCwd
		evolvingSQL = origSQL
		evolvingDryRun = origDryRun
	}()

	name, version := resolveEvolvingTarget(nil)
	dbPath, err := store.DbPath(name, version)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = db.ExecuteAll(conn, evolvingSQL, true)
	if err != nil {
		t.Fatalf("ExecuteAll dry-run: %v", err)
	}

	var count int
	if err := conn.QueryRow("SELECT count(*) FROM dna WHERE name='test-dry'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("dry-run persisted data: count=%d", count)
	}
}

func TestEvolvingEndToEndJSONOutputFormat(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	origSQL := evolvingSQL
	origDryRun := evolvingDryRun
	evolvingCwd = cwd
	evolvingSQL = "SELECT 1 AS value"
	evolvingDryRun = false
	defer func() {
		evolvingCwd = origCwd
		evolvingSQL = origSQL
		evolvingDryRun = origDryRun
	}()

	name, version := resolveEvolvingTarget(nil)
	dbPath, err := store.DbPath(name, version)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	results, err := db.ExecuteAll(conn, evolvingSQL, false)
	if err != nil {
		t.Fatalf("ExecuteAll: %v", err)
	}

	data, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"type":"rows"`) {
		t.Fatalf("JSON output missing type: %s", string(data))
	}
}

func TestEvolvingInvalidSQLReturnsError(t *testing.T) {
	cwd, _ := setupEvolvingTest(t)

	origCwd := evolvingCwd
	origSQL := evolvingSQL
	evolvingCwd = cwd
	evolvingSQL = "NOT A VALID STATEMENT"
	defer func() {
		evolvingCwd = origCwd
		evolvingSQL = origSQL
	}()

	name, version := resolveEvolvingTarget(nil)
	dbPath, err := store.DbPath(name, version)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = db.ExecuteAll(conn, evolvingSQL, false)
	if err == nil {
		t.Fatal("expected error for invalid SQL")
	}
}
