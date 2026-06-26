package store

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestIrollPathRejectsUnsafeName(t *testing.T) {
	setTestHome(t)

	if _, err := IrollPath("../outside", "latest"); err == nil {
		t.Fatal("IrollPath returned nil error for unsafe name")
	}
}

func TestExtractRejectsZipSlip(t *testing.T) {
	home := setTestHome(t)

	archivePath := filepath.Join(t.TempDir(), "malicious.iroll")
	writeZip(t, archivePath, map[string]string{
		"roll-inner.db":     "not-a-database",
		"../escaped.txt": "escaped",
	})

	err := Extract(archivePath, "safe-agent", "latest")
	if err == nil {
		t.Fatal("Extract returned nil error for ZIP traversal")
	}

	escapedPath := filepath.Join(home, ".iroll", "escaped.txt")
	if _, statErr := os.Stat(escapedPath); !os.IsNotExist(statErr) {
		t.Fatalf("ZIP traversal wrote outside destination: %v", statErr)
	}
}

func TestExtractRejectsUnsafeName(t *testing.T) {
	setTestHome(t)

	archivePath := filepath.Join(t.TempDir(), "valid.iroll")
	writeZip(t, archivePath, map[string]string{"roll-inner.db": "database"})

	if err := Extract(archivePath, "../escaped", "latest"); err == nil {
		t.Fatal("Extract returned nil error for unsafe iroll name")
	}
}

func TestExtractValidArchive(t *testing.T) {
	home := setTestHome(t)

	archivePath := filepath.Join(t.TempDir(), "valid.iroll")
	writeZip(t, archivePath, map[string]string{
		"roll-inner.db":             "database",
		"Resources/greeting.txt": "hello",
	})

	if err := Extract(archivePath, "safe-agent", "latest"); err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, ".iroll", "safe-agent", "latest", "Resources", "greeting.txt"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("extracted content = %q, want hello", got)
	}
}

func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range files {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
