package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNowISOIsFixedWidthUTCAndLexicallyChronological(t *testing.T) {
	got := nowISO()
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{9}Z$`).MatchString(got) {
		t.Fatalf("nowISO() = %q, want fixed-width UTC nanoseconds", got)
	}
	if _, err := time.Parse(time.RFC3339Nano, got); err != nil {
		t.Fatalf("time.Parse(RFC3339Nano, nowISO()) = %v", err)
	}

	earlier := "2026-06-09T10:00:00.123456788Z"
	later := "2026-06-09T10:00:00.123456789Z"
	if !(earlier < later) {
		t.Fatalf("fixed-width lexical chronology failed: %q >= %q", earlier, later)
	}
}

func TestOpenEnablesForeignKeys(t *testing.T) {
	tests := []struct {
		name            string
		query           string
		wantBusyTimeout int
	}{
		{name: "plain"},
		{name: "preserves benign params", query: "_busy_timeout=1234", wantBusyTimeout: 1234},
		{name: "overrides foreign keys off", query: "_foreign_keys=off"},
		{name: "removes fk alias off", query: "_fk=off"},
		{name: "overrides both conflicting aliases", query: "_foreign_keys=off&_fk=off"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "test.db")
			if tt.query != "" {
				dsn += "?" + tt.query
			}
			conn, err := Open(dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			var enabled int
			if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
				t.Fatal(err)
			}
			if enabled != 1 {
				t.Fatalf("foreign_keys = %d, want 1", enabled)
			}

			if tt.wantBusyTimeout != 0 {
				var busyTimeout int
				if err := conn.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
					t.Fatal(err)
				}
				if busyTimeout != tt.wantBusyTimeout {
					t.Fatalf("busy_timeout = %d, want %d", busyTimeout, tt.wantBusyTimeout)
				}
			}

			if _, err := conn.Exec(`
				CREATE TABLE parent (id INTEGER PRIMARY KEY);
				CREATE TABLE child (parent_id INTEGER REFERENCES parent(id));
			`); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Exec("INSERT INTO child (parent_id) VALUES (1)"); err == nil {
				t.Fatal("insert with invalid foreign key succeeded")
			}
		})
	}
}

func TestDeletePageMissingReturnsStableError(t *testing.T) {
	conn := openLoopTestDB(t)

	if err := DeletePage(conn, "missing-page"); err == nil ||
		!errors.Is(err, ErrPageNotFound) || !strings.Contains(err.Error(), `page "missing-page" not found`) {
		t.Fatalf("DeletePage error = %v", err)
	}
}

func TestResolveContextRejectsFileTraversal(t *testing.T) {
	root := t.TempDir()
	raw := `{"secret":{"@file":"../secret.txt"}}`

	if _, err := ResolveContext(raw, root, &sql.DB{}, "page-a"); err == nil {
		t.Fatal("ResolveContext returned nil error for file traversal")
	}
}

func TestResolveContextAllowsNestedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Resources", "greeting.txt")
	if err := writeTestFile(path, "hello"); err != nil {
		t.Fatal(err)
	}
	raw := `{"greeting":{"@file":"Resources/greeting.txt"}}`

	conn := openLoopTestDB(t)
	got, err := ResolveContext(raw, root, conn, "page-a")
	if err != nil {
		t.Fatalf("ResolveContext returned error: %v", err)
	}
	if got != `{"greeting":"hello","loop_available":[],"loop_focus":[]}` {
		t.Fatalf("ResolveContext = %s, want nested file content", got)
	}
}

func TestResolveContextPreservesInvalidAndNonObjectJSON(t *testing.T) {
	conn := openLoopTestDB(t)
	for _, raw := range []string{`{invalid`, `["array"]`, `"string"`, `null`} {
		got, err := ResolveContext(raw, t.TempDir(), conn, "page-a")
		if err != nil {
			t.Fatalf("ResolveContext(%q) returned error: %v", raw, err)
		}
		if got != raw {
			t.Fatalf("ResolveContext(%q) = %q", raw, got)
		}
	}
}

func TestResolveContextDoesNotChangeStoredPageContext(t *testing.T) {
	conn := openLoopTestDB(t)
	raw := `{"loop":"stored","value":1}`
	now := nowISO()
	if _, err := conn.Exec(`
		INSERT INTO pages (page_id, cwd, context, created_at, updated_at)
		VALUES ('page-a', '.', ?, ?, ?)
	`, raw, now, now); err != nil {
		t.Fatal(err)
	}

	if _, err := ResolveContext(raw, t.TempDir(), conn, "page-a"); err != nil {
		t.Fatal(err)
	}
	page, err := GetPageByPageID(conn, "page-a")
	if err != nil {
		t.Fatal(err)
	}
	if page.Context != raw {
		t.Fatalf("stored context = %q, want %q", page.Context, raw)
	}
}

func writeTestFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}
