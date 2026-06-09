package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveContextRejectsFileTraversal(t *testing.T) {
	root := t.TempDir()
	raw := `{"secret":{"@file":"../secret.txt"}}`

	if _, err := ResolveContext(raw, root, &sql.DB{}); err == nil {
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

	got, err := ResolveContext(raw, root, &sql.DB{})
	if err != nil {
		t.Fatalf("ResolveContext returned error: %v", err)
	}
	if got != `{"greeting":"hello"}` {
		t.Fatalf("ResolveContext = %s, want nested file content", got)
	}
}

func writeTestFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}
