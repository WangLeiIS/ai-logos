package book

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExistingPathWithinReturnsResolvedInternalTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "actual")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	symlinkOrSkip(t, target, link)

	got, err := existingPathWithin(root, "link")
	if err != nil {
		t.Fatal(err)
	}
	want, err := evalAbsolute(target)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("existingPathWithin() = %q, want resolved target %q", got, want)
	}
}

func TestOpenValidatedFileReturnsHandleForResolvedInternalTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "actual.txt")
	if err := os.WriteFile(target, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.txt")
	symlinkOrSkip(t, target, link)

	file, err := openValidatedFile(root, "linked.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	handleInfo, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(handleInfo, targetInfo) {
		t.Fatal("opened handle does not refer to the validated target")
	}
}

func TestOpenValidatedFileRejectsResolvedExternalTarget(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(root, "linked.txt"))

	if file, err := openValidatedFile(root, "linked.txt"); err == nil {
		file.Close()
		t.Fatal("openValidatedFile accepted an external target")
	}
}

func TestOpenValidatedFileAllowsFileThroughInternalDirectoryLink(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "actual")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "inside.txt")
	if err := os.WriteFile(target, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, targetDir, filepath.Join(root, "linked"))

	file, err := openValidatedFile(root, "linked/inside.txt")
	if err != nil {
		t.Fatalf("openValidatedFile rejected internal directory link: %v", err)
	}
	file.Close()
}

func TestOpenValidatedFileRejectsFileThroughExternalDirectoryLink(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "outside.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outsideDir, filepath.Join(root, "linked"))

	if file, err := openValidatedFile(root, "linked/outside.txt"); err == nil {
		file.Close()
		t.Fatal("openValidatedFile accepted file through external directory link")
	}
}
