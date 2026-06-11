package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSkillOK(t *testing.T) {
	parent := t.TempDir()
	namedDir := filepath.Join(parent, "test-skill")
	os.MkdirAll(namedDir, 0755)
	os.WriteFile(filepath.Join(namedDir, "skill.md"), []byte("---\nname: test-skill\ndescription: A test skill\n---\n\n# Test\nContent here."), 0644)

	result, err := ValidateSkill(namedDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "test-skill" {
		t.Fatalf("name = %s, want test-skill", result.Name)
	}
	if result.Description != "A test skill" {
		t.Fatalf("description = %s, want 'A test skill'", result.Description)
	}
	if result.ResourcePath != "Resources/skills/test-skill" {
		t.Fatalf("resource_path = %s", result.ResourcePath)
	}
}

func TestValidateSkillMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := ValidateSkill(dir)
	if err == nil {
		t.Fatal("expected error for missing skill.md")
	}
}

func TestValidateSkillMissingName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "skill.md"), []byte("---\ndescription: No name\n---\n"), 0644)
	_, err := ValidateSkill(dir)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateSkillMissingDescription(t *testing.T) {
	parent := t.TempDir()
	namedDir := filepath.Join(parent, "my-skill")
	os.MkdirAll(namedDir, 0755)
	os.WriteFile(filepath.Join(namedDir, "skill.md"), []byte("---\nname: my-skill\n---\n"), 0644)
	_, err := ValidateSkill(namedDir)
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}

func TestValidateSkillNameMismatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "skill.md"), []byte("---\nname: wrong-name\ndescription: test\n---\n"), 0644)
	_, err := ValidateSkill(dir)
	if err == nil {
		t.Fatal("expected error for name mismatch")
	}
}

func TestDiscoverEmpty(t *testing.T) {
	dir := t.TempDir()
	results, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
}

func TestDiscoverWithSkills(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "Resources", "skills", "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("---\nname: my-skill\ndescription: A skill\n---\n\n# My Skill"), 0644)

	results, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "my-skill" {
		t.Fatalf("got %v, want 1 result with name my-skill", results)
	}
}
