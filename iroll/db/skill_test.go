package db

import (
	"database/sql"
	"testing"

	"logos/skill"
)

func openSkillTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestSyncSkillsUpsert(t *testing.T) {
	conn := openSkillTestDB(t)

	skills := []skill.ValidatedSkill{
		{Name: "code-helper", Description: "Help with code", ResourcePath: "Resources/skills/code-helper"},
	}
	if err := SyncSkills(conn, skills); err != nil {
		t.Fatal(err)
	}

	results, err := ListSkills(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "code-helper" {
		t.Fatalf("got %v, want 1 skill named code-helper", results)
	}
	if results[0].Path != "Resources/skills/code-helper/skill.md" {
		t.Fatalf("path = %s, want Resources/skills/code-helper/skill.md", results[0].Path)
	}

	// Upsert with new description
	skills[0].Description = "Updated description"
	if err := SyncSkills(conn, skills); err != nil {
		t.Fatal(err)
	}
	results, _ = ListSkills(conn)
	if results[0].Description != "Updated description" {
		t.Fatalf("description = %s, want 'Updated description'", results[0].Description)
	}
}

func TestSyncSkillsDeleteStale(t *testing.T) {
	conn := openSkillTestDB(t)

	SyncSkills(conn, []skill.ValidatedSkill{
		{Name: "a", Description: "Skill A", ResourcePath: "Resources/skills/a"},
		{Name: "b", Description: "Skill B", ResourcePath: "Resources/skills/b"},
	})
	results, _ := ListSkills(conn)
	if len(results) != 2 {
		t.Fatalf("got %d skills, want 2", len(results))
	}

	// Sync with only "a" — "b" should be deleted
	SyncSkills(conn, []skill.ValidatedSkill{
		{Name: "a", Description: "Skill A", ResourcePath: "Resources/skills/a"},
	})
	results, _ = ListSkills(conn)
	if len(results) != 1 || results[0].Name != "a" {
		t.Fatalf("after stale cleanup, got %v, want 1 skill named a", results)
	}
}

func TestListSkillsFiltersArchived(t *testing.T) {
	conn := openSkillTestDB(t)

	SyncSkills(conn, []skill.ValidatedSkill{
		{Name: "active", Description: "Active skill", ResourcePath: "Resources/skills/active"},
	})

	// Archive one skill
	conn.Exec("UPDATE skill SET archived_at = '2026-01-01T00:00:00Z' WHERE name = 'active'")

	SyncSkills(conn, []skill.ValidatedSkill{
		{Name: "new-skill", Description: "New skill", ResourcePath: "Resources/skills/new-skill"},
	})

	results, _ := ListSkills(conn)
	for _, s := range results {
		if s.Name == "active" {
			t.Fatal("archived skill should not appear in ListSkills")
		}
	}
}

func TestGetSkill(t *testing.T) {
	conn := openSkillTestDB(t)

	SyncSkills(conn, []skill.ValidatedSkill{
		{Name: "my-skill", Description: "My skill", ResourcePath: "Resources/skills/my-skill"},
	})

	s, err := GetSkill(conn, "my-skill")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "my-skill" {
		t.Fatalf("name = %s, want my-skill", s.Name)
	}
	if s.Weight != 0.5 {
		t.Fatalf("weight = %f, want 0.5", s.Weight)
	}

	_, err = GetSkill(conn, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}
}
