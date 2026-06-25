package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeLoopJSONPreservesJSONAndWrapsText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "compact object", input: `{ "step": 1 }`, want: `{"step":1}`},
		{name: "compact string", input: ` "review memory" `, want: `"review memory"`},
		{name: "wrap text", input: "review memory", want: `"review memory"`},
		{name: "wrap blank text", input: "", want: `""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLoopJSON(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeLoopJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoopSeedLifecycle(t *testing.T) {
	conn := openLoopTestDB(t)
	seed, err := InsertLoopSeed(conn, " review ", "normal", " Review memory ", " Inspect useful memories ", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	if seed.Name != "review" || seed.Describe != "Review memory" || seed.Content != "Inspect useful memories" {
		t.Fatalf("seed was not trimmed: %#v", seed)
	}
	if seed.ArchivedAt != nil || seed.CreatedAt == "" || seed.UpdatedAt == "" {
		t.Fatalf("unexpected seed metadata: %#v", seed)
	}

	content := " Inspect memories and contradictions "
	weight := 0.9
	updated, err := UpdateLoopSeed(conn, seed.Name, LoopSeedPatch{Content: &content, Weight: &weight})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Content != "Inspect memories and contradictions" || updated.Weight != 0.9 {
		t.Fatalf("updated = %#v", updated)
	}

	got, err := GetLoopSeedByName(conn, " review ")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != seed.ID || got.Content != updated.Content {
		t.Fatalf("GetLoopSeedByName = %#v", got)
	}

	archived, err := ArchiveLoopSeed(conn, seed.Name)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("ArchiveLoopSeed = %#v", archived)
	}
	if got, err := ListLoopSeeds(conn, false); err != nil || len(got) != 0 {
		t.Fatalf("active seeds = %#v, %v", got, err)
	}
	if got, err := ListLoopSeeds(conn, true); err != nil || len(got) != 1 {
		t.Fatalf("all seeds = %#v, %v", got, err)
	}

	restored, err := RestoreLoopSeed(conn, seed.Name)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("RestoreLoopSeed = %#v", restored)
	}
	if err := RemoveLoopSeed(conn, seed.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := GetLoopSeedByName(conn, seed.Name); err == nil || !strings.Contains(err.Error(), "loop seed \"review\" not found") {
		t.Fatalf("GetLoopSeedByName after remove error = %v", err)
	}
}

func TestLoopSeedListUsesStableOrderAndArchivedFilter(t *testing.T) {
	conn := openLoopTestDB(t)
	for _, name := range []string{"zeta", "alpha", "middle"} {
		if _, err := InsertLoopSeed(conn, name, "normal", name, name, 0.5); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ArchiveLoopSeed(conn, "middle"); err != nil {
		t.Fatal(err)
	}

	active, err := ListLoopSeeds(conn, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 || active[0].Name != "alpha" || active[1].Name != "zeta" {
		t.Fatalf("active order = %#v", active)
	}
	all, err := ListLoopSeeds(conn, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Name != "alpha" || all[1].Name != "middle" || all[2].Name != "zeta" {
		t.Fatalf("all order = %#v", all)
	}
}

func TestLoopSeedRejectsDuplicateAndInvalidValues(t *testing.T) {
	conn := openLoopTestDB(t)
	if _, err := InsertLoopSeed(conn, "review", "normal", "Review", "Review memory", 0.5); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertLoopSeed(conn, " review ", "normal", "Other", "Other", 0.5); err == nil ||
		!errors.Is(err, ErrLoopSeedAlreadyExists) || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate error = %v", err)
	}

	tests := []struct {
		name     string
		seedName string
		describe string
		content  string
		weight   float64
		want     string
	}{
		{name: "blank name", seedName: " ", describe: "ok", content: "ok", weight: 0.5, want: "name must not be blank"},
		{name: "blank describe", seedName: "name", describe: " ", content: "ok", weight: 0.5, want: "describe must not be blank"},
		{name: "blank content", seedName: "name", describe: "ok", content: "\t", weight: 0.5, want: "content must not be blank"},
		{name: "low weight", seedName: "name", describe: "ok", content: "ok", weight: -0.1, want: "weight must be between 0 and 1"},
		{name: "high weight", seedName: "name", describe: "ok", content: "ok", weight: 1.1, want: "weight must be between 0 and 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := InsertLoopSeed(conn, tt.seedName, "normal", tt.describe, tt.content, tt.weight)
			if err == nil || !errors.Is(err, ErrInvalidLoopSeed) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("InsertLoopSeed error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoopSeedRejectsInvalidType(t *testing.T) {
	conn := openLoopTestDB(t)
	_, err := InsertLoopSeed(conn, "test", "invalid", "desc", "content", 0.5)
	if err == nil || !errors.Is(err, ErrInvalidLoopSeed) || !strings.Contains(err.Error(), "type must be") {
		t.Fatalf("invalid type error = %v", err)
	}
}

func TestLoopSeedUpdateRejectsEmptyAndInvalidPatch(t *testing.T) {
	conn := openLoopTestDB(t)
	if _, err := InsertLoopSeed(conn, "review", "normal", "Review", "Review memory", 0.5); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateLoopSeed(conn, "review", LoopSeedPatch{}); err == nil ||
		!errors.Is(err, ErrInvalidLoopSeed) || !strings.Contains(err.Error(), "no fields supplied") {
		t.Fatalf("empty patch error = %v", err)
	}

	blank := " "
	if _, err := UpdateLoopSeed(conn, "review", LoopSeedPatch{Content: &blank}); err == nil ||
		!errors.Is(err, ErrInvalidLoopSeed) || !strings.Contains(err.Error(), "content must not be blank") {
		t.Fatalf("blank content error = %v", err)
	}
	invalidWeight := 2.0
	if _, err := UpdateLoopSeed(conn, "review", LoopSeedPatch{Weight: &invalidWeight}); err == nil ||
		!errors.Is(err, ErrInvalidLoopSeed) || !strings.Contains(err.Error(), "weight must be between 0 and 1") {
		t.Fatalf("invalid weight error = %v", err)
	}
}

func TestLoopSeedMutationsReturnTheirOwnUpdatedRow(t *testing.T) {
	dir := t.TempDir()
	innerPath, outerPath := setupDualDB(t, dir)
	conn, err := OpenOuter(outerPath, innerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := InsertLoopSeed(conn, "review", "normal", "Review", "Review memory", 0.5); err != nil {
		t.Fatal(err)
	}
	// Create trigger directly on the inner DB (cross-database triggers are not allowed)
	innerConn, err := Open(innerPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := innerConn.Exec(`
		CREATE TRIGGER replace_loop_seed_after_update
		AFTER UPDATE ON loop
		BEGIN
			UPDATE loop
			SET describe = 'replaced after mutation',
			    archived_at = CASE
			        WHEN NEW.archived_at IS NULL THEN 'replaced-after-restore'
			        ELSE NULL
			    END
			WHERE id = NEW.id;
		END
	`); err != nil {
		innerConn.Close()
		t.Fatal(err)
	}
	innerConn.Close()

	describe := "Updated by mutation"
	updated, err := UpdateLoopSeed(conn, "review", LoopSeedPatch{Describe: &describe})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Describe != describe {
		t.Fatalf("UpdateLoopSeed returned later row state: %#v", updated)
	}

	archived, err := ArchiveLoopSeed(conn, "review")
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("ArchiveLoopSeed returned later row state: %#v", archived)
	}

	restored, err := RestoreLoopSeed(conn, "review")
	if err != nil {
		t.Fatal(err)
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("RestoreLoopSeed returned later row state: %#v", restored)
	}
}

func TestLoopSeedMissingErrorsAreStable(t *testing.T) {
	conn := openLoopTestDB(t)
	for name, call := range map[string]func() error{
		"get": func() error {
			_, err := GetLoopSeedByName(conn, "missing")
			return err
		},
		"update": func() error {
			content := "content"
			_, err := UpdateLoopSeed(conn, "missing", LoopSeedPatch{Content: &content})
			return err
		},
		"archive": func() error {
			_, err := ArchiveLoopSeed(conn, "missing")
			return err
		},
		"restore": func() error {
			_, err := RestoreLoopSeed(conn, "missing")
			return err
		},
		"remove": func() error {
			return RemoveLoopSeed(conn, "missing")
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil || !errors.Is(err, ErrLoopSeedNotFound) || !strings.Contains(err.Error(), `loop seed "missing" not found`) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestLoopSeedRemoveWithHistoryRequiresArchive(t *testing.T) {
	conn := openLoopTestDB(t)
	seed, err := InsertLoopSeed(conn, "review", "normal", "Review", "Review memory", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		INSERT INTO loop_runs (
			loop_id, page_id, seed_name, seed_describe, seed_content, seed_weight,
			status, started_at, updated_at
		) VALUES (?, 'page-one', ?, ?, ?, ?, 'completed', ?, ?)
	`, seed.ID, seed.Name, seed.Describe, seed.Content, seed.Weight, nowISO(), nowISO()); err != nil {
		t.Fatal(err)
	}

	err = RemoveLoopSeed(conn, seed.Name)
	if err == nil || !strings.Contains(err.Error(), "archive") {
		t.Fatalf("RemoveLoopSeed error = %v", err)
	}
	if _, err := GetLoopSeedByName(conn, seed.Name); err != nil {
		t.Fatalf("seed was removed despite history: %v", err)
	}
}

func openLoopTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	innerPath, outerPath := setupDualDB(t, dir)
	conn, err := OpenOuter(outerPath, innerPath)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func setupDualDB(t *testing.T, dir string) (innerPath, outerPath string) {
	t.Helper()
	innerPath = filepath.Join(dir, "roll-inner.db")
	outerPath = filepath.Join(dir, "roll-outer.db")

	// Create inner DB with schema only (no seed data from init_data.sql)
	innerConn, err := Open(innerPath)
	if err != nil {
		t.Fatal(err)
	}
	applyInnerSchema(t, innerConn)
	if err := innerConn.Close(); err != nil {
		t.Fatal(err)
	}

	// Create outer DB with schema
	outerConn, err := Open(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	applyOuterSchema(t, outerConn)
	if err := outerConn.Close(); err != nil {
		t.Fatal(err)
	}

	return innerPath, outerPath
}

func applyInnerSchema(t *testing.T, conn *sql.DB) {
	t.Helper()
	schemaPath := filepath.Join("..", "..", "examples", "base-agent", "init_inner.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
}

func applyOuterSchema(t *testing.T, conn *sql.DB) {
	t.Helper()
	schemaPath := filepath.Join("..", "..", "examples", "base-agent", "init_outer.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
}
