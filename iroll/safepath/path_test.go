package safepath

import (
	"path/filepath"
	"testing"
)

func TestValidateName(t *testing.T) {
	t.Parallel()

	valid := []string{"base-agent", "python_expert", "agent.v2", "6061-aluminum"}
	for _, name := range valid {
		name := name
		t.Run("valid_"+name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateName(name); err != nil {
				t.Fatalf("ValidateName(%q) returned error: %v", name, err)
			}
		})
	}

	invalid := []string{"", ".", "..", "../agent", `..\agent`, "agents/base", `agents\base`, "/agent", `C:\agent`}
	for _, name := range invalid {
		name := name
		t.Run("invalid_"+name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateName(name); err == nil {
				t.Fatalf("ValidateName(%q) returned nil", name)
			}
		})
	}
}

func TestJoin(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	got, err := Join(root, filepath.Join("Resources", "books", "index.json"))
	if err != nil {
		t.Fatalf("Join valid path returned error: %v", err)
	}
	want := filepath.Join(root, "Resources", "books", "index.json")
	if got != want {
		t.Fatalf("Join valid path = %q, want %q", got, want)
	}
}

func TestJoinRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := []string{"", ".", "..", filepath.Join("..", "secret.txt"), filepath.Join("Resources", "..", "..", "secret.txt"), filepath.Join(root, "secret.txt")}
	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if _, err := Join(root, path); err == nil {
				t.Fatalf("Join(%q) returned nil error", path)
			}
		})
	}
}
